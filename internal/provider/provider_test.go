package provider

import (
	"bufio"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/docker/docker/pkg/archive"
	"io"
	"io/ioutil"
	"os"
	"testing"
	"time"

	"context"
	"github.com/goombaio/namegenerator"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"log"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"
)

const (
	dockerRegistryUserID = "127.0.0.1:9016"
)

// providerFactories are used to instantiate a provider during acceptance testing.
// The factory function will be invoked for every Terraform CLI command executed
// to create a provider server to which the CLI can reattach.
var providerFactories = map[string]func() (*schema.Provider, error){
	"skopeo2": func() (*schema.Provider, error) {
		return New("dev")(), nil
	},
}

func TestProvider(t *testing.T) {
	if err := New("dev")().InternalValidate(); err != nil {
		t.Fatalf("err: %s", err)
	}
}

func testAccPreCheck(t *testing.T) {
	// You can add code here to run prior to any test case execution, for example assertions
	// about the appropriate environment variables being set are common to see in a pre-check
	// function.
	// registry with no auth
	_, err := StartLocalRegistry("9016", "")
	if err != nil {
		t.Logf("err: %s", err)
	}
	// password is "testpassword"
	_, err = StartLocalRegistry("9017", "testuser:$2y$05$3LiR99c.hXq.vRZGEHLVV.ZhzBV78VtmhxoK/ZypyDRbVdovxJTw.")
	if err != nil {
		t.Logf("err: %s", err)
	}
	// password is "testpassword2"
	_, err = StartLocalRegistry("9018", "testuser:$2y$05$6FpW38jCKtV5o/IdU7rUY.ODltYvTnq39EJxK8Ac9cPt8WOIEpMyq")
	if err != nil {
		t.Logf("err: %s", err)
	}
	err = ListContainer()
	if err != nil {
		t.Logf("err: %s", err)
	}

	// create a test image
	err = imageBuild(newDockerCli(context.Background()))
	if err != nil {
		t.Logf("err: %s", err)
	}

	// push the test image to one of the registries
	err = imagePush(newDockerCli(context.Background()))
	if err != nil {
		t.Logf("err: %s", err)
	}
}

func newDockerCli(ctx context.Context) *client.Client {
	cli, err := client.NewClientWithOpts(client.FromEnv)
	if err != nil {
		log.Fatalf("Unable to get new docker client: %v", err)
	}
	cli.NegotiateAPIVersion(ctx)
	return cli
}

// ListContainer lists all the containers running on host machine
func ListContainer() error {
	cli := newDockerCli(context.Background())

	containers, err := cli.ContainerList(context.Background(), types.ContainerListOptions{})
	if err != nil {
		log.Printf("Unable to list containers: %v", err)
		return err
	}
	if len(containers) > 0 {
		for _, c := range containers {
			log.Printf("Container ID: %s Image: %s", c.ID, c.Image)
		}
	} else {
		log.Println("There are no containers running")
	}
	return nil
}

func StartLocalRegistry(hostPort string, htpasswd string) (string, error) {
	// htpasswd file to mount into the container when it is started
	htpasswdFile := "/tmp/htpasswd" + hostPort
	err := os.WriteFile(htpasswdFile, []byte(htpasswd), 0644)
	if err != nil {
		log.Fatal(err)
	}

	cli := newDockerCli(context.Background())

	resp, err := cli.ImagePull(context.Background(), "registry:2", types.ImagePullOptions{})
	if err != nil {
		log.Println("Unable to pull")
		return "", err
	}
	_, err = ioutil.ReadAll(resp)
	if err != nil {
		return "", err
	}

	hostBinding := nat.PortBinding{
		HostIP:   "127.0.0.1",
		HostPort: hostPort,
	}
	containerPort, err := nat.NewPort("tcp", "5000")
	if err != nil {
		log.Println("Unable to get newPort")
		return "", err
	}

	seed := time.Now().UTC().UnixNano()
	nameGenerator := namegenerator.NewNameGenerator(seed)

	portBinding := nat.PortMap{containerPort: []nat.PortBinding{hostBinding}}

	var env []string
	env = append(env, "REGISTRY_STORAGE_DELETE_ENABLED=true")
	if htpasswd != "" {
		env = append(env,
			"REGISTRY_AUTH=htpasswd",
			"REGISTRY_AUTH_HTPASSWD_REALM=Registry Realm",
			"REGISTRY_AUTH_HTPASSWD_PATH=/htpasswd")
	}

	cont, err := cli.ContainerCreate(
		context.Background(),
		&container.Config{
			Image: "registry:2",
			Env:   env,
		},
		&container.HostConfig{
			PortBindings: portBinding,
			Binds:        []string{htpasswdFile + ":/htpasswd"},
		}, nil, nil, nameGenerator.Generate())
	if err != nil {
		log.Println("ContainerCreate failed")
		return "", err
	}

	err = cli.ContainerStart(context.Background(), cont.ID, types.ContainerStartOptions{})
	if err != nil {
		log.Println("ContainerStart failed")
		return "", err
	}
	log.Printf("Container %s has been started\n", cont.ID)
	return cont.ID, nil
}

