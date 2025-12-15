package provider

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/bsquare-corp/terraform-provider-skopeo2/internal/providerlog"
	"github.com/bsquare-corp/terraform-provider-skopeo2/internal/skopeo"
	"github.com/go-cmd/cmd"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

var (
	loginInProgress sync.Mutex
)

const (
	defaultTimeout          = 60
	defaultLoginScript      = "true"
	defaultWorkingDirectory = "."
	defaultLoginRetries     = 0
	defaultCertDir          = ""
	defaultRegistryAuthFile = ""
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
func SomewhereSchema(parent string, scriptOptions bool) map[string]*schema.Schema {
	s := map[string]*schema.Schema{}
	s["login_username"] = &schema.Schema{
		Type:        schema.TypeString,
		Optional:    true,
		Description: "Registry login username",
	}
	s["login_password"] = &schema.Schema{
		Type:         schema.TypeString,
		Optional:     true,
		Description:  "Registry login password",
		RequiredWith: subResArray(parent, "login_username"),
	}
	s["certificate_directory"] = &schema.Schema{
		Type:        schema.TypeString,
		Optional:    true,
		Description: "Use certificates at the specified path (*.crt, *.cert, *.key) to access the registry",
	}
	s["registry_auth_file"] = &schema.Schema{
		Type:     schema.TypeString,
		Optional: true,
		Description: "Path of the authentication file. Use REGISTRY_AUTH_FILE environment variable to override. " +
			"Default is ${XDG_RUNTIME_DIR}/containers/auth.json",
	}
	if scriptOptions {
		s["login_username"].ConflictsWith = subResArray(parent, "login_script")
		s["login_password"].ConflictsWith = subResArray(parent, "login_script", "login_password_script")
		s["login_password_script"] = &schema.Schema{
			Type:     schema.TypeString,
			Optional: true,
			Description: "Script to be executed to obtain the registry login password to be used to skopeo login." +
				" Password returned on STDOUT by the script.",
			ConflictsWith: subResArray(parent, "login_script", "login_password"),
			RequiredWith:  subResArray(parent, "login_username"),
		}
		s["login_script"] = &schema.Schema{
			Type:     schema.TypeString,
			Optional: true,
			Description: "Script to be executed by the login_script_interpreter to authenticate" +
				" following skopeo operations, default " + defaultLoginScript,
			ConflictsWith: subResArray(parent, "login_username", "login_password", "login_password_script"),
		}
		s["login_retries"] = &schema.Schema{
			Type:     schema.TypeInt,
			Optional: true,
			Description: "Either if the login_script/login_password_script reports failure with non-zero exit code, " +
				"or if following successful login the copy operation fails, " +
				"retry this number of times. Default " + strconv.Itoa(defaultLoginRetries),
			ConflictsWith: subResArray(parent, "login_password"),
		}
		s["login_environment"] = &schema.Schema{
			Type:          schema.TypeMap,
			Optional:      true,
			Elem:          schema.TypeString,
			Description:   "Map of environment variables passed to the login_script/login_password_script",
			ConflictsWith: subResArray(parent, "login_password"),
		}
		s["login_script_interpreter"] = &schema.Schema{
			Type:     schema.TypeList,
			Optional: true,
			Elem: &schema.Schema{
				Type: schema.TypeString,
			},
			Description: "The interpreter used to execute the login_script/login_password_script, defaults to" +
				" [\"/bin/sh\", \"-c\"]",
			ConflictsWith: subResArray(parent, "login_password"),
		}
		s["working_directory"] = &schema.Schema{
			Type:     schema.TypeString,
			Optional: true,
			Description: "The working directory in which to execute the login_script/login_password_script, " +
				"default " + defaultWorkingDirectory,
			ConflictsWith: subResArray(parent, "login_password"),
		}
		s["timeout"] = &schema.Schema{
			Type:          schema.TypeInt,
			Optional:      true,
			Description:   "Timeout for login_script/login_password_script to execute in seconds, default " + strconv.Itoa(defaultTimeout),
			ConflictsWith: subResArray(parent, "login_password"),
		}
	}
	return s
}

// We copy from *somewhere* to *somewhere*
type somewhere struct {
	image                   string
	hasImage                bool
	loginScript             string
	hasLoginScript          bool
	loginRetries            int
	hasLoginRetries         bool
	loginEnv                []string
	hasLoginEnv             bool
	loginInterpreter        []string
	hasLoginInterpreter     bool
	loginRetriesRemaining   int
	workingDirectory        string
	hasWorkingDirectory     bool
	loginUsername           string
	loginPassword           string
	loginPasswordScript     string
	unPwLogin               bool
	pwScript                bool
	cmdTimeout              time.Duration
	hasTimeout              bool
	hasCertificateDirectory bool
	certificateDirectory    string
	hasRegistryAuthFile     bool
	registryAuthFile        string
}

// Overriding Update this _somewhere_ object with elements from the provider block _somewhere_ object where this
// object's data takes priority.
func (sw *somewhere) Overriding(other *somewhere) {
	if !sw.hasImage {
		sw.image = other.image
		sw.hasImage = other.hasImage
	}

	if !sw.hasLoginScript {
		sw.loginScript = other.loginScript
		sw.hasLoginScript = other.hasLoginScript
	}

	if !sw.hasLoginRetries {
		sw.loginRetries = other.loginRetries
		sw.hasLoginRetries = other.hasLoginRetries
	}

	if !sw.hasLoginEnv {
		sw.loginEnv = other.loginEnv
		sw.hasLoginEnv = other.hasLoginEnv
	}

	if !sw.hasLoginInterpreter {
		sw.loginInterpreter = other.loginInterpreter
		sw.hasLoginInterpreter = other.hasLoginInterpreter
	}

	if !sw.hasWorkingDirectory {
		sw.workingDirectory = other.workingDirectory
		sw.hasWorkingDirectory = other.hasWorkingDirectory
	}

	if !sw.unPwLogin {
		sw.unPwLogin = other.unPwLogin
		sw.loginUsername = other.loginUsername
		sw.pwScript = other.pwScript
		sw.loginPasswordScript = other.loginPasswordScript
		sw.loginPassword = other.loginPassword
	}

	if !sw.hasTimeout {
		sw.hasTimeout = other.hasTimeout
		sw.cmdTimeout = other.cmdTimeout
	}

	if !sw.hasCertificateDirectory {
		sw.certificateDirectory = other.certificateDirectory
		sw.hasCertificateDirectory = other.hasCertificateDirectory
	}

	if !sw.hasRegistryAuthFile {
		sw.registryAuthFile = other.registryAuthFile
		sw.hasRegistryAuthFile = other.hasRegistryAuthFile
	}
}

func (sw *somewhere) SetImage(image string) {
	sw.image = image
	sw.hasImage = true
}

func (sw *somewhere) HasImage() bool {
	return sw.hasImage
}

func GetSomewhereParams(d *schema.ResourceData, key string) (*somewhere, error) {

	getOkSubRes := func(subKey string) (any, bool) {
		return d.GetOk(subRes(key, subKey))
	}

	sw := somewhere{}
	var attribute any

	if attribute, sw.hasLoginScript = getOkSubRes("login_script"); sw.hasLoginScript {
		sw.loginScript = attribute.(string)
	} else {
		sw.loginScript = defaultLoginScript
	}

	if attribute, sw.hasWorkingDirectory = getOkSubRes("working_directory"); sw.hasWorkingDirectory {
		sw.workingDirectory = attribute.(string)
	} else {
		sw.workingDirectory = defaultWorkingDirectory
	}

	if attribute, sw.hasLoginRetries = getOkSubRes("login_retries"); sw.hasLoginRetries {
		sw.loginRetries = attribute.(int)
	} else {
		sw.loginRetries = defaultLoginRetries
	}

	if attribute, sw.hasLoginEnv = getOkSubRes("login_environment"); sw.hasLoginEnv {
		var envList []string
		for k, v := range attribute.(map[string]any) {
			envList = append(envList, k+"="+v.(string))
		}
		sw.loginEnv = envList
	}

	var interpreter []string
	if attribute, sw.hasLoginInterpreter = getOkSubRes("login_script_interpreter"); attribute != nil && len(
		attribute.([]any)) > 0 {
		for _, val := range attribute.([]any) {
			interpreter = append(interpreter, val.(string))
		}
	} else {
		interpreter = []string{"/bin/sh", "-c"}
	}
	sw.loginInterpreter = interpreter

	username, unPwLogin := getOkSubRes("login_username")
	sw.unPwLogin = unPwLogin
	if unPwLogin {
		sw.loginUsername = username.(string)
		if loginPasswordScript, ok := getOkSubRes("login_password_script"); ok {
			sw.loginPasswordScript = loginPasswordScript.(string)
			sw.pwScript = true
		} else if loginPassword, ok := getOkSubRes("login_password"); ok {
			sw.loginPassword = loginPassword.(string)
			sw.pwScript = false
		} else {
			return nil, fmt.Errorf("either login_password or login_password_script needs to be specified")
		}
	}

	if attribute, sw.hasTimeout = getOkSubRes("timeout"); sw.hasTimeout {
		sw.cmdTimeout = time.Duration(attribute.(int)) * time.Second
	} else {
		sw.cmdTimeout = time.Duration(defaultTimeout) * time.Second
	}

	if attribute, sw.hasCertificateDirectory = getOkSubRes("certificate_directory"); sw.hasCertificateDirectory {
		sw.certificateDirectory = attribute.(string)
	} else {
		sw.certificateDirectory = defaultCertDir
	}

	if attribute, sw.hasRegistryAuthFile = getOkSubRes("registry_auth_file"); sw.hasRegistryAuthFile {
		sw.registryAuthFile = attribute.(string)
	} else {
		sw.registryAuthFile = defaultRegistryAuthFile
	}

	return &sw, nil
}

func (sw *somewhere) WithEndpointLogin(ctx context.Context, d *schema.ResourceData, locked bool, op func(locked bool) (any, error)) (any, error) {

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
	err = sw.DoLogin(ctx, d)
	if err != nil {
		return nil, err
	}

	//Try the operation a final time now that the login has completed
	return op(true)
}

func (sw *somewhere) DoLogin(ctx context.Context, d *schema.ResourceData) error {
	var err error
	if sw.unPwLogin {
		var password = sw.loginPassword
		if sw.pwScript {
			tflog.Info(ctx, "Running script to obtain password", map[string]any{"image": sw.image,
				"username": sw.loginUsername})
			if password, err = sw.RunLoginPasswordScript(ctx, sw.loginPasswordScript); err != nil {
				return err
			}
		}
		tflog.Info(ctx, "Login using username and password", map[string]any{"image": sw.image,
			"username": sw.loginUsername})
		return sw.doUnPwLogin(ctx, password, d)
	}
	tflog.Info(ctx, "Login using script", map[string]any{"image": sw.image})
	_, err = sw.RunLoginPasswordScript(ctx, sw.loginScript)
	return err
}

func (sw *somewhere) doUnPwLogin(ctx context.Context, password string, d *schema.ResourceData) error {

	var err error

	logWriter := providerlog.NewProviderLogWriter(
		log.Default().Writer(),
	)
	defer logWriter.Close()

	tflog.Debug(ctx, "Logging in", map[string]any{"image": sw.image, "user": sw.loginUsername})
	err = skopeo.Login(ctx, sw.image, newLoginOptions(d, sw, logWriter, password))

	if err != nil {
		tflog.Info(ctx, "Login fail", map[string]any{"image": sw.image, "user": sw.loginUsername, "err": err})
		return err
	}

	tflog.Info(ctx, "Logged on", map[string]any{"image": sw.image, "user": sw.loginUsername})
	return nil
}

func (sw *somewhere) RunLoginPasswordScript(ctx context.Context, script string) (string, error) {

	shell := sw.loginInterpreter[0]
	flags := append(sw.loginInterpreter[1:], script)
	loginCmd := cmd.NewCmdOptions(cmd.Options{Buffered: true}, shell, flags...)
	loginCmd.Env = append(os.Environ(), sw.loginEnv...)
	loginCmd.Dir = sw.workingDirectory

	statusChan := loginCmd.Start()

	go func() {
		<-time.After(sw.cmdTimeout)
		err := loginCmd.Stop()
		if err != nil {
			tflog.Info(ctx, "Login password script failed to be stopped after timeout",
				map[string]any{"image": sw.image, "err": err})
		}
	}()

	result := <-statusChan
	if !result.Complete {
		tflog.Warn(ctx, "Login password script timed out or was signalled", map[string]any{"image": sw.image})
		return "", fmt.Errorf("login password script timed out or was signalled for image %s", sw.image)
	}
	if result.Error != nil {
		tflog.Info(ctx, "Login password script failed", map[string]any{"image": sw.image, "err": result.Error})
		if _, ok := result.Error.(*exec.ExitError); ok {
			return "", fmt.Errorf("login password script failed for image %s: %s\n%s", sw.image,
				result.Error.Error(), strings.Join(result.Stderr, "\n"))
		}
		return "", result.Error
	}
	if result.Exit != 0 {
		tflog.Info(ctx, "Login password script returned non-zero exit status", map[string]any{"image": sw.image,
			"status": result.Exit})
		return "", fmt.Errorf("login password script failed with non-zero exit status for image %s exit"+
			" status: %d\n%s",
			sw.image, result.Exit, strings.Join(result.Stderr, "\n"))
	}
	return strings.Join(result.Stdout, ""), nil
}
