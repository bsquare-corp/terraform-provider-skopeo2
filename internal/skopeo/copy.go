package skopeo

import (
	"context"
	"fmt"
	skopeoPkg "github.com/bsquare-corp/terraform-provider-skopeo2/pkg/skopeo"
	"io"

	"github.com/containers/common/pkg/retry"
	"github.com/containers/image/v5/copy"
	"github.com/containers/image/v5/docker/reference"
	"github.com/containers/image/v5/manifest"
	"github.com/containers/image/v5/transports/alltransports"

	encconfig "github.com/containers/ocicrypt/config"
	enchelpers "github.com/containers/ocicrypt/helpers"
)

type CopyOptions struct {
	ReportWriter      io.Writer
	SrcImage          *skopeoPkg.ImageOptions
	DestImage         *skopeoPkg.ImageDestOptions
	RetryOpts         *retry.RetryOptions
	AdditionalTags    []string // For docker-archive: destinations, in addition to the name:tag specified as destination, also add these
	PreserveDigests   bool     // Fail if we cannot preserve the source digests in the destination image
	removeSignatures  bool     // Do not copy signatures from the source image
	signByFingerprint string   // Sign the image using a GPG key with the specified fingerprint
	format            string
	quiet             bool     // Suppress output information when copying images
	all               bool     // Copy all of the images if the source is a list
	encryptLayer      []int    // The list of layers to encrypt
	encryptionKeys    []string // Keys needed to encrypt the image
	decryptionKeys    []string // Keys needed to decrypt the image
}

type CopyResult struct {
	Digest string
}

func Copy(ctx context.Context, sourceImageName, destinationImageName string, opts *CopyOptions) (*CopyResult, error) {

	if err := skopeoPkg.ReexecIfNecessaryForImages(sourceImageName, destinationImageName); err != nil {
		return nil, err
	}

	insecurePolicy := false
	policyPath := ""
	policyContext, err := getPolicyContext(insecurePolicy, policyPath)
	if err != nil {
		return nil, fmt.Errorf("Error loading trust policy: %v", err)
	}
	defer policyContext.Destroy()

	srcRef, err := alltransports.ParseImageName(sourceImageName)
	if err != nil {
		return nil, fmt.Errorf("Invalid source name %s: %v", sourceImageName, err)
	}
	destRef, err := alltransports.ParseImageName(destinationImageName)
	if err != nil {
		return nil, fmt.Errorf("Invalid destination name %s: %v", destinationImageName, err)
	}

	sourceCtx, err := opts.SrcImage.NewSystemContext()
	if err != nil {
		return nil, err
	}
	destinationCtx, err := opts.DestImage.NewSystemContext()
	if err != nil {
		return nil, err
	}

	var manifestType string
	if opts.format != "" {
		manifestType, err = skopeoPkg.ParseManifestFormat(opts.format)
		if err != nil {
			return nil, err
		}
	}

	for _, image := range opts.AdditionalTags {
		ref, err := reference.ParseNormalizedNamed(image)
		if err != nil {
			return nil, fmt.Errorf("error parsing additional-tag '%s': %v", image, err)
		}
		namedTagged, isNamedTagged := ref.(reference.NamedTagged)
		if !isNamedTagged {
			return nil, fmt.Errorf("additional-tag '%s' must be a tagged reference", image)
		}
		destinationCtx.DockerArchiveAdditionalTags = append(destinationCtx.DockerArchiveAdditionalTags, namedTagged)
	}

	imageListSelection := copy.CopySystemImage
	if opts.all {
		imageListSelection = copy.CopyAllImages
	}

	if len(opts.encryptionKeys) > 0 && len(opts.decryptionKeys) > 0 {
		return nil, fmt.Errorf("--encryption-key and --decryption-key cannot be specified together")
	}

	var encLayers *[]int
	var encConfig *encconfig.EncryptConfig
	var decConfig *encconfig.DecryptConfig

	if len(opts.encryptLayer) > 0 && len(opts.encryptionKeys) == 0 {
		return nil, fmt.Errorf("--encrypt-layer can only be used with --encryption-key")
	}

	if len(opts.encryptionKeys) > 0 {
		// encryption
		p := opts.encryptLayer
		encLayers = &p
		encryptionKeys := opts.encryptionKeys
		ecc, err := enchelpers.CreateCryptoConfig(encryptionKeys, []string{})
		if err != nil {
			return nil, fmt.Errorf("Invalid encryption keys: %v", err)
		}
		cc := encconfig.CombineCryptoConfigs([]encconfig.CryptoConfig{ecc})
		encConfig = cc.EncryptConfig
	}

	if len(opts.decryptionKeys) > 0 {
		// decryption
		decryptionKeys := opts.decryptionKeys
		dcc, err := enchelpers.CreateCryptoConfig([]string{}, decryptionKeys)
		if err != nil {
			return nil, fmt.Errorf("Invalid decryption keys: %v", err)
		}
		cc := encconfig.CombineCryptoConfigs([]encconfig.CryptoConfig{dcc})
		decConfig = cc.DecryptConfig
	}
	var manifestBytes []byte
	err = retry.RetryIfNecessary(ctx, func() error {
		manifestBytes, err = copy.Image(ctx, policyContext, destRef, srcRef, &copy.Options{
			RemoveSignatures:      opts.removeSignatures,
			SignBy:                opts.signByFingerprint,
			ReportWriter:          opts.ReportWriter,
			SourceCtx:             sourceCtx,
			DestinationCtx:        destinationCtx,
			ForceManifestMIMEType: manifestType,
			ImageListSelection:    imageListSelection,
			PreserveDigests:       opts.PreserveDigests,
			OciDecryptConfig:      decConfig,
			OciEncryptLayers:      encLayers,
			OciEncryptConfig:      encConfig,
		})
		if err != nil {
			return err
		}
		return nil
	}, opts.RetryOpts)
	if err != nil {
		return nil, err
	}

	manifestDigest, err := manifest.Digest(manifestBytes)
	if err != nil {
		return nil, err
	}

	return &CopyResult{
		Digest: string(manifestDigest.String()),
	}, nil
}
