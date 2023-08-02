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
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

var (
	imageDescriptionTemplate = fmt.Sprintf(`specified as a "transport":"details" format.

Supported transports:
%s`, "`"+strings.Join(transports.ListNames(), "`, `")+"`")
	loginInProgress sync.Mutex
)

func subRes(prefix, res string) string {
	return prefix + ".0." + res
}

func subResArray(prefix string, refs ...string) []string {
	out := make([]string, len(refs))
	for i := range refs {
		out[i] = subRes(prefix, refs[i])
	}
	return out
}

// somewhere can be the source or destination
func somewhereResource(parent string, imageDescription string, imageValidator schema.SchemaValidateDiagFunc) *schema.
	Resource {
	return &schema.Resource{
		Schema: map[string]*schema.Schema{
			"image": {
				Type:             schema.TypeString,
				Required:         true,
				Description:      imageDescription,
				ValidateDiagFunc: imageValidator,
			},
			"login_username": {
				Type:          schema.TypeString,
				Optional:      true,
				Description:   "Registry login username",
				ConflictsWith: subResArray(parent, "login_script"),
			},
			"login_password": {
				Type:          schema.TypeString,
				Optional:      true,
				Description:   "Registry login password",
				ConflictsWith: subResArray(parent, "login_script", "login_password_script"),
				RequiredWith:  subResArray(parent, "login_username"),
			},
			"login_password_script": {
				Type:     schema.TypeString,
				Optional: true,
				Description: "Script to be executed to obtain the registry login password to be used to skopeo login." +
					" Password returned on STDOUT by the script.",
				ConflictsWith: subResArray(parent, "login_script", "login_password"),
				RequiredWith:  subResArray(parent, "login_username"),
			},
			"login_script": {
				Type:     schema.TypeString,
				Optional: true,
				Default:  "true",
				Description: "Script to be executed by the login_script_interpreter to authenticate" +
					" following skopeo operations",
				ConflictsWith: subResArray(parent, "login_username", "login_password", "login_password_script"),
			},
			"login_retries": {
				Type:     schema.TypeInt,
				Optional: true,
				Default:  "0",
				Description: "Either if the login_script/login_password_script reports failure with non-exit code, " +
					"or if following successful login the copy operation fails, retry this number of times.",
				ConflictsWith: subResArray(parent, "login_password"),
			},
			"login_environment": {
				Type:          schema.TypeMap,
				Optional:      true,
				Elem:          schema.TypeString,
				Description:   "Map of environment variables passed to the login_script/login_password_script",
				ConflictsWith: subResArray(parent, "login_password"),
			},
			"login_script_interpreter": {
				Type:     schema.TypeList,
				Optional: true,
				Elem: &schema.Schema{
					Type: schema.TypeString,
				},
				Description: "The interpreter used to execute the login_script/login_password_script, defaults to" +
					" [\"/bin/sh\", \"-c\"]",
				ConflictsWith: subResArray(parent, "login_password"),
			},
			"working_directory": {
				Type:          schema.TypeString,
				Optional:      true,
				Default:       ".",
				Description:   "The working directory in which to execute the login_script/login_password_script",
				ConflictsWith: subResArray(parent, "login_password"),
			},
			"timeout": {
				Type:          schema.TypeInt,
				Optional:      true,
				Default:       "60",
				Description:   "Timeout for login_script/login_password_script to execute in seconds",
				ConflictsWith: subResArray(parent, "login_password"),
			},
		},
	}
}

