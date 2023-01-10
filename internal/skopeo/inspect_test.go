package skopeo

import (
	"context"
	"testing"

	skopeoPkg "github.com/bsquare-corp/terraform-provider-skopeo/pkg/skopeo"
)

func TestInspect(t *testing.T) {
	t.Parallel()

	out, err := Inspect(context.TODO(), "docker://alpine:latest", &InspectOptions{
		Image: &skopeoPkg.ImageOptions{
			DockerImageOptions: skopeoPkg.DockerImageOptions{
				Global: &skopeoPkg.GlobalOptions{
					Debug: true,
				},
				Shared: &skopeoPkg.SharedImageOptions{},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.Digest == "" {
		t.Fatal("Digest not expected")
	}
}
