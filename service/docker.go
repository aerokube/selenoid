package service

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/url"
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
	InDocker         bool
	CPU              int64
	Memory           int64
	Client           *client.Client
	Service          *config.Browser
	LogConfig        *container.LogConfig
	ScreenResolution string
	VNC              bool
	RequestId        uint64
}

// StartWithCancel - Starter interface implementation
func (d *Docker) StartWithCancel() (*url.URL, string, string, func(), error) {
	selenium, err := nat.NewPort("tcp", d.Service.Port)
	if err != nil {
		return nil, "", "", nil, fmt.Errorf("new selenium port: %v", err)
	}
	exposedPorts := map[nat.Port]struct{}{selenium: {}}
	var vnc nat.Port
	if d.VNC {
		vnc, err = nat.NewPort("tcp", "5900")
		if err != nil {
			return nil, "", "", nil, fmt.Errorf("new vnc port: %v", err)
		}
		exposedPorts[vnc] = struct{}{}
	}
	portBindings := nat.PortMap{}
	if d.IP != "" || !d.InDocker {
		portBindings[selenium] = []nat.PortBinding{{HostIP: "0.0.0.0"}}
		if d.VNC {
			portBindings[vnc] = []nat.PortBinding{{HostIP: "0.0.0.0"}}
		}
	}
	ctx := context.Background()
	log.Printf("[%d] [CREATING_CONTAINER] [%s]\n", d.RequestId, d.Service.Image)
	env := []string{
		fmt.Sprintf("TZ=%s", time.Local),
		fmt.Sprintf("SCREEN_RESOLUTION=%s", d.ScreenResolution),
		fmt.Sprintf("ENABLE_VNC=%v", d.VNC),
	}
	container, err := d.Client.ContainerCreate(ctx,
		&container.Config{
			Hostname:     "localhost",
			Image:        d.Service.Image.(string),
			Env:          env,
			ExposedPorts: exposedPorts,
		},
		&container.HostConfig{
			AutoRemove:   true,
			PortBindings: portBindings,
			LogConfig:    *d.LogConfig,
			Tmpfs:        d.Service.Tmpfs,
			ShmSize:      268435456,
			Privileged:   true,
			Resources: container.Resources{
				Memory:   d.Memory,
				NanoCPUs: d.CPU,
			},
		},
		&network.NetworkingConfig{}, "")
	if err != nil {
		return nil, "", "", nil, fmt.Errorf("create container: %v", err)
	}
	containerStartTime := time.Now()
	log.Printf("[%d] [STARTING_CONTAINER] [%s] [%s]\n", d.RequestId, d.Service.Image, container.ID)
	err = d.Client.ContainerStart(ctx, container.ID, types.ContainerStartOptions{})
	if err != nil {
		d.removeContainer(ctx, d.Client, container.ID)
		return nil, "", "", nil, fmt.Errorf("start container: %v", err)
	}
	log.Printf("[%d] [CONTAINER_STARTED] [%s] [%s] [%v]\n", d.RequestId, d.Service.Image, container.ID, time.Since(containerStartTime))
	stat, err := d.Client.ContainerInspect(ctx, container.ID)
	if err != nil {
		d.removeContainer(ctx, d.Client, container.ID)
		return nil, "", "", nil, fmt.Errorf("inspect container %s: %s", container.ID, err)
	}
	_, ok := stat.NetworkSettings.Ports[selenium]
	if !ok {
		d.removeContainer(ctx, d.Client, container.ID)
		return nil, "", "", nil, fmt.Errorf("no bingings available for %v", selenium)
	}
	seleniumHostPort, vncHostPort := "", ""
	if d.IP == "" {
		if d.InDocker {
			seleniumHostPort = net.JoinHostPort(stat.NetworkSettings.IPAddress, d.Service.Port)
			if d.VNC {
				vncHostPort = net.JoinHostPort(stat.NetworkSettings.IPAddress, "5900")
			}
		} else {
			seleniumHostPort = net.JoinHostPort("127.0.0.1", stat.NetworkSettings.Ports[selenium][0].HostPort)
			if d.VNC {
				vncHostPort = net.JoinHostPort("127.0.0.1", stat.NetworkSettings.Ports[vnc][0].HostPort)
			}
		}
	} else {
		seleniumHostPort = net.JoinHostPort(d.IP, stat.NetworkSettings.Ports[selenium][0].HostPort)
		if d.VNC {
			vncHostPort = net.JoinHostPort(d.IP, stat.NetworkSettings.Ports[vnc][0].HostPort)
		}
	}
	u := &url.URL{Scheme: "http", Host: seleniumHostPort, Path: d.Service.Path}
	serviceStartTime := time.Now()
	err = wait(u.String(), 30*time.Second)
	if err != nil {
		d.removeContainer(ctx, d.Client, container.ID)
		return nil, "", "", nil, fmt.Errorf("wait: %v", err)
	}
	log.Printf("[%d] [SERVICE_STARTED] [%s] [%s] [%v]\n", d.RequestId, d.Service.Image, container.ID, time.Since(serviceStartTime))
	log.Printf("[%d] [PROXY_TO] [%s] [%s] [%s]\n", d.RequestId, d.Service.Image, container.ID, u.String())
	return u, container.ID, vncHostPort, func() { d.removeContainer(ctx, d.Client, container.ID) }, nil
}

func (d *Docker) removeContainer(ctx context.Context, cli *client.Client, id string) {
	log.Printf("[%d] [REMOVE_CONTAINER] [%s]\n", d.RequestId, id)
	err := cli.ContainerRemove(ctx, id, types.ContainerRemoveOptions{Force: true, RemoveVolumes: true})
	if err != nil {
		log.Printf("[%d] [FAILED_TO_REMOVE_CONTAINER] [%s] [%v]\n", d.RequestId, id, err)
		return
	}
	log.Printf("[%d] [CONTAINER_REMOVED] [%s]\n", d.RequestId, id)
}
