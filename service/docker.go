package service

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/url"
	"time"

	"github.com/aerokube/selenoid/session"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"
	"strings"
)

// Docker - docker container manager
type Docker struct {
	ServiceBase
	Environment
	session.Caps
	LogConfig *container.LogConfig
	Client    *client.Client
}

// StartWithCancel - Starter interface implementation
func (d *Docker) StartWithCancel() (*StartedService, error) {
	selenium, err := nat.NewPort("tcp", d.Service.Port)
	if err != nil {
		return nil, fmt.Errorf("new selenium port: %v", err)
	}
	exposedPorts := map[nat.Port]struct{}{selenium: {}}
	var vnc nat.Port
	if d.VNC {
		vnc, err = nat.NewPort("tcp", "5900")
		if err != nil {
			return nil, fmt.Errorf("new vnc port: %v", err)
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
	timeZone := time.Local
	if d.TimeZone != "" {
		tz, err := time.LoadLocation(d.TimeZone)
		if err != nil {
			log.Printf("[%d] [BAD_TIMEZONE] [%s]\n", d.RequestId, d.TimeZone)
		} else {
			timeZone = tz
		}
	}
	env := []string{
		fmt.Sprintf("TZ=%s", timeZone),
		fmt.Sprintf("SCREEN_RESOLUTION=%s", d.ScreenResolution),
		fmt.Sprintf("ENABLE_VNC=%v", d.VNC),
	}
	env = append(env, d.Service.Env...)
	shmSize := int64(268435456)
	if d.Service.ShmSize > 0 {
		shmSize = d.Service.ShmSize
	}
	extraHosts := []string{}
	if len(d.Service.Hosts) > 0 {
		extraHosts = append(extraHosts, d.Service.Hosts...)
	}
	container, err := d.Client.ContainerCreate(ctx,
		&container.Config{
			Hostname:     d.ContainerHostname,
			Image:        d.Service.Image.(string),
			Env:          env,
			ExposedPorts: exposedPorts,
		},
		&container.HostConfig{
			Binds:        d.Service.Volumes,
			AutoRemove:   true,
			PortBindings: portBindings,
			LogConfig:    *d.LogConfig,
			NetworkMode:  container.NetworkMode(d.Network),
			Tmpfs:        d.Service.Tmpfs,
			ShmSize:      shmSize,
			Privileged:   true,
			Links:        strings.Split(d.ApplicationContainers, ","),
			Resources: container.Resources{
				Memory:   d.Memory,
				NanoCPUs: d.CPU,
			},
			ExtraHosts: extraHosts,
		},
		&network.NetworkingConfig{}, "")
	if err != nil {
		return nil, fmt.Errorf("create container: %v", err)
	}
	containerStartTime := time.Now()
	log.Printf("[%d] [STARTING_CONTAINER] [%s] [%s]\n", d.RequestId, d.Service.Image, container.ID)
	err = d.Client.ContainerStart(ctx, container.ID, types.ContainerStartOptions{})
	if err != nil {
		d.removeContainer(ctx, d.Client, container.ID)
		return nil, fmt.Errorf("start container: %v", err)
	}
	log.Printf("[%d] [CONTAINER_STARTED] [%s] [%s] [%v]\n", d.RequestId, d.Service.Image, container.ID, time.Since(containerStartTime))
	stat, err := d.Client.ContainerInspect(ctx, container.ID)
	if err != nil {
		d.removeContainer(ctx, d.Client, container.ID)
		return nil, fmt.Errorf("inspect container %s: %s", container.ID, err)
	}
	_, ok := stat.NetworkSettings.Ports[selenium]
	if !ok {
		d.removeContainer(ctx, d.Client, container.ID)
		return nil, fmt.Errorf("no bindings available for %v", selenium)
	}
	seleniumHostPort, vncHostPort := "", ""
	if d.IP == "" {
		if d.InDocker {
			containerIP := getContainerIP(d.Network, stat)
			seleniumHostPort = net.JoinHostPort(containerIP, d.Service.Port)
			if d.VNC {
				vncHostPort = net.JoinHostPort(containerIP, "5900")
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
	err = wait(u.String(), d.StartupTimeout)
	if err != nil {
		d.removeContainer(ctx, d.Client, container.ID)
		return nil, fmt.Errorf("wait: %v", err)
	}
	log.Printf("[%d] [SERVICE_STARTED] [%s] [%s] [%v]\n", d.RequestId, d.Service.Image, container.ID, time.Since(serviceStartTime))
	log.Printf("[%d] [PROXY_TO] [%s] [%s] [%s]\n", d.RequestId, d.Service.Image, container.ID, u.String())
	s := StartedService{
		Url:         u,
		ID:          container.ID,
		VNCHostPort: vncHostPort,
		Cancel:      func() { d.removeContainer(ctx, d.Client, container.ID) },
	}
	return &s, nil
}

func getContainerIP(networkName string, stat types.ContainerJSON) string {
	ns := stat.NetworkSettings
	if ns.IPAddress != "" {
		return stat.NetworkSettings.IPAddress
	}
	if len(ns.Networks) > 0 {
		possibleAddresses := []string{}
		for name, nt := range ns.Networks {
			if nt.IPAddress != "" {
				if name == networkName {
					return nt.IPAddress
				}
				possibleAddresses = append(possibleAddresses, nt.IPAddress)
			}
		}
		if len(possibleAddresses) > 0 {
			return possibleAddresses[0]
		}
	}
	return ""
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
