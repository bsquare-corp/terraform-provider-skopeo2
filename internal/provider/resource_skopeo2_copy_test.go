package provider

import (
	"fmt"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/acctest"
	"net/http"
	"net/http/httputil"
	"net/url"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
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
					resource.TestCheckResourceAttrSet(fmt.Sprintf("skopeo2_copy.alpine_%s", rName), "docker_digest"),
				),
			},
			{
				Config: testAccCopyResource_addTag(rName),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttrSet(fmt.Sprintf("skopeo2_copy.alpine_%s", rName), "docker_digest"),
				),
			},
			{
				Config: testAccCopyResource_withRetry(rName),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttrSet(fmt.Sprintf("skopeo2_copy.alpine_%s", rName), "docker_digest"),
				),
			},
		},
	})
}

func testAccCopyResource(name string) string {
	return fmt.Sprintf(`resource "skopeo2_copy" "alpine_%s" {
    source {
	  image = "docker://alpine"
    }
    destination {
	  image = "docker://localhost:5000/alpine"
    }
    insecure = true
}`, name)
}

func testAccCopyResource_loginSource(name string) string {
	return fmt.Sprintf(`resource "skopeo2_copy" "alpine_%s" {
    source {
	  image         = "docker://753989949864.dkr.ecr.us-west-1.amazonaws.com/blib/deployed-container-scanner-trivy:latest"
      login_script = "aws --profile bsquare-jenkins2 ecr get-login-password --region us-west-1 | docker login --username AWS --password-stdin 753989949864.dkr.ecr.us-west-1.amazonaws.com"
    }
    destination {
	  image = "docker://localhost:5000/deployed-container-scanner-trivy"
    }
    insecure = true
}`, name)
}

func testAccCopyResource_withRetry(name string) string {
	return fmt.Sprintf(`resource "skopeo2_copy" "alpine_%s" {
    source {
	  image = "docker://alpine"
    }
    destination {
	  image = "docker://localhost:5000/alpine"
    }
    retries = 2
    retry_delay = 30
    insecure = true
}`, name)
}

func testAccCopyResource_addTag(name string) string {
	return fmt.Sprintf(`resource "skopeo2_copy" "alpine_%s" {
    source {
	  image = "docker://alpine"
    }
    destination {
	  image = "docker://localhost:5000/alpine"
    }
	additional_tags   = ["alpine:fine"]
	keep_image        = true
    insecure          = true
}`, name)
}
