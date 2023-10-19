package provider

import (
	"github.com/bsquare-corp/terraform-provider-skopeo2/internal/providerlog"
	"github.com/bsquare-corp/terraform-provider-skopeo2/internal/skopeo"
	skopeoPkg "github.com/bsquare-corp/terraform-provider-skopeo2/pkg/skopeo"
	"github.com/containers/common/pkg/retry"
	"time"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func getStringList(d *schema.ResourceData, key string, def []string) []string {
	at := d.Get(key)
	if at == nil {
		return def
	}
	atl := at.([]interface{})
	stringList := make([]string, 0, len(atl))
	for _, t := range atl {
		stringList = append(stringList, t.(string))
	}
	return stringList
}

func newCopyOptions(d *schema.ResourceData, reportWriter *providerlog.ProviderLogWriter, src, dst *somewhere) *skopeo.CopyOptions {
	additionalTags := getStringList(d, "additional_tags", nil)
	preserveDigests := d.Get("preserve_digests").(bool)

	opts := &skopeo.CopyOptions{
		ReportWriter:    reportWriter,
		SrcImage:        newImageOptions(d, src),
		DestImage:       newImageDestOptions(d, dst),
		RetryOpts:       newRetryOptions(d),
		AdditionalTags:  additionalTags,
		PreserveDigests: preserveDigests,
		All:             d.Get("copy_all_images").(bool),
	}
	return opts
}

func newDeleteOptions(d *schema.ResourceData, dst *somewhere) *skopeoPkg.DeleteOptions {
	opts := &skopeoPkg.DeleteOptions{
		Image:     newImageDestOptions(d, dst).ImageOptions,
		RetryOpts: newRetryOptions(d),
	}
	return opts
}

func newGlobalOptions() *skopeoPkg.GlobalOptions {
	opts := &skopeoPkg.GlobalOptions{}
	return opts
}

func newImageDestOptions(d *schema.ResourceData, sw *somewhere) *skopeoPkg.ImageDestOptions {
	opts := &skopeoPkg.ImageDestOptions{
		ImageOptions: newImageOptions(d, sw),
	}
	return opts
}

func newImageOptions(d *schema.ResourceData, sw *somewhere) *skopeoPkg.ImageOptions {
	opts := &skopeoPkg.ImageOptions{
		DockerImageOptions: skopeoPkg.DockerImageOptions{
			Global:         newGlobalOptions(),
			Shared:         newSharedImageOptions(),
			Insecure:       d.Get("insecure").(bool),
			AuthFilePath:   sw.registryAuthFile,
			DockerCertPath: sw.certificateDirectory,
		},
	}
	return opts
}

func newInspectOptions(d *schema.ResourceData, sw *somewhere) *skopeo.InspectOptions {
	opts := &skopeo.InspectOptions{
		Image:     newImageOptions(d, sw),
		RetryOpts: newRetryOptions(d),
	}
	return opts
}

func newRetryOptions(d *schema.ResourceData) *retry.RetryOptions {
	opts := &retry.RetryOptions{
		MaxRetry: d.Get("retries").(int),
		Delay:    time.Duration(d.Get("retry_delay").(int)) * time.Second,
	}
	return opts
}

func newSharedImageOptions() *skopeoPkg.SharedImageOptions {
	opts := &skopeoPkg.SharedImageOptions{}
	return opts
}

func newLoginOptions(d *schema.ResourceData, sw *somewhere, reportWriter *providerlog.ProviderLogWriter, password string) *skopeo.LoginOptions {
	opts := &skopeo.LoginOptions{
		Image:    newImageOptions(d, sw),
		Username: sw.loginUsername,
		Password: password,
		CertPath: sw.certificateDirectory,
		AuthFile: sw.registryAuthFile,
		Stdout:   reportWriter,
	}
	return opts
}
