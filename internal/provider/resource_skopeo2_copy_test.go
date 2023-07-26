package provider

import (
	"context"
	"fmt"
	skopeoPkg "github.com/bsquare-corp/terraform-provider-skopeo2/pkg/skopeo"
	"github.com/containers/common/pkg/retry"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/acctest"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"regexp"
	"testing"
)

func serveReverseProxy(target string, res http.ResponseWriter, req *http.Request) {
	// parse the targetUrl
	targetUrl, _ := url.Parse(target)

	// create the reverse proxy
	proxy := httputil.NewSingleHostReverseProxy(targetUrl)

	// Update the headers to allow for SSL redirection
	req.URL.Host = "ghcr.io"
	req.URL.Scheme = targetUrl.Scheme
	req.Header.Set("X-Forwarded-Host", req.Header.Get("Host"))
	req.Host = targetUrl.Host

	// Note that ServeHttp is non blocking and uses a go routine under the hood
	proxy.ServeHTTP(res, req)
}

func handleRequestAndRedirect(res http.ResponseWriter, req *http.Request) {
	serveReverseProxy(req.RequestURI, res, req)
}

func TestAccResourceSkopeo2_deletedDestination(t *testing.T) {
	// This test is not to be run in parallel as the state achieved by the first test step is used in the second

	rName := acctest.RandStringFromCharSet(10, acctest.CharSetAlphaNum)

	resource.UnitTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: providerFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccCopyResource(rName),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttrSet(fmt.Sprintf("skopeo2_copy.alpine_copy_resource_%s", rName),
						"docker_digest"),
				),
			},
			{
				PreConfig: deleteDest(rName),
				Config:    testAccCopyResource(rName),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttrSet(fmt.Sprintf("skopeo2_copy.alpine_copy_resource_%s", rName),
						"docker_digest"),
				),
			},
		},
	})
}

func deleteDest(name string) func() {
	return func() {
		opts := &skopeoPkg.DeleteOptions{
			Image: &skopeoPkg.ImageOptions{
				DockerImageOptions: skopeoPkg.DockerImageOptions{
					Global:       &skopeoPkg.GlobalOptions{},
					Shared:       &skopeoPkg.SharedImageOptions{},
					AuthFilePath: os.Getenv("REGISTRY_AUTH_FILE"),
					Insecure:     true,
				},
			},
			RetryOpts: &retry.RetryOptions{},
		}

		_ = skopeoPkg.Delete(context.Background(),
			fmt.Sprintf("docker://127.0.0.1:9016/alpine-copy-resource-%s", name),
			opts)
	}
}

func TestAccResourceSkopeo2(t *testing.T) {
	t.Parallel()

	rName := acctest.RandStringFromCharSet(10, acctest.CharSetAlphaNum)

	resource.UnitTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: providerFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccCopyResource(rName),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttrSet(fmt.Sprintf("skopeo2_copy.alpine_copy_resource_%s", rName),
						"docker_digest"),
				),
			},
			{
				Config: testAccCopyResourceMultiImage(rName),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttrSet(fmt.Sprintf("skopeo2_copy.alpine_copy_resource_multi_image_%s",
						rName),
						"docker_digest"),
				),
			},
			{
				Config:      testAccCopyResourceFail(rName),
				ExpectError: expectErrorRegExpr("requested access to the resource is denied"),
			},
			{
				Config:      testAccCopyBadResourceFail(rName),
				ExpectError: expectErrorRegExpr("Invalid image name"),
			},
			{
				Config:      testAccCopyResourceLoginFail(rName),
				ExpectError: expectErrorRegExpr("Login script failed"),
			},
			{
				Config: testAccCopyResource_addTag(rName),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttrSet(fmt.Sprintf("skopeo2_copy.alpine_add_tag_%s", rName),
						"docker_digest"),
				),
			},
			{
				Config: testAccCopyResource_withRetry(rName),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttrSet(fmt.Sprintf("skopeo2_copy.alpine_with_retry_%s", rName),
						"docker_digest"),
				),
			},
			{
				Config: testAccCopyResource_multipleRetry(rName),
				Check: resource.ComposeTestCheckFunc(
					resource.ComposeAggregateTestCheckFunc(
						resource.TestCheckResourceAttrSet(fmt.Sprintf("skopeo2_copy.alpine_multiple_retry_%s", rName),
							"docker_digest"),
						resource.TestCheckResourceAttrSet(fmt.Sprintf("skopeo2_copy.alpine_multiple_retry_2_%s",
							rName), "docker_digest"),
						resource.TestCheckResourceAttrSet(fmt.Sprintf("skopeo2_copy.alpine_multiple_retry_3_%s",
							rName), "docker_digest"),
						resource.TestCheckResourceAttrSet(fmt.Sprintf("skopeo2_copy.alpine_multiple_retry_4_%s",
							rName), "docker_digest"),
					),
				),
			},
			{
				Config: testAccCopyResourceWithDigest(rName),
				Check: resource.ComposeTestCheckFunc(
					resource.TestMatchResourceAttr(fmt.Sprintf("skopeo2_copy.alpine_copy_resource_digest_%s", rName),
						"docker_digest", regexp.MustCompile(`^sha256`)),
				),
			},

			/*
				{ // TODO Login source can only be executed with an actual AWS account
					Config: testAccCopyResource_loginSourceUnPwScript(rName),
					Check: resource.ComposeTestCheckFunc(
						resource.TestCheckResourceAttrSet(fmt.Sprintf("skopeo2_copy.alpine_login_source_%s", rName),
							"docker_digest"),
					),
				},

					{ // TODO Login source can only be executed with an actual AWS account
						Config: testAccCopyResource_loginSource(rName),
						Check: resource.ComposeTestCheckFunc(
							resource.TestCheckResourceAttrSet(fmt.Sprintf("skopeo2_copy.alpine_login_source_%s", rName),
								"docker_digest"),
						),
					},
					{
						Config: testAccCopyResource_loginSourceRetry(rName),
						Check: resource.ComposeTestCheckFunc(
							resource.TestCheckResourceAttrSet(fmt.Sprintf("skopeo2_copy.alpine_login_source_retry_%s", rName),
								"docker_digest"),
						),
					},
			*/
		},
	})
}

