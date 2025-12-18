package skopeo

import (
	"context"
	"fmt"
	"time"

	skopeoPkg "github.com/bsquare-corp/terraform-provider-skopeo2/pkg/skopeo"
	"github.com/containers/common/pkg/retry"
	"github.com/containers/image/v5/docker"
	"github.com/containers/image/v5/image"
	"github.com/containers/image/v5/manifest"
	"github.com/containers/image/v5/types"
	"github.com/docker/distribution/registry/api/errcode"
	"github.com/opencontainers/go-digest"
	"github.com/pkg/errors"
)

type InspectOptions struct {
	Image     *skopeoPkg.ImageOptions
	RetryOpts *retry.RetryOptions
}

type InspectOutput struct {
	Name          string `json:",omitempty"`
	Tag           string `json:",omitempty"`
	Digest        digest.Digest
	RepoTags      []string
	Created       *time.Time
	DockerVersion string
	Labels        map[string]string
	Architecture  string
	Os            string
	Layers        []string
	LayersData    []types.ImageInspectLayer
	Env           []string
}

func Inspect(ctx context.Context, imageName string, opts *InspectOptions) (out *InspectOutput, retErr error) {
	var err error
	var src types.ImageSource
	var imgInspect *types.ImageInspectInfo
	var repoTags []string

	sysCtx, err := opts.Image.NewSystemContext()
	if err != nil {
		return nil, err
	}

	if err := retry.IfNecessary(ctx, func() error {
		src, err = skopeoPkg.ParseImageSource(ctx, opts.Image, imageName)
		return err
	}, opts.RetryOpts); err != nil {
		return nil, errors.Wrapf(err, "error parsing image name %q", imageName)
	}

	defer func() {
		if err := src.Close(); err != nil {
			retErr = errors.Wrapf(retErr, "could not close image")
		}
	}()

	var rawManifest []byte
	if err := retry.IfNecessary(ctx, func() error {
		rawManifest, _, err = src.GetManifest(ctx, nil)
		return err
	}, opts.RetryOpts); err != nil {
		return nil, errors.Wrapf(err, "error retrieving manifest for image")
	}

	digest, err := manifest.Digest(rawManifest)
	if err != nil {
		return nil, errors.Wrapf(err, "error computing manifest digest")
	}

	img, err := image.FromUnparsedImage(ctx, sysCtx, image.UnparsedInstance(src, nil))
	if err != nil {
		return nil, fmt.Errorf("error parsing manifest for image: %w", err)
	}

	if err := retry.IfNecessary(ctx, func() error {
		imgInspect, err = img.Inspect(ctx)
		return err
	}, opts.RetryOpts); err != nil {
		return nil, err
	}

	refName := ""
	if dockerRef := img.Reference().DockerReference(); dockerRef != nil {
		refName = dockerRef.Name()
	}

	if img.Reference().Transport() == docker.Transport {
		repoTags, err = docker.GetRepositoryTags(ctx, sysCtx, img.Reference())
		if err != nil {
			// Some registries may decide to block the "list all tags" endpoint;
			// gracefully allow the inspection to continue in this case:
			fatalFailure := true
			// - AWS ECR rejects it if the "ecr:ListImages" action is not allowed.
			//   https://github.com/containers/skopeo/issues/726
			var ec errcode.ErrorCoder
			if ok := errors.As(err, &ec); ok && errors.Is(ec.ErrorCode(), errcode.ErrorCodeDenied) {
				fatalFailure = false
			}
			// - public.ecr.aws does not implement the endpoint at all, and fails with 404:
			//   https://github.com/containers/skopeo/issues/1230
			//   This is actually "code":"NOT_FOUND", and the parser doesn’t preserve that.
			//   So, also check the error text.
			if ok := errors.As(err, &ec); ok && errors.Is(ec.ErrorCode(), errcode.ErrorCodeUnknown) {
				var e errcode.Error
				if ok := errors.As(err, &e); ok && errors.Is(e.Code, errcode.ErrorCodeUnknown) && e.Message == "404 page not found" {
					fatalFailure = false
				}
			}
			if fatalFailure {
				return nil, fmt.Errorf("error determining repository tags: %w", err)
			}
			fmt.Println("Registry disallows tag list retrieval; skipping")
		}
	} else {
		fmt.Printf("Tag list only available for %s, not %s transport\n",
			docker.Transport.Name(), img.Reference().Transport().Name())
	}

	return &InspectOutput{
		Name:          refName,
		Tag:           imgInspect.Tag,
		Digest:        digest,
		RepoTags:      repoTags,
		Created:       imgInspect.Created,
		DockerVersion: imgInspect.DockerVersion,
		Labels:        imgInspect.Labels,
		Architecture:  imgInspect.Architecture,
		Os:            imgInspect.Os,
		Layers:        imgInspect.Layers,
		LayersData:    imgInspect.LayersData,
		Env:           imgInspect.Env,
	}, nil
}
