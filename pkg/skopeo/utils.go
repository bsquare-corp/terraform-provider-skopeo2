package skopeo

import (
	"context"
	"fmt"
	"strings"

	"github.com/containers/image/v5/manifest"
	"github.com/containers/image/v5/pkg/compression"
	"github.com/containers/image/v5/transports/alltransports"
	"github.com/containers/image/v5/types"
	imgspecv1 "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/pkg/errors"
)

// errorShouldDisplayUsage is a subtype of error used by command handlers to indicate that cli.ShowSubcommandHelp should be called.
type errorShouldDisplayUsage struct {
	error
}

// SharedImageOptions collects CLI flags which are image-related, but do not change across images.
// This really should be a part of globalOptions, but that would break existing users of (skopeo copy --authfile=).
type SharedImageOptions struct {
	authFilePath string // Path to a */containers/auth.json
}

// DockerImageOptions collects CLI flags specific to the "docker" transport, which are
// the same across subcommands, but may be different for each image
// (e.g. may differ between the source and destination of a copy)
type DockerImageOptions struct {
	Global         *GlobalOptions      // May be shared across several imageOptions instances.
	Shared         *SharedImageOptions // May be shared across several imageOptions instances.
	AuthFilePath   string              // Path to a */containers/auth.json (prefixed version to override shared image option).
	credsOption    string              // username[:password] for accessing a registry
	userName       string              // username for accessing a registry
	password       string              // password for accessing a registry
	registryToken  string              // token to be used directly as a Bearer token when accessing the registry
	dockerCertPath string              // A directory using Docker-like *.{crt,cert,key} files for connecting to a registry or a daemon
	noCreds        bool                // Access the registry anonymously
}

// ImageOptions collects CLI flags which are the same across subcommands, but may be different for each image
// (e.g. may differ between the source and destination of a copy)
type ImageOptions struct {
	DockerImageOptions
	sharedBlobDir    string // A directory to use for OCI blobs, shared across repositories
	dockerDaemonHost string // docker-daemon: host to connect to
}

// NewSystemContext returns a *types.SystemContext corresponding to opts.
// It is guaranteed to return a fresh instance, so it is safe to make additional updates to it.
func (opts *ImageOptions) NewSystemContext() (*types.SystemContext, error) {
	// *types.SystemContext instance from globalOptions
	//  imageOptions option overrides the instance if both are present.
	ctx := opts.Global.newSystemContext()
	ctx.DockerCertPath = opts.dockerCertPath
	ctx.OCISharedBlobDirPath = opts.sharedBlobDir
	ctx.AuthFilePath = opts.Shared.authFilePath
	ctx.DockerDaemonHost = opts.dockerDaemonHost
	ctx.DockerDaemonCertPath = opts.dockerCertPath
	if opts.DockerImageOptions.AuthFilePath != "" {
		ctx.AuthFilePath = opts.DockerImageOptions.AuthFilePath
	}
	if opts.credsOption != "" && opts.noCreds {
		return nil, errors.New("creds and no-creds cannot be specified at the same time")
	}
	if opts.userName != "" && opts.noCreds {
		return nil, errors.New("username and no-creds cannot be specified at the same time")
	}
	if opts.credsOption != "" && opts.userName != "" {
		return nil, errors.New("creds and username cannot be specified at the same time")
	}
	// if any of username or password is present, then both are expected to be present
	if opts.userName != "" || opts.password != "" {
		if opts.userName != "" {
			return nil, errors.New("password must be specified when username is specified")
		}
		return nil, errors.New("username must be specified when password is specified")
	}
	if opts.credsOption != "" {
		var err error
		ctx.DockerAuthConfig, err = getDockerAuth(opts.credsOption)
		if err != nil {
			return nil, err
		}
	} else if opts.userName != "" {
		ctx.DockerAuthConfig = &types.DockerAuthConfig{
			Username: opts.userName,
			Password: opts.password,
		}
	}
	if opts.registryToken != "" {
		ctx.DockerBearerRegistryToken = opts.registryToken
	}
	if opts.noCreds {
		ctx.DockerAuthConfig = &types.DockerAuthConfig{}
	}

	return ctx, nil
}

