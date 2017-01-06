package service

import (
	"context"
	"fmt"
	"log"
	"net/url"
	"os"
	"time"

	"errors"
	"github.com/aandryashin/selenoid/config"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"
)

type Docker struct {
	Ip      string
	Client  *client.Client
	Service *config.Browser
}

func (docker *Docker) StartWithCancel() (*url.URL, func(), error) {
	port, err := nat.NewPort("tcp", docker.Service.Port)
	if err != nil {
		return nil, nil, err
	}
	ctx := context.Background()
	log.Println("Creating Docker container", docker.Service.Image, "...")
	resp, err := docker.Client.ContainerCreate(ctx,
		&container.Config{
			Hostname:     "localhost",
			Image:        docker.Service.Image.(string),
			ExposedPorts: map[nat.Port]struct{}{port: struct{}{}},
		},
		&container.HostConfig{
			AutoRemove: true,
			PortBindings: nat.PortMap{
				port: []nat.PortBinding{nat.PortBinding{HostIP: "0.0.0.0"}},
			},
			ShmSize:    268435456,
			Privileged: true,
		},
		&network.NetworkingConfig{}, "")
	if err != nil {
		return nil, nil, fmt.Errorf("error creating container: %v", err)
	}
	log.Println("Starting container...")
	err = docker.Client.ContainerStart(ctx, resp.ID, types.ContainerStartOptions{})
	if err != nil {
		removeContainer(ctx, docker.Client, resp.ID)
		return nil, nil, fmt.Errorf("error starting container: %v", err)
	}
	log.Printf("Container %s started\n", resp.ID)
	stat, err := docker.Client.ContainerInspect(ctx, resp.ID)
	if err != nil {
		stopAndRemoveContainer(ctx, docker.Client, resp.ID)
		return nil, nil, fmt.Errorf("unable to inspect container %s: %s\n", resp.ID, err)
	}
	_, ok := stat.NetworkSettings.Ports[port]
	if !ok {
		stopAndRemoveContainer(ctx, docker.Client, resp.ID)
		return nil, nil, fmt.Errorf("no bingings available for %v...\n", port)
	}
	if len(stat.NetworkSettings.Ports[port]) != 1 {
		stopAndRemoveContainer(ctx, docker.Client, resp.ID)
		return nil, nil, errors.New("error: wrong number of port bindings")
	}
	addr := stat.NetworkSettings.Ports[port][0]
	if docker.Ip == "" {
		_, err = os.Stat("/.dockerenv")
		if err != nil {
			addr.HostIP = "127.0.0.1"
		} else {
			addr.HostIP = "172.17.0.1"
		}
	} else {
		addr.HostIP = docker.Ip
	}
	host := fmt.Sprintf("http://%s:%s%s", addr.HostIP, addr.HostPort, docker.Service.Path)
	s := time.Now()
	err = wait(host, 10*time.Second)
	if err != nil {
		stopAndRemoveContainer(ctx, docker.Client, resp.ID)
		return nil, nil, err
	}
	log.Println(time.Since(s))
	u, _ := url.Parse(host)
	log.Println("proxying requests to:", host)
	return u, func() { stopAndRemoveContainer(ctx, docker.Client, resp.ID) }, nil
}

func stopAndRemoveContainer(ctx context.Context, cli *client.Client, id string) {
	stopContainer(ctx, cli, id)
	removeContainer(ctx, cli, id)
}

func removeContainer(ctx context.Context, cli *client.Client, id string) {
	fmt.Println("Removing container", id)
	err := cli.ContainerRemove(ctx, id, types.ContainerRemoveOptions{Force: true})
	if err != nil {
		fmt.Println("error: unable to remove container", id, err)
		return
	}
	fmt.Printf("Container %s removed\n", id)
}

func stopContainer(ctx context.Context, cli *client.Client, id string) {
	fmt.Println("Stopping container", id)
	err := cli.ContainerStop(ctx, id, nil)
	if err != nil {
		log.Println("error: unable to stop container", id, err)
		return
	}
	cli.ContainerWait(ctx, id)
	fmt.Printf("Container %s stopped\n", id)
}
