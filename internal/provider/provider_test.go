package provider

import (
	"io/ioutil"
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
	StartLocalRegistry()
	ListContainer()
}

// ListContainer lists all the containers running on host machine
func ListContainer() error {
	cli, err := client.NewEnvClient()
	if err != nil {
		log.Fatalf("Unable to get new docker client: %v", err)
	}
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

func StartLocalRegistry() (string, error) {
	cli, err := client.NewEnvClient()
	if err != nil {
		log.Fatalf("Unable to create docker client\n")
	}

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
		HostPort: "9016",
	}
	containerPort, err := nat.NewPort("tcp", "5000")
	if err != nil {
		log.Println("Unable to get newPort")
		return "", err
	}

	seed := time.Now().UTC().UnixNano()
	nameGenerator := namegenerator.NewNameGenerator(seed)

	portBinding := nat.PortMap{containerPort: []nat.PortBinding{hostBinding}}
	cont, err := cli.ContainerCreate(
		context.Background(),
		&container.Config{
			Image: "registry:2",
			Env:   []string{"REGISTRY_STORAGE_DELETE_ENABLED=true"},
		},
		&container.HostConfig{
			PortBindings: portBinding,
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
	cli, err := client.NewEnvClient()
	if err != nil {
		log.Fatalf("Unable to create docker client\n")
	}

	err = cli.ContainerStop(context.Background(), containerID, container.StopOptions{})
	if err != nil {
		log.Println("Stop container failed")
		return err
	}
	return nil
}

// PruneContainers clears all containers that are not running
func PruneContainers() error {
	cli, err := client.NewEnvClient()
	if err != nil {
		log.Fatalf("Unable to create docker client\n")
	}
	report, err := cli.ContainersPrune(context.Background(), filters.Args{})
	if err != nil {
		log.Println("Prune container failed")
		return err
	}
	log.Printf("Containers pruned: %v\n", report.ContainersDeleted)
	return nil
}
