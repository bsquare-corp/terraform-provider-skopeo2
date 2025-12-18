package provider

import (
	"context"
	"time"

	"github.com/bsquare-corp/terraform-provider-skopeo2/internal/skopeo"
	"github.com/containers/image/v5/transports/alltransports"
	"github.com/hashicorp/go-cty/cty"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func dataSourceSkopeo2Inspect() *schema.Resource {
	validateImageFunc := func(v interface{}, p cty.Path) diag.Diagnostics {
		imageName := v.(string)
		_, err := alltransports.ParseImageName(imageName)
		if err != nil {
			return diag.Errorf("Invalid image name %s: %v", imageName, err)
		}
		return nil
	}

	return &schema.Resource{
		Description: "Inspect resource in the Terraform provider skopeo2.",

		ReadContext: dataSourceSkopeo2InspectRead,

		Schema: map[string]*schema.Schema{
			"source_image": {
				Type:             schema.TypeString,
				Required:         true,
				Description:      imageDescriptionTemplate,
				ValidateDiagFunc: validateImageFunc,
			},
			"insecure": {
				Type:        schema.TypeBool,
				Optional:    true,
				Default:     false,
				Description: "allow access to non-TLS insecure repositories.",
			},
			"retries": {
				Type:     schema.TypeInt,
				Optional: true,
				Default:  "0",
				Description: "Retry the inspect operation following transient failure. " +
					"Retrying following access failure error is configured through login_retries in the provider" +
					" configuration.",
			},
			"retry_delay": {
				Type:        schema.TypeInt,
				Optional:    true,
				Default:     "0",
				Description: "Delay between retry attempts, in seconds. ",
			},
			"name": {
				Type:        schema.TypeString,
				Computed:    true,
				Description: "Fully qualified image name.",
			},
			"source_digest": {
				Type:        schema.TypeString,
				Computed:    true,
				Description: "Image manifest digest.",
			},
			"repo_tags": {
				Type:        schema.TypeList,
				Computed:    true,
				Description: "List of repository tags associated with the image.",
				Elem: &schema.Schema{
					Type: schema.TypeString,
				},
			},
			"created": {
				Type:        schema.TypeString,
				Computed:    true,
				Description: "Image creation timestamp (RFC3339).",
			},
			"docker_version": {
				Type:        schema.TypeString,
				Computed:    true,
				Description: "Docker version used to build the image.",
			},
			"labels": {
				Type:        schema.TypeMap,
				Computed:    true,
				Description: "Image labels.",
				Elem: &schema.Schema{
					Type: schema.TypeString,
				},
			},
			"architecture": {
				Type:        schema.TypeString,
				Computed:    true,
				Description: "CPU architecture of the image.",
			},
			"os": {
				Type:        schema.TypeString,
				Computed:    true,
				Description: "Operating system of the image.",
			},
			"layers": {
				Type:        schema.TypeList,
				Computed:    true,
				Description: "List of layer digests.",
				Elem: &schema.Schema{
					Type: schema.TypeString,
				},
			},
			"layers_data": {
				Type:        schema.TypeList,
				Computed:    true,
				Description: "Detailed metadata for each image layer.",
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"mime_type": {
							Type:        schema.TypeString,
							Computed:    true,
							Description: "MIME type of the layer.",
						},
						"digest": {
							Type:        schema.TypeString,
							Computed:    true,
							Description: "Digest of the layer.",
						},
						"size": {
							Type:        schema.TypeInt,
							Computed:    true,
							Description: "Size of the layer in bytes.",
						},
						"annotations": {
							Type:        schema.TypeMap,
							Computed:    true,
							Optional:    true,
							Description: "Optional annotations associated with the layer.",
							Elem: &schema.Schema{
								Type: schema.TypeString,
							},
						},
					},
				},
			},
			"env": {
				Type:        schema.TypeList,
				Computed:    true,
				Description: "Environment variables defined in the image.",
				Elem: &schema.Schema{
					Type: schema.TypeString,
				},
			},
		},

		Timeouts: &schema.ResourceTimeout{
			Read: schema.DefaultTimeout(20 * time.Minute),
		},
	}
}

func dataSourceSkopeo2InspectRead(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
	config := meta.(*PConfig)
	var diagnosticsOut []diag.Diagnostic

	src, err := getSomewhereParamsOverriding(d, "source", config.source)
	if err != nil {
		return append(diagnosticsOut, diag.FromErr(err)...)
	}

	src.loginRetriesRemaining = src.loginRetries + 1

	for {
		result, err := loginInspect(ctx, d, src)
		if err != nil {
			diagnosticsOut = append(diagnosticsOut, diag.FromErr(err)...)
		} else if result != nil {
			d.SetId(src.image)

			srcDigest := result.Digest
			tflog.Info(ctx, "Inspection", map[string]any{"image": src.image, "digest": srcDigest})
			diagnosticsOut = append(diagnosticsOut, diag.FromErr(setStateFromInspection(d, result))...)
			break
		}

		tflog.Info(ctx, "Retries remaining", map[string]any{"count": src.loginRetriesRemaining})
		if src.loginRetriesRemaining <= 0 {
			// If we get an error the problem may be because the login script has changed, swallow the error and
			// report the resource as deleted forcing the create copy operation.
			tflog.Warn(ctx, "Login errors during read", map[string]any{"error": err.Error()})
			d.SetId("")
			return append(diagnosticsOut, diag.Errorf("Exhausted %d source login/retries", src.loginRetries)...)
		}
	}

	return diagnosticsOut
}

func setStateFromInspection(
	d *schema.ResourceData,
	inspect *skopeo.InspectOutput,
) error {
	if err := d.Set("name", inspect.Name); err != nil {
		return err
	}
	if err := d.Set("source_digest", inspect.Digest); err != nil {
		return err
	}
	if err := d.Set("repo_tags", inspect.RepoTags); err != nil {
		return err
	}
	if !inspect.Created.IsZero() {
		if err := d.Set("created", inspect.Created.Format(time.RFC3339Nano)); err != nil {
			return err
		}
	}
	if err := d.Set("docker_version", inspect.DockerVersion); err != nil {
		return err
	}
	if err := d.Set("labels", inspect.Labels); err != nil {
		return err
	}
	if err := d.Set("architecture", inspect.Architecture); err != nil {
		return err
	}
	if err := d.Set("os", inspect.Os); err != nil {
		return err
	}
	if err := d.Set("layers", inspect.Layers); err != nil {
		return err
	}
	if err := d.Set("env", inspect.Env); err != nil {
		return err
	}
	if inspect.LayersData != nil {
		layersData := make([]map[string]interface{}, 0, len(inspect.LayersData))
		for _, layer := range inspect.LayersData {
			layerMap := map[string]interface{}{
				"mime_type": layer.MIMEType,
				"digest":    layer.Digest,
				"size":      int(layer.Size),
			}
			// annotations may be nil
			if layer.Annotations != nil {
				layerMap["annotations"] = layer.Annotations
			}
			layersData = append(layersData, layerMap)
		}
		if err := d.Set("layers_data", layersData); err != nil {
			return err
		}
	}
	return nil
}