func resourceSkopeo2Copy() *schema.Resource {
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
				Required:    true,
				MaxItems:    1,
				Description: "Source image location",
				Elem:        somewhereResource("source", imageDescriptionTemplate, validateSourceImage),
			},
			"destination": {
				Type:        schema.TypeList,
				Required:    true,
				MaxItems:    1,
				ForceNew:    true,
				Description: "Destination image location",
				Elem:        somewhereResource("destination", imageDescriptionTemplate+"\nWhen working with GitHub Container registry `keep_image` needs to be set to `true`.", validateDestinationImage),
			},
			"retries": {
				Type:     schema.TypeInt,
				Optional: true,
				Default:  "0",
				Description: "Retry the copy operation following transient failure. " +
					"Retrying following access failure error is configured through login_retries.",
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
				Type:        schema.TypeBool,
				Optional:    true,
				Default:     false,
				ForceNew:    true,
				Description: "fail if we cannot preserve the source digests in the destination image.",
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

// We copy from *somewhere* to *somewhere*
type somewhere struct {
	image                 string
	loginScript           string
	loginRetries          int
	loginEnv              []string
	loginInterpreter      []string
	loginRetriesRemaining int
	workingDirectory      string
	loginUsername         string
	loginPassword         string
	loginPasswordScript   string
	unPwLogin             bool
	pwScript              bool
	imageOptions          *skopeoPkg.ImageOptions
	cmdTimeout            time.Duration
}

func getSomewhereParams(d *schema.ResourceData, key string) (*somewhere, error) {

	l := d.Get(key).([]interface{})
	e := l[0].(map[string]interface{})

	params := somewhere{}
	params.image = e["image"].(string)
	params.loginScript = e["login_script"].(string)
	params.workingDirectory = e["working_directory"].(string)
	if lr := e["login_retries"]; lr != nil {
		params.loginRetries = lr.(int)
	}

	eEnv := e["login_environment"].(map[string]any)
	var envList []string
	for k, v := range eEnv {
		envList = append(envList, k+"="+v.(string))
	}
	params.loginEnv = envList

	var interpreter []string
	if v := e["login_script_interpreter"]; v != nil && len(v.([]any)) > 0 {
		for _, val := range v.([]any) {
			interpreter = append(interpreter, val.(string))
		}
	} else {
		interpreter = []string{"/bin/sh", "-c"}
	}
	params.loginInterpreter = interpreter
	username, unPwLogin := d.GetOk(subRes(key, "login_username"))
	params.unPwLogin = unPwLogin
	if unPwLogin {
		params.loginUsername = username.(string)
		if loginPasswordScript, ok := d.GetOk(subRes(key, "login_password_script")); ok {
			params.loginPasswordScript = loginPasswordScript.(string)
			params.pwScript = true
		} else if loginPassword, ok := d.GetOk(subRes(key, "login_password")); ok {
			params.loginPassword = loginPassword.(string)
			params.pwScript = false
		} else {
			return nil, fmt.Errorf("either login_password or login_password_script needs to be specified")
		}
		params.loginPasswordScript = d.Get(subRes(key, "login_password_script")).(string)
	}
	params.imageOptions = newImageOptions(d)
	params.cmdTimeout = time.Duration(d.Get(subRes(key, "timeout")).(int)) * time.Second

	return &params, nil
}

func (sw *somewhere) withEndpointLogin(ctx context.Context, locked bool, op func(locked bool) (any, error)) (any, error) {

	//Try the operation without logging in first, as the credentials may already be in place
	result, err := op(locked)
	if err == nil {
		return result, nil
	}

	sw.loginRetriesRemaining--

	//Return error without attempting login if no login command is provided
	if sw.loginScript == "true" && sw.unPwLogin == false {
		return nil, err
	}

	if !locked {
		//We will get the lock if no other login is happening
		if loginInProgress.TryLock() {
			defer loginInProgress.Unlock()
		} else {
			//Block until the other login has completed
			loginInProgress.Lock()
			defer loginInProgress.Unlock()

			//Try the operation again now that some other login has completed
			result, err = op(true)
			if err == nil {
				return result, nil
			}
		}
	}

	//Didn't succeed so login
	err = sw.doLogin(ctx)
	if err != nil {
		return nil, err
	}

	//Try the operation a final time now that the login has completed
	return op(true)
}

func (sw *somewhere) doLogin(ctx context.Context) error {
	var err error
	if sw.unPwLogin {
		var password = sw.loginPassword
		if sw.pwScript {
			if password, err = sw.runLoginPasswordScript(ctx, sw.loginPasswordScript); err != nil {
				return err
			}
		}
		return sw.doUnPwLogin(ctx, password)
	}
	_, err = sw.runLoginPasswordScript(ctx, sw.loginScript)
	return err
}

func (sw *somewhere) doUnPwLogin(ctx context.Context, password string) error {

	tflog.Info(ctx, "Login using user/pass script", map[string]any{"image": sw.image, "username": sw.loginUsername})
	var err error

	opts := &skopeo.LoginOptions{
		Image:    sw.imageOptions,
		Username: sw.loginUsername,
		Password: password,
	}

	opts.Stdout = providerlog.NewProviderLogWriter(
		log.Default().Writer(),
	)
	defer opts.Stdout.(*providerlog.ProviderLogWriter).Close()

	tflog.Debug(ctx, "Logging in", map[string]any{"image": sw.image, "user": sw.loginUsername})
	err = skopeo.Login(ctx, sw.image, opts)

	if err != nil {
		tflog.Info(ctx, "Login fail", map[string]any{"image": sw.image, "user": sw.loginUsername, "err": err})
		return err
	}

	tflog.Info(ctx, "Logged on", map[string]any{"image": sw.image, "user": sw.loginUsername})
	return nil
}

type cmdResult struct {
	outb []byte
	err  error
}

func (sw *somewhere) runLoginPasswordScript(ctx context.Context, script string) (string, error) {

	shell := sw.loginInterpreter[0]
	flags := append(sw.loginInterpreter[1:], script)
	cmd := exec.CommandContext(ctx, shell, flags...)
	cmd.Env = append(os.Environ(), sw.loginEnv...)
	cmd.Dir = sw.workingDirectory

	cmdDone := make(chan cmdResult, 1)

	go func() {
		stdOutErr, err := cmd.CombinedOutput()
		cmdDone <- cmdResult{err: err, outb: stdOutErr}
	}()

	select {
	case <-time.After(sw.cmdTimeout):
		syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		return "", fmt.Errorf("login password script timed out for image %s", sw.image)
	case result := <-cmdDone:
		if result.err != nil {
			tflog.Info(ctx, "Login password script failed", map[string]any{"image": sw.image, "err": result.err})
			if _, ok := result.err.(*exec.ExitError); ok {
				return "", fmt.Errorf("login password script failed for image %s: %s %s", sw.image,
					result.err.Error(), string(result.outb))
			}
			return "", result.err
		}
		return string(result.outb), nil
	}
}

func resourceSkopeo2CopyCreate(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {

	tflog.Debug(ctx, "Creating a resource")

	src, err := getSomewhereParams(d, "source")
	if err != nil {
		return diag.FromErr(err)
	}

	dst, err := getSomewhereParams(d, "destination")
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
		result, err := src.withEndpointLogin(ctx, false, func(locked bool) (any, error) {
			return dst.withEndpointLogin(ctx, locked, func(_ bool) (any, error) {
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

func resourceSkopeo2CopyRead(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
	dst, err := getSomewhereParams(d, "destination")
	if err != nil {
		return diag.FromErr(err)
	}

	dst.loginRetriesRemaining = dst.loginRetries + 1

	for {
		result, err := dst.withEndpointLogin(ctx, false, func(_ bool) (any, error) {
			tflog.Debug(ctx, "Inspecting", map[string]any{"image": dst.image})
			result, err := skopeo.Inspect(ctx, dst.image, newInspectOptions(d))
			if err != nil {
				tflog.Info(ctx, "Inspection failed", map[string]any{"image": dst.image, "err": err.Error()})
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

		if err == nil {
			if result == nil {
				d.SetId("")
				return nil
			}
			digest := result.(*skopeo.InspectOutput).Digest
			tflog.Info(ctx, "Inspection", map[string]any{"image": dst.image, "digest": digest})
			return diag.FromErr(d.Set("docker_digest", digest))
		}

		tflog.Info(ctx, "Retries remaining", map[string]any{"count": dst.loginRetriesRemaining})
		if dst.loginRetriesRemaining <= 0 {
			return diag.FromErr(err)
		}
	}
}

func resourceSkopeo2CopyUpdate(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
	return resourceSkopeo2CopyCreate(ctx, d, meta)
}

func resourceSkopeo2CopyDelete(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
	keep := d.Get("keep_image").(bool)
	if keep {
		return nil
	}

	// We need to delete
	dst, err := getSomewhereParams(d, "destination")
	if err != nil {
		return diag.FromErr(err)
	}

	if ghcr.Match([]byte(dst.image)) {
		return diag.Errorf("GitHub does not support deleting specific container images. Set keep_image to true.")
	}

	for {
		_, err := dst.withEndpointLogin(ctx, false, func(_ bool) (any, error) {
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

func newCopyOptions(d *schema.ResourceData, reportWriter *providerlog.ProviderLogWriter) *skopeo.CopyOptions {
	additionalTags := getStringList(d, "additional_tags", nil)
	preserveDigests := d.Get("preserve_digests").(bool)

	opts := &skopeo.CopyOptions{
		ReportWriter:    reportWriter,
		SrcImage:        newImageOptions(d),
		DestImage:       newImageDestOptions(d),
		RetryOpts:       newRetryOptions(d),
		AdditionalTags:  additionalTags,
		PreserveDigests: preserveDigests,
		All:             d.Get("copy_all_images").(bool),
	}
	return opts
}

func newDeleteOptions(d *schema.ResourceData) *skopeoPkg.DeleteOptions {
	opts := &skopeoPkg.DeleteOptions{
		Image:     newImageDestOptions(d).ImageOptions,
		RetryOpts: newRetryOptions(d),
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
				Insecure:     d.Get("insecure").(bool),
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
			Insecure:     d.Get("insecure").(bool),
		},
	}
	return opts
}

func newInspectOptions(d *schema.ResourceData) *skopeo.InspectOptions {
	opts := &skopeo.InspectOptions{
		Image:     newImageOptions(d),
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
