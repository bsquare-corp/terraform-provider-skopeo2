package provider

import (
	"context"
	"errors"
	"fmt"
	"github.com/bsquare-corp/terraform-provider-skopeo2/internal/providerlog"
	"github.com/bsquare-corp/terraform-provider-skopeo2/internal/skopeo"
	skopeoPkg "github.com/bsquare-corp/terraform-provider-skopeo2/pkg/skopeo"
	"github.com/containers/image/v5/storage"
	"github.com/containers/image/v5/transports"
	"github.com/containers/image/v5/transports/alltransports"
	"github.com/hashicorp/go-cty/cty"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"log"
	"regexp"
	"strings"
)

var (
	imageDescriptionTemplate = fmt.Sprintf(`specified as a "transport":"details" format.

Supported transports:
%s`, "`"+strings.Join(transports.ListNames(), "`, `")+"`")
)

func resourceSkopeo2Copy() *schema.Resource {
	validateImageFunc := func(v interface{}, p cty.Path) diag.Diagnostics {
		imageName := v.(string)
		_, err := alltransports.ParseImageName(imageName)
		if err != nil {
			return diag.Errorf("Invalid image name %s: %v", imageName, err)
		}
		return nil
	}

	return &schema.Resource{
		// This description is used by the documentation generator and the language server.
		Description: "Copy resource in the Terraform provider skopeo2.",

		CreateContext: resourceSkopeo2CopyCreate,
		ReadContext:   resourceSkopeo2CopyRead,
		UpdateContext: resourceSkopeo2CopyUpdate,
		DeleteContext: resourceSkopeo2CopyDelete,

		Schema: map[string]*schema.Schema{
			"source": {
				Type:        schema.TypeList,
				Optional:    true,
				MaxItems:    1,
				Description: "Source image location/access credentials. Overrides provider configuration.",
				Elem: &schema.Resource{
					Schema: func() map[string]*schema.Schema {
						swSchema := SomewhereSchema("source", true)
						// Add the "image" param
						swSchema["image"] = &schema.Schema{
							Type:             schema.TypeString,
							Optional:         true,
							Description:      imageDescriptionTemplate,
							ValidateDiagFunc: validateImageFunc,
						}
						return swSchema
					}(),
				},
				Deprecated: "Configure the source block at the Provider Configuration level and the" +
					" source_image instead. This attribute will be removed in the next major version of the provider.",
			},
			"source_image": {
				Type:             schema.TypeString,
				Optional:         true,
				Description:      imageDescriptionTemplate,
				ValidateDiagFunc: validateImageFunc,
			},
			"destination": {
				Type:        schema.TypeList,
				Optional:    true,
				MaxItems:    1,
				ForceNew:    true,
				Description: "Destination image location/access credentials, Overrides provider configuration.",
				Elem: &schema.Resource{
					Schema: func() map[string]*schema.Schema {
						swSchema := SomewhereSchema("destination", true)
						// Add the "image" param
						swSchema["image"] = &schema.Schema{
							Type:             schema.TypeString,
							Optional:         true,
							Description:      imageDescriptionTemplate + "\nWhen working with GitHub Container registry `keep_image` needs to be set to `true`.",
							ValidateDiagFunc: validateImageFunc,
						}
						return swSchema
					}(),
				},
				Deprecated: "Configure the destination block at the Provider Configuration level and the" +
					" destination_image instead. This attribute will be removed in the next major version of the provider.",
			},
			"destination_image": {
				Type:             schema.TypeString,
				Optional:         true,
				Description:      imageDescriptionTemplate + "\nWhen working with GitHub Container registry `keep_image` needs to be set to `true`.",
				ValidateDiagFunc: validateImageFunc,
			},
			"retries": {
				Type:     schema.TypeInt,
				Optional: true,
				Default:  "0",
				Description: "Retry the copy operation following transient failure. " +
					"Retrying following access failure error is configured through login_retries in the provider" +
					" configuration.",
			},
			"retry_delay": {
				Type:        schema.TypeInt,
				Optional:    true,
				Default:     "0",
				Description: "Delay between retry attempts, in seconds. ",
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
				Type:     schema.TypeBool,
				Optional: true,
				Default:  false,
				ForceNew: true,
				Description: "fail if we cannot preserve the source digests in the destination image and" +
					" automatically detect when the source has a different digest to the destination",
			},
			"insecure": {
				Type:        schema.TypeBool,
				Optional:    true,
				Default:     false,
				Description: "allow access to non-TLS insecure repositories.",
			},
			"copy_all_images": {
				Type:     schema.TypeBool,
				Optional: true,
				Default:  false,
				Description: "indicates that the caller expects to copy all images from a multiple image manifest, " +
					"otherwise only one image matching the system arch/platform is copied",
			},
			"docker_digest": {
				Type:        schema.TypeString,
				Optional:    true,
				Computed:    true,
				Description: "digest string for the destination image.",
			},
		},
	}
}

