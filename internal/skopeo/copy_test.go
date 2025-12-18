package skopeo

import (
	"context"
	"fmt"
	"log"
	"os"
	"testing"

	"github.com/bsquare-corp/terraform-provider-skopeo2/internal/providerlog"
	skopeoPkg "github.com/bsquare-corp/terraform-provider-skopeo2/pkg/skopeo"
	"github.com/containers/common/pkg/retry"
)

func TestCopy(t *testing.T) {

	t.Parallel()

	reportWriter := providerlog.NewProviderLogWriter(
		log.Default().Writer(),
	)
	defer reportWriter.Close()

	writeDir := t.TempDir()
	result, err := Copy(context.TODO(), "docker://alpine:latest", fmt.Sprintf("dir:%s", writeDir), &CopyOptions{
		ReportWriter: reportWriter,
		SrcImage: &skopeoPkg.ImageOptions{
			DockerImageOptions: skopeoPkg.DockerImageOptions{
				Global: &skopeoPkg.GlobalOptions{
					Debug: true,
				},
				Shared: &skopeoPkg.SharedImageOptions{},
			},
		},
		DestImage: &skopeoPkg.ImageDestOptions{
			ImageOptions: &skopeoPkg.ImageOptions{
				DockerImageOptions: skopeoPkg.DockerImageOptions{
					Global: &skopeoPkg.GlobalOptions{
						Debug: true,
					},
					Shared: &skopeoPkg.SharedImageOptions{},
				},
			},
		},
		RetryOpts: &retry.RetryOptions{},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = os.Stat(fmt.Sprintf("%s/manifest.json", writeDir))
	if err != nil {
		files := readDir(t, writeDir)
		t.Fatalf("Expected manifest.json. Found %s. Error: %s", files, err)
	}
	if result.Digest == "" {
		t.Fatal("Digest should be empty")
	}
}

func readDir(t *testing.T, dir string) (entries []string) {
	files, err := os.ReadDir(dir)
	if err != nil {
		t.Error(err)
	}

	for _, f := range files {
		entries = append(entries, f.Name())
	}

	return
}
