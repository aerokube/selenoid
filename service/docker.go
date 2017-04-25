package service

import (
	"context"
	"fmt"
	"log"
	"net/url"
	"os"
	"time"

	"github.com/aerokube/selenoid/config"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"
)

// Docker - docker container manager
type Docker struct {
	IP               string
	Client           *client.Client
	Service          *config.Browser
	LogConfig        *container.LogConfig
	ScreenResolution string
	RequestId uint64
}

// StartWithCancel - Starter interface implementation
func (d *Docker) StartWithCancel() (*url.URL, func(), error) {
	requestId := d.RequestId
	port, err := nat.NewPort("tcp", d.Service.Port)
	if err != nil {
		return nil, nil, err
	}
	ctx := context.Background()
	imageRef := d.Service.Image.(string)
	log.Printf("[%d] [CREATING_CONTAINER] [%s]\n", requestId, imageRef)
	env := []string{
		fmt.Sprintf("TZ=%s", time.Local),
		fmt.Sprintf("SCREEN_RESOLUTION=%s", d.ScreenResolution),
	}
	resp, err := d.Client.ContainerCreate(ctx,
		&container.Config{
			Hostname:     "localhost",
			Image:        d.Service.Image.(string),
			Env:          env,
			ExposedPorts: map[nat.Port]struct{}{port: {}},
		},
		&container.HostConfig{
			AutoRemove: true,
			PortBindings: nat.PortMap{
				port: []nat.PortBinding{{HostIP: "0.0.0.0"}},
			},
			LogConfig:  *d.LogConfig,
			Tmpfs:      d.Service.Tmpfs,
			ShmSize:    268435456,
			Privileged: true,
		},
		&network.NetworkingConfig{}, "")
	if err != nil {
		return nil, nil, fmt.Errorf("create container: %v", err)
	}
	containerStartTime := time.Now()
	containerId := resp.ID
	log.Printf("[%d] [STARTING_CONTAINER] [%s] [%s]\n", requestId, containerId, imageRef)
	err = d.Client.ContainerStart(ctx, containerId, types.ContainerStartOptions{})
	if err != nil {
		d.removeContainer(ctx, d.Client, containerId)
		return nil, nil, fmt.Errorf("start container: %v", err)
	}
	log.Printf("[%d] [CONTAINER_STARTED] [%s] [%s] [%v]\n", requestId, imageRef, containerId, time.Since(containerStartTime))
	stat, err := d.Client.ContainerInspect(ctx, containerId)
	if err != nil {
		d.removeContainer(ctx, d.Client, containerId)
		return nil, nil, fmt.Errorf("inspect container %s: %s", containerId, err)
	}
	_, ok := stat.NetworkSettings.Ports[port]
	if !ok {
		d.removeContainer(ctx, d.Client, containerId)
		return nil, nil, fmt.Errorf("no bindings available for %v", port)
	}
	numBundings := len(stat.NetworkSettings.Ports[port])
	if numBundings != 1 {
		d.removeContainer(ctx, d.Client, containerId)
		return nil, nil, fmt.Errorf("wrong number of port bindings: %d", numBundings)
	}
	addr := stat.NetworkSettings.Ports[port][0]
	if d.IP == "" {
		_, err = os.Stat("/.dockerenv")
		if err != nil {
			addr.HostIP = "127.0.0.1"
		} else {
			addr.HostIP = stat.NetworkSettings.IPAddress
			addr.HostPort = d.Service.Port
		}
	} else {
		addr.HostIP = d.IP
	}
	host := fmt.Sprintf("http://%s:%s%s", addr.HostIP, addr.HostPort, d.Service.Path)
	serviceStartTime := time.Now()
	err = wait(host, 30*time.Second)
	if err != nil {
		d.removeContainer(ctx, d.Client, containerId)
		return nil, nil, err
	}
	log.Printf("[%d] [SERVICE_STARTED] [%s] [%s] [%v]\n", requestId, imageRef, containerId, time.Since(serviceStartTime))
	u, _ := url.Parse(host)
	log.Println("proxying requests to:", host)
	return u, func() { d.removeContainer(ctx, d.Client, resp.ID) }, nil
}

func (docker *Docker) removeContainer(ctx context.Context, cli *client.Client, id string) {
	requestId := docker.RequestId
	log.Printf("[%d] [REMOVE_CONTAINER] [%s]\n", requestId, id)
	err := cli.ContainerRemove(ctx, id, types.ContainerRemoveOptions{Force: true, RemoveVolumes: true})
	if err != nil {
		log.Printf("[%d] [FAILED_TO_REMOVE_CONTAINER] [%s] [%v]\n", requestId, id, err)
		return
	}
	log.Printf("[%s] [CONTAINER_REMOVED] [%s]\n", requestId, id)
}
