package skopeo

import (
	"context"
	"testing"

	skopeoPkg "github.com/bsquare-corp/terraform-provider-skopeo2/pkg/skopeo"
	"github.com/containers/common/pkg/retry"
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
		RetryOpts: &retry.RetryOptions{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.Digest == "" {
		t.Fatal("Digest not expected")
	}
}
