package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func init() {
	// Set descriptions to support markdown syntax, this will be used in document generation
	// and the language server.
	schema.DescriptionKind = schema.StringMarkdown
}

func New(version string) func() *schema.Provider {
	return func() *schema.Provider {
		p := &schema.Provider{
			DataSourcesMap: map[string]*schema.Resource{
				"skopeo2_inspect": dataSourceSkopeo2Inspect(),
			},
			ResourcesMap: map[string]*schema.Resource{
				"skopeo2_copy": resourceSkopeo2Copy(),
			},
			Schema: map[string]*schema.Schema{
				"source": {
					Type:        schema.TypeList,
					Optional:    true,
					MaxItems:    1,
					Description: "Source image access credentials",
					Elem:        &schema.Resource{Schema: SomewhereSchema("source", true)},
				},
				"destination": {
					Type:        schema.TypeList,
					Optional:    true,
					MaxItems:    1,
					ForceNew:    true,
					Description: "Destination image access credentials",
					Elem:        &schema.Resource{Schema: SomewhereSchema("destination", true)},
				},
			},
		}

		p.ConfigureContextFunc = configure(version, p)

		return p
	}
}

type PConfig struct {
	// Source/dest params can be overridden in the copy resource
	source, destination *somewhere
}

func configure(version string, p *schema.Provider) func(context.Context, *schema.ResourceData) (any, diag.Diagnostics) {
	return func(ctx context.Context, d *schema.ResourceData) (any, diag.Diagnostics) {
		src, err := GetSomewhereParams(d, "source")
		if err != nil {
			return nil, diag.FromErr(err)
		}

		dst, err := GetSomewhereParams(d, "destination")
		if err != nil {
			return nil, diag.FromErr(err)
		}

		return &PConfig{
			source:      src,
			destination: dst,
		}, nil
	}
}