// ImageDestOptions is a superset of imageOptions specialized for image destinations.
type ImageDestOptions struct {
	*ImageOptions
	dirForceCompression         bool   // Compress layers when saving to the dir: transport
	dirForceDecompression       bool   // Decompress layers when saving to the dir: transport
	ociAcceptUncompressedLayers bool   // Whether to accept uncompressed layers in the oci: transport
	compressionFormat           string // Format to use for the compression
	compressionLevel            *int   // Level to use for the compression
	precomputeDigests           bool   // Precompute digests to dedup layers when saving to the docker: transport
}

// newSystemContext returns a *types.SystemContext corresponding to opts.
// It is guaranteed to return a fresh instance, so it is safe to make additional updates to it.
func (opts *ImageDestOptions) newSystemContext() (*types.SystemContext, error) {
	ctx, err := opts.ImageOptions.NewSystemContext()
	if err != nil {
		return nil, err
	}

	ctx.DirForceCompress = opts.dirForceCompression
	ctx.DirForceDecompress = opts.dirForceDecompression
	ctx.OCIAcceptUncompressedLayers = opts.ociAcceptUncompressedLayers
	if opts.compressionFormat != "" {
		cf, err := compression.AlgorithmByName(opts.compressionFormat)
		if err != nil {
			return nil, err
		}
		ctx.CompressionFormat = &cf
	}
	if opts.compressionLevel != nil {
		value := opts.compressionLevel
		ctx.CompressionLevel = value
	}
	return ctx, err
}

func parseCreds(creds string) (string, string, error) {
	if creds == "" {
		return "", "", errors.New("credentials can't be empty")
	}
	up := strings.SplitN(creds, ":", 2)
	if len(up) == 1 {
		return up[0], "", nil
	}
	if up[0] == "" {
		return "", "", errors.New("username can't be empty")
	}
	return up[0], up[1], nil
}

func getDockerAuth(creds string) (*types.DockerAuthConfig, error) {
	username, password, err := parseCreds(creds)
	if err != nil {
		return nil, err
	}
	return &types.DockerAuthConfig{
		Username: username,
		Password: password,
	}, nil
}

// ParseImageSource converts image URL-like string to an ImageSource.
// The caller must call .Close() on the returned ImageSource.
func ParseImageSource(ctx context.Context, opts *ImageOptions, name string) (types.ImageSource, error) {
	ref, err := alltransports.ParseImageName(name)
	if err != nil {
		return nil, err
	}
	sys, err := opts.NewSystemContext()
	if err != nil {
		return nil, err
	}
	return ref.NewImageSource(ctx, sys)
}

// ParseManifestFormat parses format parameter for copy and sync command.
// It returns string value to use as manifest MIME type
func ParseManifestFormat(manifestFormat string) (string, error) {
	switch manifestFormat {
	case "oci":
		return imgspecv1.MediaTypeImageManifest, nil
	case "v2s1":
		return manifest.DockerV2Schema1SignedMediaType, nil
	case "v2s2":
		return manifest.DockerV2Schema2MediaType, nil
	default:
		return "", fmt.Errorf("unknown format %q. Choose one of the supported formats: 'oci', 'v2s1', or 'v2s2'", manifestFormat)
	}
}

// usageTemplate returns the usage template for skopeo commands
// This blocks the displaying of the global options. The main skopeo
// command should not use this.
const usageTemplate = `Usage:{{if .Runnable}}
{{.UseLine}}{{end}}{{if .HasAvailableSubCommands}}

{{.CommandPath}} [command]{{end}}{{if gt (len .Aliases) 0}}

Aliases:
{{.NameAndAliases}}{{end}}{{if .HasExample}}

Examples:
{{.Example}}{{end}}{{if .HasAvailableSubCommands}}

Available Commands:{{range .Commands}}{{if (or .IsAvailableCommand (eq .Name "help"))}}
{{rpad .Name .NamePadding }} {{.Short}}{{end}}{{end}}{{end}}{{if .HasAvailableLocalFlags}}

Flags:
{{.LocalFlags.FlagUsages | trimTrailingWhitespaces}}{{end}}{{if .HasAvailableInheritedFlags}}
{{end}}
`
