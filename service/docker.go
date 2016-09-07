package service

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"time"

	"github.com/docker/engine-api/client"
	"github.com/docker/engine-api/types"
	"github.com/docker/engine-api/types/container"
	"github.com/docker/engine-api/types/network"
	"github.com/docker/go-connections/nat"
)

type Docker struct {
	Image string
	Port  string
	Path  string
}

func (d *Docker) StartWithCancel() (*url.URL, func(), error) {
	defaultHeaders := map[string]string{"User-Agent": "engine-api-cli-1.0"}
	cli, err := client.NewClient("unix:///var/run/docker.sock", client.DefaultVersion, nil, defaultHeaders)
	if err != nil {
		return nil, nil, err
	}
	port, err := nat.NewPort("tcp", d.Port)
	if err != nil {
		return nil, nil, err
	}
	ctx := context.Background()
	log.Println("Creating Docker container", d.Image, "...")
	resp, err := cli.ContainerCreate(ctx,
		&container.Config{
			Hostname:     "localhost",
			Image:        d.Image,
			ExposedPorts: map[nat.Port]struct{}{port: struct{}{}},
		},
		&container.HostConfig{
			AutoRemove: true,
			PortBindings: nat.PortMap{
				port: []nat.PortBinding{nat.PortBinding{HostIP: "127.0.0.1"}},
			},
		},
		&network.NetworkingConfig{}, "")
	if err != nil {
		log.Println("error creating container:", err)
		return nil, nil, err
	}
	log.Println("Starting container...")
	err = cli.ContainerStart(ctx, resp.ID, types.ContainerStartOptions{})
	if err != nil {
		log.Println("error starting container:", err)
		return nil, nil, err
	}
	log.Printf("Container %s started\n", resp.ID)
	stat, err := cli.ContainerInspect(ctx, resp.ID)
	if err != nil {
		log.Printf("unable to inspect container %s: %s\n", resp.ID, err)
		return nil, nil, err
	}
	_, ok := stat.NetworkSettings.Ports[port]
	if !ok {
		err := errors.New(fmt.Sprintf("no bingings available for %v...\n", port))
		log.Println(err)
		return nil, nil, err
	}
	if len(stat.NetworkSettings.Ports[port]) != 1 {
		err := errors.New(fmt.Sprintf("error: wrong number of port bindings"))
		log.Println(err)
		return nil, nil, err
	}
	addr := stat.NetworkSettings.Ports[port][0]
	host := fmt.Sprintf("http://%s:%s%s", addr.HostIP, addr.HostPort, d.Path)
	s := time.Now()
	done := make(chan struct{})
	go func() {
	loop:
		for {
			select {
			case <-time.After(50 * time.Millisecond):
				_, err := http.Get(host)
				if err == nil {
					done <- struct{}{}
				}
			case <-done:
				break loop
			}
		}
	}()
	select {
	case <-time.After(10 * time.Second):
		err := errors.New(fmt.Sprintf("error: service does not respond in %v", 10*time.Second))
		log.Println(err)
		return nil, func() { stop(ctx, cli, resp.ID) }, err
	case <-done:
		close(done)
	}
	log.Println(time.Since(s))
	u, _ := url.Parse(host)
	log.Println("proxying requests to:", host)
	return u, func() { stop(ctx, cli, resp.ID) }, nil
}

func stop(ctx context.Context, cli *client.Client, id string) {
	fmt.Println("Stopping container", id)
	err := cli.ContainerStop(ctx, id, nil)
	if err != nil {
		log.Println("error: unable to stop container", id, err)
		return
	}
	fmt.Printf("Container %s stopped\n", id)
	err = cli.ContainerRemove(ctx, id, types.ContainerRemoveOptions{Force: true})
	if err != nil {
		fmt.Println("error: unable to remove container", id, err)
		return
	}
	fmt.Printf("Container %s removed\n", id)
}
