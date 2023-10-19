package provider

import (
	"context"
	"fmt"
	skopeoPkg "github.com/bsquare-corp/terraform-provider-skopeo2/pkg/skopeo"
	"github.com/containers/common/pkg/auth"
	"github.com/containers/common/pkg/retry"
	"github.com/go-cmd/cmd"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/acctest"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"regexp"
	"testing"
	"time"
)

const (
	testSrcImage         = "docker://127.0.0.1:9016/test-image:latest"
	testSrcImageWithAuth = "docker://127.0.0.1:9017/test-image:latest"
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
		PreCheck: func() {
			copyTestImageToSource(t)
		},
		ProviderFactories: providerFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccCopyResource(rName),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttrSet(fmt.Sprintf("skopeo2_copy.testimage_copy_resource_%s", rName),
						"docker_digest"),
				),
			},
			{
				PreConfig: deleteDest(rName),
				Config:    testAccCopyResource(rName),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttrSet(fmt.Sprintf("skopeo2_copy.testimage_copy_resource_%s", rName),
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
			fmt.Sprintf("docker://127.0.0.1:9016/testimage-copy-resource-%s", name),
			opts)
	}
}

func copyTestImageToSource(t *testing.T) {
	resource.UnitTest(t, resource.TestCase{
		PreCheck: func() {
			testAccPreCheck(t)
		},
		ProviderFactories: providerFactories,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
resource "skopeo2_copy" "testimage_to_src_with_auth" {
    source_image = "%s"
    destination_image = "%s"
    destination {
		login_username = "testuser"
		login_password = "testpassword"
    }
    insecure = true
    copy_all_images = true
    preserve_digests = true
	keep_image = true
}
`, testSrcImage, testSrcImageWithAuth),
			},
		},
	})
}

func TestAccResourceSkopeo2MultiRetry(t *testing.T) {
	rName := acctest.RandStringFromCharSet(10, acctest.CharSetAlphaNum)

	resource.UnitTest(t, resource.TestCase{
		PreCheck: func() {
			copyTestImageToSource(t)
		},
		ProviderFactories: providerFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccCopyResource_multipleRetry(rName),
				Check: resource.ComposeTestCheckFunc(
					resource.ComposeAggregateTestCheckFunc(
						resource.TestCheckResourceAttrSet(fmt.Sprintf("skopeo2_copy.testimage_multiple_retry_%s", rName),
							"docker_digest"),
						resource.TestCheckResourceAttrSet(fmt.Sprintf("skopeo2_copy.testimage_multiple_retry_2_%s",
							rName), "docker_digest"),
						resource.TestCheckResourceAttrSet(fmt.Sprintf("skopeo2_copy.testimage_multiple_retry_3_%s",
							rName), "docker_digest"),
						resource.TestCheckResourceAttrSet(fmt.Sprintf("skopeo2_copy.testimage_multiple_retry_4_%s",
							rName), "docker_digest"),
					),
				),
			},
		},
	})
}

func testAccCopyResource_multipleRetry(name string) string {
	return fmt.Sprintf(`
resource "skopeo2_copy" "testimage_multiple_retry_%s" {
    source_image = "%s"
    destination_image = "docker://127.0.0.1:9016/testimage-multiple-retry-%s"
    source {
      login_retries = 3
    }
    destination {
	  login_script = "if test -f /tmp/testimage-%s; then exit 0; else touch /tmp/testimage-%s; exit 1; fi"
      login_retries = 3
    }
    insecure = true
}

resource "skopeo2_copy" "testimage_multiple_retry_2_%s" {
    source_image = "%s"
    destination_image = "docker://127.0.0.1:9016/testimage-multiple-retry-2-%s"
    source {
	  login_script = "if test -f /tmp/testimage-2-%s; then exit 0; else touch /tmp/testimage-2-%s; exit 1; fi"
      login_retries = 3
    }
    destination {
      login_retries = 3
    }
    insecure = true
}

resource "skopeo2_copy" "testimage_multiple_retry_3_%s" {
    source_image = "%s"
    destination_image = "docker://127.0.0.1:9016/testimage-multiple-retry-3-%s"
    source {
	  login_script = "if test -f /tmp/testimage-3s-%s; then exit 0; else touch /tmp/testimage-3s-%s; exit 1; fi"
      login_retries = 3
    }
    destination {
	  login_script = "if test -f /tmp/testimage-3d-%s; then exit 0; else touch /tmp/testimage-3d-%s; exit 1; fi"
      login_retries = 3
    }
    insecure = true
}

resource "skopeo2_copy" "testimage_multiple_retry_4_%s" {
    source_image = "%s"
    destination_image = "docker://127.0.0.1:9016/testimage-multiple-retry-4-%s"
    source {
      login_retries = 3
    }
    destination {
      login_retries = 3
    }
    insecure = true
}
`, name, testSrcImage, name, name, name, name, testSrcImage, name, name, name, name, testSrcImage, name, name, name,
		name, name, name, testSrcImage, name)
}

func TestAccResourceSkopeo2Login(t *testing.T) {
	//	t.Skipped()
	rName := acctest.RandStringFromCharSet(10, acctest.CharSetAlphaNum)

	resource.UnitTest(t, resource.TestCase{
		PreCheck: func() {
			copyTestImageToSource(t)
		},
		ProviderFactories: providerFactories,
		Steps: []resource.TestStep{
			{
				PreConfig: logoutAll(),
				Config:    testAccCopyResource_loginSourceUnPwScript(rName),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttrSet(fmt.Sprintf("skopeo2_copy.testimage_login_source_%s", rName),
						"docker_digest"),
					CheckImageInRegistry(fmt.Sprintf("docker://127.0.0.1:9016/login-unpw-script-source-%s", rName)),
				),
			},

			{
				PreConfig: logoutAll(),
				Config:    testAccCopyResource_login(rName),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttrSet(fmt.Sprintf("skopeo2_copy.testimage_login_source_2_%s", rName),
						"docker_digest"),
					CheckImageInRegistry(fmt.Sprintf("docker://127.0.0.1:9016/login-us-west-1-source-%s", rName)),
				),
			},

			{
				PreConfig: logoutAll(),
				Config:    testAccCopyResource_loginSourceRetry(rName),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttrSet(fmt.Sprintf("skopeo2_copy.testimage_login_source_retry_%s", rName),
						"docker_digest"),
					CheckImageInRegistry(fmt.Sprintf("docker://127.0.0.1:9016/login-source-retry-%s", rName)),
				),
			},

			{
				PreConfig: logoutAll(),
				Config:    testAccCopyResource_loginSourceWithPassword(rName),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttrSet(fmt.Sprintf("skopeo2_copy.testimage_login_source_with_password_%s", rName),
						"docker_digest"),
					CheckImageInRegistry(fmt.Sprintf("docker://127.0.0.1:9016/login-source-with-password-%s", rName)),
				),
			},
			{
				PreConfig: logoutAll(),
				Config:    testAccCopyResource_loginSourceWithPasswordAndAuthfile(rName),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttrSet(fmt.Sprintf("skopeo2_copy.testimage_login_source_with_password_and_authfile_%s", rName),
						"docker_digest"),
					CheckImageInRegistry(fmt.Sprintf("docker://127.0.0.1:9016/login-source-with-password-and-authfile-%s", rName)),
				),
			},
		},
	})
}

func CheckImageInRegistry(imageName string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		ctx := context.Background()
		imageOpts := skopeoPkg.ImageOptions{
			DockerImageOptions: skopeoPkg.DockerImageOptions{
				Global:       &skopeoPkg.GlobalOptions{},
				Shared:       &skopeoPkg.SharedImageOptions{},
				AuthFilePath: auth.GetDefaultAuthFile(),
				Insecure:     true,
			},
		}
		src, err := skopeoPkg.ParseImageSource(ctx, &imageOpts, imageName)
		if err != nil {
			return err
		}
		defer src.Close()

		_, _, err = src.GetManifest(ctx, nil)
		if err != nil {
			return err
		}
		return nil
	}
}

func dockerLogout(registry string) {
	ctx := context.Background()
	loginCmd := cmd.NewCmdOptions(cmd.Options{Buffered: true}, "docker", "logout", registry)
	loginCmd.Env = os.Environ()
	loginCmd.Dir = "."

	statusChan := loginCmd.Start()

	go func() {
		<-time.After(time.Duration(defaultTimeout) * time.Second)
		err := loginCmd.Stop()
		if err != nil {
			tflog.Info(ctx, "Logout script failed to be stopped after timeout")
		}
	}()

	result := <-statusChan
	if !result.Complete {
		tflog.Warn(ctx, "Logout timed out or was signalled")
		return
	}
	if result.Error != nil {
		tflog.Info(ctx, "Logout failed", map[string]any{"err": result.Error})
		return
	}
	if result.Exit != 0 {
		tflog.Info(ctx, "Logout returned non-zero exit status", map[string]any{"status": result.Exit})
		return
	}
	tflog.Info(ctx, result.Stdout[0])
}

// func Logout(systemContext *types.SystemContext, opts *LogoutOptions, args []string) error {
func logoutAll() func() {
	return func() {
		imageOpts := skopeoPkg.ImageOptions{
			DockerImageOptions: skopeoPkg.DockerImageOptions{
				Global:       &skopeoPkg.GlobalOptions{},
				Shared:       &skopeoPkg.SharedImageOptions{},
				AuthFilePath: auth.GetDefaultAuthFile(),
				Insecure:     true,
			},
		}
		sysCtx, err := imageOpts.NewSystemContext()
		if err != nil {
			return
		}

		err = auth.Logout(sysCtx, &auth.LogoutOptions{All: true, Stdout: os.Stdout}, nil)
		if err != nil {
			return
		}

		imageOpts.DockerImageOptions.AuthFilePath = "/tmp/auth.json"

		sysCtx, err = imageOpts.NewSystemContext()
		if err != nil {
			return
		}

		err = auth.Logout(sysCtx, &auth.LogoutOptions{All: true, Stdout: os.Stdout}, nil)
		if err != nil {
			return
		}

		dockerLogout("127.0.0.1:9017")
		dockerLogout("127.0.0.1:9018")
	}
}

func testAccCopyResource_loginSourceUnPwScript(name string) string {
	return fmt.Sprintf(`
resource "skopeo2_copy" "testimage_login_source_%s" {
    source {
      image = "%s"
      login_username = "testuser"
      login_password_script = "echo testpassword"
      login_retries = 3
    }
    destination {
      image = "docker://127.0.0.1:9016/login-unpw-script-source-%s"
      login_retries = 3
    }
    insecure = true
}
`, name, testSrcImageWithAuth, name)
}

func testAccCopyResource_login(name string) string {
	return fmt.Sprintf(`
resource "skopeo2_copy" "testimage_login_source_%s" {
    source {
      image = "%s"
      login_script = "echo testpassword | docker login --username testuser --password-stdin 127.0.0.1:9017"
    }
    destination {
      image = "docker://127.0.0.1:9016/login-us-west-1-source-%s"
    }
    insecure = true
}

resource "skopeo2_copy" "testimage_login_source_2_%s" {
    source {
      image = "%s"
    }
    destination {
      image = "docker://127.0.0.1:9018/login-us-west-2-source-%s"
      login_script = "echo testpassword2 | docker login --username testuser --password-stdin 127.0.0.1:9018"
      login_retries = 3 
    }
    insecure = true
}
`, name, testSrcImageWithAuth, name, name, testSrcImage, name)
}

func testAccCopyResource_loginSourceRetry(name string) string {
	return fmt.Sprintf(`
resource "skopeo2_copy" "testimage_login_source_retry_%s" {
    source {
      image = "%s"
      login_script  = <<-EOT
if test -f /tmp/tf-%s; then
    echo testpassword | docker login --username testuser --password-stdin 127.0.0.1:9017
else
	touch /tmp/tf-%s
fi
EOT
      login_retries = 3
    }
    destination {
      image = "docker://127.0.0.1:9016/login-source-retry-%s"
      login_retries = 3
    }
    insecure = true
}
`, name, testSrcImageWithAuth, name, name, name)
}

func testAccCopyResource_loginSourceWithPassword(name string) string {
	return fmt.Sprintf(`
provider "skopeo2" {
    source {
		login_username = "testuser"
		login_password = "testpassword"
    }
}

resource "skopeo2_copy" "testimage_login_source_with_password_%s" {
    source_image = "%s"
    destination_image = "docker://127.0.0.1:9016/login-source-with-password-%s"
    insecure = true
}
`, name, testSrcImageWithAuth, name)
}

func testAccCopyResource_loginSourceWithPasswordAndAuthfile(name string) string {
	return fmt.Sprintf(`
provider "skopeo2" {
    source {
		login_username = "testuser"
		login_password = "testpassword"
		registry_auth_file = "/tmp/auth.json"
    }
}

resource "skopeo2_copy" "testimage_login_source_with_password_and_authfile_%s" {
    source_image = "%s"
    destination_image = "docker://127.0.0.1:9016/login-source-with-password-and-authfile-%s"
    insecure = true
}
`, name, testSrcImageWithAuth, name)
}

func TestAccResourceSkopeo2(t *testing.T) {
	rName := acctest.RandStringFromCharSet(10, acctest.CharSetAlphaNum)

	resource.UnitTest(t, resource.TestCase{
		PreCheck: func() {
			copyTestImageToSource(t)
		},
		ProviderFactories: providerFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccCopyResource(rName),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttrSet(fmt.Sprintf("skopeo2_copy.testimage_copy_resource_%s", rName),
						"docker_digest"),
				),
			},
			{
				Config: testAccCopyResourceOldImageParams(rName),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttrSet(fmt.Sprintf("skopeo2_copy."+
						"testimage_copy_resource_old_image_params_%s", rName),
						"docker_digest"),
				),
			},
			{
				Config: testAccCopyResourceMultiImage(rName),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttrSet(fmt.Sprintf("skopeo2_copy.testimage_copy_resource_multi_image_%s",
						rName),
						"docker_digest"),
				),
			},

			{
				Config:      testAccCopyResourceFail(rName),
				ExpectError: expectErrorRegExpr("manifest unknown"),
			},
			{
				Config:      testAccCopyBadResourceFail(rName),
				ExpectError: expectErrorRegExpr("Invalid image name"),
			},
			{
				Config:      testAccCopyResourceLoginFail(rName),
				ExpectError: expectErrorRegExpr("login password script failed"),
			},
			{
				Config:      testAccCopyResourceLoginTimeoutSrc(rName),
				ExpectError: expectErrorRegExpr("login password script timed out"),
			},
			{
				Config:      testAccCopyResourceLoginTimeoutDest(rName),
				ExpectError: expectErrorRegExpr("login password script timed out"),
			},
			{
				Config: testAccCopyResource_addTag(rName),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttrSet(fmt.Sprintf("skopeo2_copy.testimage_add_tag_%s", rName),
						"docker_digest"),
				),
			},
			{
				Config: testAccCopyResource_withRetry(rName),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttrSet(fmt.Sprintf("skopeo2_copy.testimage_with_retry_%s", rName),
						"docker_digest"),
				),
			},
			{
				Config: testAccCopyResourceWithDigest(rName),
				Check: resource.ComposeTestCheckFunc(
					resource.TestMatchResourceAttr(fmt.Sprintf("skopeo2_copy.testimage_copy_resource_digest_%s", rName),
						"docker_digest", regexp.MustCompile(`^sha256`)),
				),
			},
		},
	})
}

func expectErrorRegExpr(expr string) *regexp.Regexp {
	re, _ := regexp.Compile(expr)
	return re
}

func testAccCopyResource(name string) string {
	return fmt.Sprintf(`
resource "skopeo2_copy" "testimage_copy_resource_%s" {
    source_image = "%s"
    destination_image = "docker://127.0.0.1:9016/testimage-copy-resource-%s"
    insecure = true
}`, name, testSrcImage, name)
}

func testAccCopyResourceOldImageParams(name string) string {
	return fmt.Sprintf(`
resource "skopeo2_copy" "testimage_copy_resource_old_image_params_%s" {
    source {
		image = "%s"
	}
    destination {
		image = "docker://127.0.0.1:9016/testimage-copy-resource-old-image-params-%s"
	}
    insecure = true
}`, name, testSrcImage, name)
}

func testAccCopyResourceWithDigest(name string) string {
	return fmt.Sprintf(`
resource "skopeo2_copy" "testimage_copy_resource_digest_%s" {
    source_image = "%s"
    destination_image = "docker://127.0.0.1:9016/testimage-copy-resource-digest-%s"
    insecure = true
    docker_digest = "testvalue"
    lifecycle {
        ignore_changes = [
          # For unit test only, because the teat fails due to the test value of docker_digest not matching the actual
          # destination digest
          docker_digest,
      ]
    }
}`, name, testSrcImage, name)
}

func testAccCopyResourceMultiImage(name string) string {
	return fmt.Sprintf(`
resource "skopeo2_copy" "testimage_copy_resource_multi_image_%s" {
    source_image = "%s"
    destination_image = "docker://127.0.0.1:9016/testimage-copy-resource-multi-image-%s"
    insecure = true
    copy_all_images = true
}`, name, testSrcImage, name)
}

func testAccCopyResourceFail(name string) string {
	return fmt.Sprintf(`
resource "skopeo2_copy" "testimage_resource_fail_%s" {
    source_image = "docker://127.0.0.1:9016/testimage-bad"
    destination_image = "docker://127.0.0.1:9016/testimage-resource-fail-%s"
    source {
      login_retries = 3
    }
    destination {
      login_retries = 3
    }
    insecure = true
}`, name, name)
}

func testAccCopyBadResourceFail(name string) string {
	return fmt.Sprintf(`
resource "skopeo2_copy" "testimage_bad_resource_%s" {
    source_image = "cocker://testimage-bad"
    destination_image = "docker://127.0.0.1:9016/testimage-bad-resource-%s"
    insecure = true
}`, name, name)
}

func testAccCopyResourceLoginFail(name string) string {
	return fmt.Sprintf(`
resource "skopeo2_copy" "testimage_login_fail_%s" {
    source_image = "docker://testimage-bad"
    destination_image = "docker://127.0.0.1:9016/testimage-login-fail-%s"
    source {
      login_script = "false"
    }
    destination {
      login_script = "false"
    }
    insecure = true
}`, name, name)
}

func testAccCopyResourceLoginTimeoutDest(name string) string {
	return fmt.Sprintf(`
resource "skopeo2_copy" "testimage_login_timeout_dest_%s" {
    source_image = "docker://testimage-bad"
    destination_image = "docker://127.0.0.1:9016/testimage-login-timeout-dest-%s"
    source {
      login_script = "true"
    }
    destination {
      login_script = "sleep 5"
	  timeout = 2
    }
    insecure = true
}`, name, name)
}

func testAccCopyResourceLoginTimeoutSrc(name string) string {
	return fmt.Sprintf(`
resource "skopeo2_copy" "testimage_login_timeout_src_%s" {
    source_image = "docker://testimage-bad"
    destination_image = "docker://127.0.0.1:9016/testimage-login-timeout-src-%s"
    source {
      login_script = "sleep 5"
	  timeout = 2
    }
    destination {
      login_script = "true"
    }
    insecure = true
}`, name, name)
}

func testAccCopyResource_withRetry(name string) string {
	return fmt.Sprintf(`
resource "skopeo2_copy" "testimage_with_retry_%s" {
    source_image = "%s"
    destination_image = "docker://127.0.0.1:9016/testimage-with-retry-%s"
    retries = 2
    retry_delay = 30
    insecure = true
}`, name, testSrcImage, name)
}

func testAccCopyResource_addTag(name string) string {
	return fmt.Sprintf(`
resource "skopeo2_copy" "testimage_add_tag_%s" {
    source_image = "%s"
    destination_image = "docker://127.0.0.1:9016/testimage-add-tag-%s"
	additional_tags   = ["testimage:fine"]
	keep_image        = true
    insecure          = true
}`, name, testSrcImage, name)
}

func TestAccResourceSkopeo2_ghcrMatch(t *testing.T) {
	// Check the matching cases
	images := []string{
		"ghcr.io/external-secrets/external-secrets",
		"docker://ghcr.io/external-secrets/external-secrets",
	}
	for _, image := range images {
		if !IsGhcr(&image) {
			t.Errorf("Image (%s) was not detected as located in the Githib code repository", image)
		}
	}

	// Check the non-matching cases
	images = []string{
		"docker://external-secrets/external-secrets",
		"external-secrets/external-secrets",
		"docker://mirror/ghcr.io/external-secrets/external-secrets",
		"mirror/ghcr.io/external-secrets/external-secrets",
	}
	for _, image := range images {
		if IsGhcr(&image) {
			t.Errorf("Repository (%s) was detected as located in the Githib code repository", image)
		}
	}
}
