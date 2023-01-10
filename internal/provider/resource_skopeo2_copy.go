package provider

import (
	"context"
	"errors"
	"fmt"
	"github.com/bsquare-corp/terraform-provider-skopeo2/internal/providerlog"
	"github.com/bsquare-corp/terraform-provider-skopeo2/internal/skopeo"
	skopeoPkg "github.com/bsquare-corp/terraform-provider-skopeo2/pkg/skopeo"
	"github.com/containers/common/pkg/retry"
	"github.com/containers/image/v5/storage"
	"github.com/containers/image/v5/transports"
	"github.com/containers/image/v5/transports/alltransports"
	"github.com/hashicorp/go-cty/cty"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"log"
	"os"
	"regexp"
	"strings"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

var (
	imageDescriptionTemplate = fmt.Sprintf(`specified as a "transport":"details" format.

Supported transports:
%s`, "`"+strings.Join(transports.ListNames(), "`, `")+"`")
)

func resourceSkopeo2Copy() *schema.Resource {
	return &schema.Resource{
		// This description is used by the documentation generator and the language server.
		Description: "Sample resource in the Terraform provider skopeo2.",

		CreateContext: resourceSkopeo2CopyCreate,
		ReadContext:   resourceSkopeo2CopyRead,
		UpdateContext: resourceSkopeo2CopyUpdate,
		DeleteContext: resourceSkopeo2CopyDelete,

		Schema: map[string]*schema.Schema{
			"source_image": {
				Description:      imageDescriptionTemplate,
				Type:             schema.TypeString,
				Required:         true,
				ValidateDiagFunc: validateSourceImage,
			},
			"destination_image": {
				Description:      imageDescriptionTemplate + ".\nWhen working with GitHub Container registry `keep_image` needs to be set to `true`.",
				Type:             schema.TypeString,
				Required:         true,
				ForceNew:         true,
				ValidateDiagFunc: validateDestinationImage,
			},
			"additional_tags": {
				Type: schema.TypeList,
				Elem: &schema.Schema{
					Type: schema.TypeString,
				},
				Optional:    true,
				Description: "additional tags (supports docker-archive)",
			},
			"keep_image": {
				Type:        schema.TypeBool,
				Optional:    true,
				Default:     false,
				Description: "keep image when Resource gets deleted. This currently needs to be set to `true` when working with GitHub Container registry.",
			},
			"preserve_digests": {
				Type:        schema.TypeBool,
				Optional:    true,
				Default:     false,
				Description: "fail if we cannot preserve the source digests in the destination image.",
			},
			"docker_digest": {
				Type:     schema.TypeString,
				Computed: true,
			},
		},
	}
}

func validateSourceImage(v interface{}, p cty.Path) diag.Diagnostics {
	sourceImageName := v.(string)
	_, err := alltransports.ParseImageName(sourceImageName)
	if err != nil {
		return diag.Errorf("Invalid source name %s: %v", sourceImageName, err)
	}

	return nil
}

func validateDestinationImage(v interface{}, p cty.Path) diag.Diagnostics {
	destinationImageName := v.(string)
	_, err := alltransports.ParseImageName(destinationImageName)
	if err != nil {
		return diag.Errorf("Invalid destination name %s: %v", destinationImageName, err)
	}

	return nil
}

var ghcr = regexp.MustCompile(`(?::\/\/)?ghcr\.io\/`)

func resourceSkopeo2CopyCreate(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {

	tflog.Trace(ctx, "created a resource")

	source := d.Get("source_image").(string)
	destination := d.Get("destination_image").(string)

	reportWriter := providerlog.NewProviderLogWriter(
		log.Default().Writer(),
	)
	defer reportWriter.Close()

	result, err := skopeo.Copy(ctx, source, destination, newCopyOptions(d, reportWriter))
	if err != nil {
		return diag.FromErr(err)
	}

	d.SetId(destination)
	return diag.FromErr(d.Set("docker_digest", result.Digest))
}

func resourceSkopeo2CopyRead(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
	destination := d.Get("destination_image").(string)

	result, err := skopeo.Inspect(ctx, destination, newInspectOptions(d))
	if err != nil {
		if errors.Is(err, storage.ErrNoSuchImage) || strings.HasSuffix(err.Error(), ": manifest unknown") {
			d.SetId("")
			return nil
		}

		return diag.FromErr(err)
	}

	return diag.FromErr(d.Set("docker_digest", result.Digest))
}

func resourceSkopeo2CopyUpdate(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
	if !d.HasChanges("additional_tags", "source_image") {
		return nil
	}

	source := d.Get("source_image").(string)
	destination := d.Get("destination_image").(string)

	reportWriter := providerlog.NewProviderLogWriter(
		log.Default().Writer(),
	)
	defer reportWriter.Close()

	result, err := skopeo.Copy(ctx, source, destination, newCopyOptions(d, reportWriter))
	if err != nil {
		return diag.FromErr(err)
	}
	return diag.FromErr(d.Set("docker_digest", result.Digest))
}

func resourceSkopeo2CopyDelete(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
	keep := d.Get("keep_image").(bool)
	if keep {
		return nil
	}

	// We need to delete
	destination := d.Get("destination_image").(string)

	if ghcr.Match([]byte(destination)) {
		return diag.Errorf("GitHub does not support deleting specific container images. Set keep_image to true.")
	}

	return diag.FromErr(skopeoPkg.Delete(ctx, destination, newDeleteOptions(d)))
}

func getStringList(d *schema.ResourceData, key string, def []string) []string {
	at := d.Get("additional_tags")
	if at == nil {
		return def
	}
	atl := at.([]interface{})
	additionalTags := make([]string, 0, len(atl))
	for _, t := range atl {
		additionalTags = append(additionalTags, t.(string))
	}
	return additionalTags
}

func newCopyOptions(d *schema.ResourceData, reportWriter *providerlog.ProviderLogWriter) *skopeo.CopyOptions {
	additionalTags := getStringList(d, "additional_tags", nil)
	preserveDigests := d.Get("preserve_digests").(bool)

	opts := &skopeo.CopyOptions{
		ReportWriter:    reportWriter,
		SrcImage:        newImageOptions(d),
		DestImage:       newImageDestOptions(d),
		RetryOpts:       newRetyOptions(),
		AdditionalTags:  additionalTags,
		PreserveDigests: preserveDigests,
	}
	return opts
}

func newDeleteOptions(d *schema.ResourceData) *skopeoPkg.DeleteOptions {
	opts := &skopeoPkg.DeleteOptions{
		Image:     newImageDestOptions(d).ImageOptions,
		RetryOpts: newRetyOptions(),
	}
	return opts
}

func newGlobalOptions() *skopeoPkg.GlobalOptions {
	opts := &skopeoPkg.GlobalOptions{}
	return opts
}

func newImageDestOptions(d *schema.ResourceData) *skopeoPkg.ImageDestOptions {
	opts := &skopeoPkg.ImageDestOptions{
		ImageOptions: &skopeoPkg.ImageOptions{
			DockerImageOptions: skopeoPkg.DockerImageOptions{
				Global:       newGlobalOptions(),
				Shared:       newSharedImageOptions(),
				AuthFilePath: os.Getenv("REGISTRY_AUTH_FILE"),
			},
		},
	}
	return opts
}

func newImageOptions(d *schema.ResourceData) *skopeoPkg.ImageOptions {
	opts := &skopeoPkg.ImageOptions{
		DockerImageOptions: skopeoPkg.DockerImageOptions{
			Global:       newGlobalOptions(),
			Shared:       newSharedImageOptions(),
			AuthFilePath: os.Getenv("REGISTRY_AUTH_FILE"),
		},
	}
	return opts
}

func newInspectOptions(d *schema.ResourceData) *skopeo.InspectOptions {
	opts := &skopeo.InspectOptions{
		Image:     newImageOptions(d),
		RetryOpts: newRetyOptions(),
	}
	return opts
}

func newRetyOptions() *retry.RetryOptions {
	opts := &retry.RetryOptions{}
	return opts
}

func newSharedImageOptions() *skopeoPkg.SharedImageOptions {
	opts := &skopeoPkg.SharedImageOptions{}
	return opts
}