func expectErrorRegExpr(expr string) *regexp.Regexp {
	re, _ := regexp.Compile(expr)
	return re
}

func testAccCopyResource(name string) string {
	return fmt.Sprintf(`resource "skopeo2_copy" "alpine_copy_resource_%s" {
    source {
	  image = "docker://alpine"
    }
    destination {
	  image = "docker://127.0.0.1:9016/alpine-copy-resource-%s"
    }
    insecure = true
}`, name, name)
}

func testAccCopyResourceWithDigest(name string) string {
	return fmt.Sprintf(`resource "skopeo2_copy" "alpine_copy_resource_digest_%s" {
    source {
	  image = "docker://alpine"
    }
    destination {
	  image = "docker://127.0.0.1:9016/alpine-copy-resource-digest-%s"
    }
    insecure = true
    docker_digest = "testvalue"
    lifecycle {
        ignore_changes = [
          # For unit test only, because the teat fails due to the test value of docker_digest not matching the actual
          # destination digest
          docker_digest,
      ]
    }
}`, name, name)
}

func testAccCopyResourceMultiImage(name string) string {
	return fmt.Sprintf(`resource "skopeo2_copy" "alpine_copy_resource_multi_image_%s" {
    source {
	  image = "docker://alpine"
    }
    destination {
	  image = "docker://127.0.0.1:9016/alpine-copy-resource-multi-image-%s"
    }
    insecure = true
    copy_all_images = true
}`, name, name)
}

func testAccCopyResourceFail(name string) string {
	return fmt.Sprintf(`resource "skopeo2_copy" "alpine_resource_fail_%s" {
    source {
	  image = "docker://alpine-bad"
      login_retries = 3
    }
    destination {
	  image = "docker://127.0.0.1:9016/alpine-resource-fail-%s"
      login_retries = 3
    }
    insecure = true
}`, name, name)
}

func testAccCopyBadResourceFail(name string) string {
	return fmt.Sprintf(`resource "skopeo2_copy" "alpine_bad_resource_%s" {
    source {
	  image = "cocker://alpine-bad"
    }
    destination {
	  image = "docker://127.0.0.1:9016/alpine-bad-resource-%s"
    }
    insecure = true
}`, name, name)
}

func testAccCopyResourceLoginFail(name string) string {
	return fmt.Sprintf(`resource "skopeo2_copy" "alpine_login_fail_%s" {
    source {
	  image = "docker://alpine-bad"
      login_script = "false"
    }
    destination {
	  image = "docker://127.0.0.1:9016/alpine-login-fail-%s"
      login_script = "false"
    }
    insecure = true
}`, name, name)
}

func testAccCopyResource_loginSource(name string) string {
	return fmt.Sprintf(`
resource "skopeo2_copy" "alpine_login_source_%s" {
    source {
	  image         = "docker://753989949864.dkr.ecr.us-west-1.amazonaws.com/ecr-public/docker/library/alpine"
      login_script = "aws --profile bsquare-jenkins2 ecr get-login-password --region us-west-1 | docker login --username AWS --password-stdin 753989949864.dkr.ecr.us-west-1.amazonaws.com"
    }
    destination {
	  image = "docker://127.0.0.1:9016/alpine-login-source-%s"
    }
    insecure = true
}

resource "skopeo2_copy" "alpine_login_source_2_%s" {
    source {
	  image         = "docker://753989949864.dkr.ecr.us-west-2.amazonaws.com/ecr-public/docker/library/alpine"
      login_script  = "aws --profile bsquare-jenkins2 ecr get-login-password --region us-west-2 | docker login --username AWS --password-stdin 753989949864.dkr.ecr.us-west-2.amazonaws.com"
      login_retries = 3
    }
    destination {
	  image = "docker://127.0.0.1:9016/alpine-login-source-2-%s"
      login_retries = 3
    }
    insecure = true
}
`, name, name, name, name)
}