// StopContainer stops the container of given ID
func StopContainer(containerID string) error {
	cli := newDockerCli(context.Background())

	err := cli.ContainerStop(context.Background(), containerID, container.StopOptions{})
	if err != nil {
		log.Println("Stop container failed")
		return err
	}
	return nil
}

// PruneContainers clears all containers that are not running
func PruneContainers() error {
	cli := newDockerCli(context.Background())

	report, err := cli.ContainersPrune(context.Background(), filters.Args{})
	if err != nil {
		log.Println("Prune container failed")
		return err
	}
	log.Printf("Containers pruned: %v\n", report.ContainersDeleted)
	return nil
}

func imageBuild(dockerClient *client.Client) error {

	dir, err := os.MkdirTemp("", "test-image")
	if err != nil {
		log.Fatal(err)
	}
	defer os.RemoveAll(dir)

	dockerfileName := dir + "/Dockerfile"
	dockerfileContents := "FROM scratch\nADD test-file.txt /test-file.txt\n"
	err = os.WriteFile(dockerfileName, []byte(dockerfileContents), 0644)
	if err != nil {
		panic(err)
	}

	err = os.WriteFile(dir+"/test-file.txt", []byte("test data"), 0644)
	if err != nil {
		panic(err)
	}

	fmt.Println(dockerfileName)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*120)
	defer cancel()

	tar, err := archive.TarWithOptions(dir, &archive.TarOptions{})
	if err != nil {
		return err
	}

	opts := types.ImageBuildOptions{
		Dockerfile: "Dockerfile",
		Tags:       []string{dockerRegistryUserID + "/test-image"},
		Remove:     true,
	}
	res, err := dockerClient.ImageBuild(ctx, tar, opts)
	if err != nil {
		return err
	}

	defer res.Body.Close()

	err = printDetails(res.Body)
	if err != nil {
		return err
	}

	return nil
}

type ErrorLine struct {
	Error       string      `json:"error"`
	ErrorDetail ErrorDetail `json:"errorDetail"`
}

type ErrorDetail struct {
	Message string `json:"message"`
}

func printDetails(rd io.Reader) error {
	var lastLine string

	scanner := bufio.NewScanner(rd)
	for scanner.Scan() {
		lastLine = scanner.Text()
		fmt.Println(scanner.Text())
	}

	errLine := &ErrorLine{}
	json.Unmarshal([]byte(lastLine), errLine)
	if errLine.Error != "" {
		return errors.New(errLine.Error)
	}

	if err := scanner.Err(); err != nil {
		return err
	}

	return nil
}

var authConfig = types.AuthConfig{
	Username:      "testuser",
	Password:      "testpassword",
	ServerAddress: "http://" + dockerRegistryUserID + "/v1/",
}

func imagePush(dockerClient *client.Client) error {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*120)
	defer cancel()

	authConfigBytes, _ := json.Marshal(authConfig)
	authConfigEncoded := base64.URLEncoding.EncodeToString(authConfigBytes)

	tag := dockerRegistryUserID + "/test-image"
	opts := types.ImagePushOptions{RegistryAuth: authConfigEncoded}
	rd, err := dockerClient.ImagePush(ctx, tag, opts)
	if err != nil {
		return err
	}

	defer rd.Close()

	err = printDetails(rd)
	if err != nil {
		return err
	}

	return nil
}
