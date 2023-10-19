package skopeo

import (
	"context"
	"fmt"
	skopeoPkg "github.com/bsquare-corp/terraform-provider-skopeo2/pkg/skopeo"
	"github.com/containers/common/pkg/auth"
	"github.com/containers/image/v5/docker/reference"
	"github.com/containers/image/v5/transports/alltransports"
	"io"
)

type LoginOptions struct {
	Image              *skopeoPkg.ImageOptions
	Username, Password string
	Stdout             io.Writer
	AuthFile           string
	CertPath           string
}

func Login(ctx context.Context, imageName string, opts *LoginOptions) error {

	ref, err := alltransports.ParseImageName(imageName)
	if err != nil {
		return fmt.Errorf("Invalid image name %s: %v", imageName, err)
	}
	registryDomain := reference.Domain(ref.DockerReference())

	sysCtx, err := opts.Image.NewSystemContext()
	if err != nil {
		return err
	}

	var authFile string
	if opts.AuthFile != "" {
		authFile = opts.AuthFile
	} else {
		authFile = auth.GetDefaultAuthFile()
	}

	return auth.Login(ctx, sysCtx,
		&auth.LoginOptions{
			AuthFile: authFile,
			CertDir:  opts.CertPath,
			Username: opts.Username,
			Password: opts.Password,
			Stdout:   opts.Stdout,
		},
		[]string{registryDomain})
}