var ghcr = regexp.MustCompile(`^\w+:\/\/ghcr\.io\/|^ghcr\.io\/`)

func IsGhcr(image *string) bool {
	return ghcr.Match([]byte(*image))
}

func getSomewhereParamsOverriding(d *schema.ResourceData, key string, other *somewhere) (*somewhere, error) {
	sw, err := GetSomewhereParams(d, key)
	if err != nil {
		return nil, err
	}

	// Old location inside block
	if attr, ok := d.GetOk(key + ".0.image"); ok {
		sw.SetImage(attr.(string))
	}

	if attr, ok := d.GetOk(key + "_image"); ok {
		sw.SetImage(attr.(string))
	}

	// Apply provider block configuration
	sw.Overriding(other)

	if !sw.HasImage() {
		return nil, fmt.Errorf("No " + key + " image specified in any of the alternative locations.")
	}

	return sw, nil
}

func resourceSkopeo2CopyCreate(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {

	tflog.Debug(ctx, "Creating a resource")

	config := meta.(*PConfig)

	src, err := getSomewhereParamsOverriding(d, "source", config.source)
	if err != nil {
		return diag.FromErr(err)
	}

	dst, err := getSomewhereParamsOverriding(d, "destination", config.destination)
	if err != nil {
		return diag.FromErr(err)
	}

	reportWriter := providerlog.NewProviderLogWriter(
		log.Default().Writer(),
	)
	defer reportWriter.Close()

	src.loginRetriesRemaining = src.loginRetries + 1
	dst.loginRetriesRemaining = dst.loginRetries + 1
	for {
		result, err := src.WithEndpointLogin(ctx, newImageOptions(d), false, func(locked bool) (any, error) {
			return dst.WithEndpointLogin(ctx, newImageDestOptions(d).ImageOptions, locked, func(_ bool) (any, error) {
				tflog.Debug(ctx, "Copying", map[string]any{"src-image": src.image, "image": dst.image})
				result, err := skopeo.Copy(ctx, src.image, dst.image, newCopyOptions(d, reportWriter))
				if err != nil {
					tflog.Info(ctx, "Copy failed", map[string]any{"src-image": src.image, "image": dst.image, "err": err})
					return nil, err
				}

				return result, nil
			})
		})

		if err == nil {
			d.SetId(dst.image)
			digest := result.(*skopeo.CopyResult).Digest
			tflog.Info(ctx, "Copied", map[string]any{"src-image": src.image, "image": dst.image, "digest": digest})
			return diag.FromErr(d.Set("docker_digest", digest))
		}

		tflog.Info(ctx, "Retries remaining", map[string]any{"source_count": src.loginRetriesRemaining,
			"destination_count": dst.loginRetriesRemaining})
		if src.loginRetriesRemaining <= 0 || dst.loginRetriesRemaining <= 0 {
			return diag.FromErr(err)
		}
	}
}

func loginInspect(ctx context.Context, d *schema.ResourceData, sw *somewhere) (any, error) {
	return sw.WithEndpointLogin(ctx, newImageOptions(d), false,
		func(_ bool) (any, error) {
			tflog.Debug(ctx, "Inspecting", map[string]any{"image": sw.image})
			result, err := skopeo.Inspect(ctx, sw.image, newInspectOptions(d))
			if err != nil {
				tflog.Info(ctx, "Inspection failed", map[string]any{"image": sw.image, "err": err.Error()})
				//The underlying storage code does not reveal the 404 response code on a missing image.
				//The only indication that an image is missing is the presence of "manifest unknown" in the error
				//reported. This comes from the body of the 404 response and is therefore subject to the whim of the
				//registry implementation.
				//Azure ACR for example reports:
				//"manifest unknown: manifest tagged by "X.Y.Z" is not found"
				//where as AWS ECR reports:
				//"manifest unknown"
				//This code is fragile but changes to the underlying library would be needed to improve on it.
				if errors.Is(err, storage.ErrNoSuchImage) || strings.Contains(err.Error(),
					"manifest unknown") {
					return nil, nil
				}
				return nil, err
			}

			return result, nil
		})
}

func resourceSkopeo2CopyRead(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
	config := meta.(*PConfig)
	dst, err := getSomewhereParamsOverriding(d, "destination", config.destination)
	if err != nil {
		return diag.FromErr(err)
	}

	dst.loginRetriesRemaining = dst.loginRetries + 1

	for {
		result, err := loginInspect(ctx, d, dst)

		if err == nil {
			if result == nil {
				// Destination image does not exist
				d.SetId("")
				return nil
			}
			// if we are detecting changes to the source image break out here and continue on to inspecting the
			// source image
			if d.Get("preserve_digests").(bool) {
				break
			}
			dstDigest := result.(*skopeo.InspectOutput).Digest
			tflog.Info(ctx, "Inspection", map[string]any{"image": dst.image, "digest": dstDigest})
			return diag.FromErr(d.Set("docker_digest", dstDigest))
		}

		tflog.Info(ctx, "Retries remaining", map[string]any{"count": dst.loginRetriesRemaining})
		if dst.loginRetriesRemaining <= 0 {
			// If we get an error the problem may be because the login script has changed, swallow the error and
			// report the resource as deleted forcing the create copy operation.
			tflog.Warn(ctx, "Login errors during refresh, plan to recreate", map[string]any{"error": err.Error()})
			d.SetId("")
			return nil
		}
	}

	src, err := getSomewhereParamsOverriding(d, "source", config.source)
	if err != nil {
		return diag.FromErr(err)
	}

	src.loginRetriesRemaining = src.loginRetries + 1

	for {
		result, err := loginInspect(ctx, d, src)

		if err == nil {
			if result == nil {
				// Source image does not exist
				d.SetId("")
				return nil
			}

			srcDigest := result.(*skopeo.InspectOutput).Digest
			tflog.Info(ctx, "Inspection", map[string]any{"image": src.image, "digest": srcDigest})
			return diag.FromErr(d.Set("docker_digest", srcDigest))
		}

		tflog.Info(ctx, "Retries remaining", map[string]any{"count": src.loginRetriesRemaining})
		if src.loginRetriesRemaining <= 0 {
			// If we get an error the problem may be because the login script has changed, swallow the error and
			// report the resource as deleted forcing the create copy operation.
			tflog.Warn(ctx, "Login errors during refresh, plan to recreate", map[string]any{"error": err.Error()})
			d.SetId("")
			return nil
		}
	}
}

func resourceSkopeo2CopyUpdate(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
	return resourceSkopeo2CopyCreate(ctx, d, meta)
}

func resourceSkopeo2CopyDelete(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
	config := meta.(*PConfig)

	keep := d.Get("keep_image").(bool)
	if keep {
		return nil
	}

	// We need to delete
	dst, err := getSomewhereParamsOverriding(d, "destination", config.destination)
	if err != nil {
		return diag.FromErr(err)
	}

	if IsGhcr(&dst.image) {
		return diag.Errorf("GitHub does not support deleting specific container images. Set keep_image to true.")
	}

	for {
		_, err := dst.WithEndpointLogin(ctx, newImageOptions(d), false, func(_ bool) (any, error) {
			tflog.Debug(ctx, "Deleting", map[string]any{"image": dst.image})
			err := skopeoPkg.Delete(ctx, dst.image, newDeleteOptions(d))
			if err != nil {
				tflog.Info(ctx, "Delete fail", map[string]any{"image": dst.image, "err": err})
				return nil, err
			}
			return nil, nil
		})
		if dst.loginRetriesRemaining <= 0 {
			return diag.FromErr(err)
		}
	}
}
