package provider

import (
	"context"
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
)

func TestAccDataSourceSkopeo2InspectLogin(t *testing.T) {

	resource.UnitTest(t, resource.TestCase{
		PreCheck: func() {
			copyTestImageToSource(t)
		},
		ProviderFactories: providerFactories,
		Steps: []resource.TestStep{
			{
				Config: `
data "skopeo2_inspect" "foo" {
	source_image = "` + testSrcImage + `"
    insecure = true
}
`,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttrSet("data.skopeo2_inspect.foo", "name"),
				),
			},
			{
				Config: `
provider "skopeo2" {
	source {
		login_username = "testuser"
		login_password = "testpassword"
	}
}

data "skopeo2_inspect" "foo" {
	source_image = "` + testSrcImageWithAuth + `"
    insecure = true
}
`,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttrSet("data.skopeo2_inspect.foo", "name"),
				),
			},
		},
	})
}

func TestAccDataSourceSkopeo2InspectData(t *testing.T) {

	buildAndPushImage := func(imageData string) string {
		_, err := imageBuild(newDockerCli(context.Background()),
			"inspect-testimage", imageData)
		if err != nil {
			t.Logf("err: %s", err)
		}

		aux, err := imagePush(newDockerCli(context.Background()), "inspect-testimage")
		if err != nil {
			t.Logf("err: %s", err)
		}
		if aux != nil {
			return aux.Digest
		}
		return ""
	}

	digest := ""
	resource.UnitTest(t, resource.TestCase{
		PreCheck: func() {
			digest = buildAndPushImage("an image to inspect")
		},
		ProviderFactories: providerFactories,
		Steps: []resource.TestStep{
			{
				Config: `
data "skopeo2_inspect" "data-test" {
	source_image = "docker://127.0.0.1:9016/inspect-testimage:latest"
    insecure = true
}
`,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttrWith("data.skopeo2_inspect.data-test",
						"source_digest", func(value string) error {
							if digest != value {
								return fmt.Errorf("source_digest incorrect: %s, should be %s", value, digest)
							}
							return nil
						},
					),
					resource.TestCheckResourceAttr("data.skopeo2_inspect.data-test", "os", "linux"),
					resource.TestCheckResourceAttr("data.skopeo2_inspect.data-test", "repo_tags.0", "latest"),
				),
			},
		},
	})
}
