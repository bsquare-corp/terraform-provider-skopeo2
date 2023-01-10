package skopeo

import (
	"context"
	"fmt"

	skopeoPkg "github.com/bsquare-corp/terraform-provider-skopeo2/pkg/skopeo"
	"github.com/containers/common/pkg/retry"
	"github.com/containers/image/v5/manifest"
	"github.com/containers/image/v5/types"
	"github.com/opencontainers/go-digest"
	"github.com/pkg/errors"
)

type InspectOptions struct {
	Image         *skopeoPkg.ImageOptions
	RetryOpts     *retry.RetryOptions
	format        string
	raw           bool // Output the raw manifest instead of parsing information about the image
	config        bool // Output the raw config blob instead of parsing information about the image
	doNotListTags bool // Do not list all tags available in the same repository
}

type InspectOutput struct {
	Digest digest.Digest
}

func Inspect(ctx context.Context, imageName string, opts *InspectOptions) (out *InspectOutput, retErr error) {
	var err error
	var src types.ImageSource
	if err := retry.RetryIfNecessary(ctx, func() error {
		src, err = skopeoPkg.ParseImageSource(ctx, opts.Image, imageName)
		return err
	}, opts.RetryOpts); err != nil {
		return nil, errors.Wrapf(err, "Error parsing image name %q", imageName)
	}

	defer func() {
		if err := src.Close(); err != nil {
			retErr = errors.Wrapf(retErr, fmt.Sprintf("(could not close image: %v) ", err))
		}
	}()

	var rawManifest []byte
	if err := retry.RetryIfNecessary(ctx, func() error {
		rawManifest, _, err = src.GetManifest(ctx, nil)
		return err
	}, opts.RetryOpts); err != nil {
		return nil, errors.Wrapf(err, "Error retrieving manifest for image")
	}

	digest, err := manifest.Digest(rawManifest)
	if err != nil {
		return nil, errors.Wrapf(err, "Error computing manifest digest")
	}
	return &InspectOutput{
		Digest: digest,
	}, nil
}
