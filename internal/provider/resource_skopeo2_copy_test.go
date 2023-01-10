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
	/* TODO Testing is broken!! This needs to be fixed! */
	t.Skip("Testing is broken!! This needs to be fixed!")
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
			/*
				{
					Config: testAccResourceSkopeo2,
					Check: resource.ComposeTestCheckFunc(
						resource.TestMatchResourceAttr(
							"skopeo2_copy.foo", "source_image", regexp.MustCompile("^docker:bar")),
					),
				},

			*/
		},
	})
}

func testAccCopyResource(name string) string {
	return fmt.Sprintf(`resource "skopeo2_copy" "alpine_%s" {
	source_image      = "docker://alpine"
	destination_image = "docker://ghcr.io/bsquare-corp/alpine"
}`, name)
}

func testAccCopyResource_addTag(name string) string {
	return fmt.Sprintf(`resource "skopeo2_copy" "alpine_%s" {
	source_image      = "docker://alpine"
	destination_image = "docker://ghcr.io/bsquare-corp/alpine"
	additional_tags   = ["alpine:fine"]
	keep_image        = true
}`, name)
}