func testAccCopyResource_loginSourceRetry(name string) string {
	return fmt.Sprintf(`
resource "skopeo2_copy" "alpine_login_source_retry_%s" {
    source {
	  image         = "docker://753989949864.dkr.ecr.us-west-2.amazonaws.com/ecr-public/docker/library/alpine"
      login_script  = <<-EOT
if test -f /tmp/tf-%s; then
	aws --profile bsquare-jenkins2 ecr get-login-password --region us-west-2 | docker login --username AWS --password-stdin 753989949864.dkr.ecr.us-west-2.amazonaws.com
else
	touch /tmp/tf-%s
fi
EOT
      login_retries = 3
    }
    destination {
	  image = "docker://127.0.0.1:9016/alpine-login-source-retry-%s"
      login_retries = 3
    }
    insecure = true
}
`, name, name, name, name)
}

func testAccCopyResource_loginSourceUnPwScript(name string) string {
	return fmt.Sprintf(`
resource "skopeo2_copy" "alpine_login_source_%s" {
    source {
	  image         = "docker://753989949864.dkr.ecr.us-west-1.amazonaws.com/ecr-public/docker/library/alpine"
      login_username = "AWS"
      login_password_script = "aws --profile bsquare-jenkins2 ecr get-login-password --region us-west-1"
      login_retries = 3
    }
    destination {
	  image = "docker://127.0.0.1:9016/alpine-login-unpw-script-source-%s"
      login_retries = 3
    }
    insecure = true
}
`, name, name)
}

func testAccCopyResource_withRetry(name string) string {
	return fmt.Sprintf(`resource "skopeo2_copy" "alpine_with_retry_%s" {
    source {
	  image = "docker://alpine"
    }
    destination {
	  image = "docker://127.0.0.1:9016/alpine-with-retry-%s"
    }
    retries = 2
    retry_delay = 30
    insecure = true
}`, name, name)
}

func testAccCopyResource_multipleRetry(name string) string {
	return fmt.Sprintf(`
resource "skopeo2_copy" "alpine_multiple_retry_%s" {
    source {
	  image = "docker://alpine"
      login_retries = 3
    }
    destination {
	  image = "docker://127.0.0.1:9016/alpine-multiple-retry-%s"
	  login_script = "if test -f /tmp/alpine-%s; then exit 0; else touch /tmp/alpine-%s; exit 1; fi"
      login_retries = 3
    }
    insecure = true
}

resource "skopeo2_copy" "alpine_multiple_retry_2_%s" {
    source {
	  image = "docker://alpine"
	  login_script = "if test -f /tmp/alpine-2-%s; then exit 0; else touch /tmp/alpine-2-%s; exit 1; fi"
      login_retries = 3
    }
    destination {
	  image = "docker://127.0.0.1:9016/alpine-multiple-retry-2-%s"
      login_retries = 3
    }
    insecure = true
}

resource "skopeo2_copy" "alpine_multiple_retry_3_%s" {
    source {
	  image = "docker://alpine"
	  login_script = "if test -f /tmp/alpine-3s-%s; then exit 0; else touch /tmp/alpine-3s-%s; exit 1; fi"
      login_retries = 3
    }
    destination {
	  image = "docker://127.0.0.1:9016/alpine-multiple-retry-3-%s"
	  login_script = "if test -f /tmp/alpine-3d-%s; then exit 0; else touch /tmp/alpine-3d-%s; exit 1; fi"
      login_retries = 3
    }
    insecure = true
}

resource "skopeo2_copy" "alpine_multiple_retry_4_%s" {
    source {
	  image = "docker://alpine"
      login_retries = 3
    }
    destination {
	  image = "docker://127.0.0.1:9016/alpine-multiple-retry-4-%s"
      login_retries = 3
    }
    insecure = true
}
`, name, name, name, name, name, name, name, name, name, name, name, name, name, name, name, name)
}

func testAccCopyResource_addTag(name string) string {
	return fmt.Sprintf(`resource "skopeo2_copy" "alpine_add_tag_%s" {
    source {
	  image = "docker://alpine"
    }
    destination {
	  image = "docker://127.0.0.1:9016/alpine-add-tag-%s"
    }
	additional_tags   = ["alpine:fine"]
	keep_image        = true
    insecure          = true
}`, name, name)
}
