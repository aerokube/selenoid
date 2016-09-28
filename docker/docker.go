package docker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/aandryashin/selenoid/service"
	"github.com/docker/engine-api/client"
	"github.com/docker/engine-api/types"
	"github.com/docker/engine-api/types/container"
	"github.com/docker/engine-api/types/network"
	"github.com/docker/go-connections/nat"
)

type Service struct {
	Image string `json:"image"`
	Port  string `json:"port"`
	Path  string `json:"path"`
}

type Versions struct {
	Default  string              `json:"default"`
	Versions map[string]*Service `json:"versions"`
}

type Config map[string]*Versions

func NewConfig(fn string) (*Config, error) {
	f, err := ioutil.ReadFile(fn)
	if err != nil {
		return nil, errors.New(fmt.Sprintf("error reading configuration file %s: %v", f, err))
	}
	c := make(Config)
	if err := json.Unmarshal(f, &c); err != nil {
		return nil, errors.New(fmt.Sprintf("error parsing configuration file %s: %v", f, err))
	}
	return &c, nil
}

func (config Config) Find(s, v string) (*Service, bool) {
	service, ok := config[s]
	if !ok {
		return nil, false
	}
	if v == "" {
		if v = service.Default; v == "" {
			return nil, false
		}
	}
	for k, s := range service.Versions {
		if strings.HasPrefix(k, v) {
			return s, true
		}
	}
	return nil, false
}

type Manager struct {
	Client *client.Client
	Config *Config
}

func (m *Manager) Find(s, v string) (service.Starter, bool) {
	service, ok := m.Config.Find(s, v)
	if !ok {
		return nil, false
	}
	return &Docker{m.Client, service}, true
}

type Docker struct {
	Client  *client.Client
	Service *Service
}

func (d *Docker) StartWithCancel() (*url.URL, func(), error) {
	port, err := nat.NewPort("tcp", d.Service.Port)
	if err != nil {
		return nil, nil, err
	}
	ctx := context.Background()
	log.Println("Creating Docker container", d.Service.Image, "...")
	resp, err := d.Client.ContainerCreate(ctx,
		&container.Config{
			Hostname:     "localhost",
			Image:        d.Service.Image,
			ExposedPorts: map[nat.Port]struct{}{port: struct{}{}},
		},
		&container.HostConfig{
			AutoRemove: true,
			PortBindings: nat.PortMap{
				port: []nat.PortBinding{nat.PortBinding{HostIP: "127.0.0.1"}},
			},
			ShmSize:    268435456,
			Privileged: true,
		},
		&network.NetworkingConfig{}, "")
	if err != nil {
		log.Println("error creating container:", err)
		return nil, nil, err
	}
	log.Println("Starting container...")
	err = d.Client.ContainerStart(ctx, resp.ID, types.ContainerStartOptions{})
	if err != nil {
		log.Println("error starting container:", err)
		return nil, nil, err
	}
	log.Printf("Container %s started\n", resp.ID)
	stat, err := d.Client.ContainerInspect(ctx, resp.ID)
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
	host := fmt.Sprintf("http://%s:%s%s", addr.HostIP, addr.HostPort, d.Service.Path)
	s := time.Now()
	done := make(chan struct{})
	go func() {
	loop:
		for {
			select {
			case <-time.After(50 * time.Millisecond):
				r, err := http.Get(host)
				if err == nil {
					r.Body.Close()
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
		stop(ctx, d.Client, resp.ID)
		return nil, nil, err
	case <-done:
		close(done)
	}
	log.Println(time.Since(s))
	u, _ := url.Parse(host)
	log.Println("proxying requests to:", host)
	return u, func() { stop(ctx, d.Client, resp.ID) }, nil
}

func stop(ctx context.Context, cli *client.Client, id string) {
	fmt.Println("Stopping container", id)
	err := cli.ContainerStop(ctx, id, nil)
	if err != nil {
		log.Println("error: unable to stop container", id, err)
		return
	}
	cli.ContainerWait(ctx, id)
	fmt.Printf("Container %s stopped\n", id)
	err = cli.ContainerRemove(ctx, id, types.ContainerRemoveOptions{Force: true})
	if err != nil {
		fmt.Println("error: unable to remove container", id, err)
		return
	}
	fmt.Printf("Container %s removed\n", id)
}
