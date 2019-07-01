package service

import (
	"context"
	"fmt"
	"github.com/docker/go-units"
	"log"
	"net"
	"net/url"
	"strconv"
	"time"

	"github.com/aerokube/selenoid/config"
	"github.com/aerokube/selenoid/session"
	"github.com/aerokube/util"
	"github.com/docker/docker/api/types"
	ctr "github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/api/types/strslice"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
	"github.com/docker/go-connections/nat"
	"os"
	"path/filepath"
	"strings"
)

const (
	sysAdmin               = "SYS_ADMIN"
	overrideVideoOutputDir = "OVERRIDE_VIDEO_OUTPUT_DIR"
)

var ports = struct {
	VNC, Devtools, Fileserver, Clipboard string
}{
	VNC:        "5900",
	Devtools:   "7070",
	Fileserver: "8080",
	Clipboard:  "9090",
}

// MemLimit - memory limit for Docker container
type MemLimit int64

func (limit *MemLimit) String() string {
	return units.HumanSize(float64(*limit))
}

func (limit *MemLimit) Set(s string) error {
	v, err := units.RAMInBytes(s)
	if err != nil {
		return fmt.Errorf("set memory limit: %v", err)
	}
	*limit = MemLimit(v)
	return nil
}

// CpuLimit - CPU limit for Docker container
type CpuLimit int64

func (limit *CpuLimit) String() string {
	return strconv.FormatFloat(float64(*limit/1000000000), 'f', -1, 64)
}

func (limit *CpuLimit) Set(s string) error {
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return fmt.Errorf("set cpu limit: %v", err)
	}
	*limit = CpuLimit(v * 1000000000)
	return nil
}

// Docker - docker container manager
type Docker struct {
	ServiceBase
	Environment
	session.Caps
	LogConfig *ctr.LogConfig
	Client    *client.Client
}

type portConfig struct {
	SeleniumPort   nat.Port
	FileserverPort nat.Port
	ClipboardPort  nat.Port
	DevtoolsPort   nat.Port
	VNCPort        nat.Port
	PortBindings   nat.PortMap
	ExposedPorts   map[nat.Port]struct{}
}

// StartWithCancel - Starter interface implementation
func (d *Docker) StartWithCancel() (*StartedService, error) {
	portConfig, err := getPortConfig(d.Service, d.Caps, d.Environment)
	if err != nil {
		return nil, fmt.Errorf("configuring ports: %v", err)
	}
	mem, err := getMemory(d.Service, d.Environment)
	if err != nil {
		return nil, fmt.Errorf("invalid memory limit: %v", err)
	}
	cpu, err := getCpu(d.Service, d.Environment)
	if err != nil {
		return nil, fmt.Errorf("invalid CPU limit: %v", err)
	}
	selenium := portConfig.SeleniumPort
	fileserver := portConfig.FileserverPort
	clipboard := portConfig.ClipboardPort
	vnc := portConfig.VNCPort
	devtools := portConfig.DevtoolsPort
	requestId := d.RequestId
	image := d.Service.Image
	ctx := context.Background()
	log.Printf("[%d] [CREATING_CONTAINER] [%s]", requestId, image)
	hostConfig := ctr.HostConfig{
		Binds:        d.Service.Volumes,
		AutoRemove:   true,
		PortBindings: portConfig.PortBindings,
		LogConfig:    getLogConfig(*d.LogConfig, d.Caps),
		NetworkMode:  ctr.NetworkMode(d.Network),
		Tmpfs:        d.Service.Tmpfs,
		ShmSize:      getShmSize(d.Service),
		Privileged:   d.Privileged,
		Resources: ctr.Resources{
			Memory:   mem,
			NanoCPUs: cpu,
		},
		ExtraHosts: getExtraHosts(d.Service, d.Caps),
	}
	hostConfig.PublishAllPorts = d.Service.PublishAllPorts
	if len(d.Caps.DNSServers) > 0 {
		hostConfig.DNS = d.Caps.DNSServers
	}
	if !d.Privileged {
		hostConfig.CapAdd = strslice.StrSlice{sysAdmin}
	}
	if len(d.ApplicationContainers) > 0 {
		hostConfig.Links = d.ApplicationContainers
	}
	if len(d.Service.Sysctl) > 0 {
		hostConfig.Sysctls = d.Service.Sysctl
	}
	cl := d.Client
	env := getEnv(d.ServiceBase, d.Caps)
	container, err := cl.ContainerCreate(ctx,
		&ctr.Config{
			Hostname:     getContainerHostname(d.Caps),
			Image:        image.(string),
			Env:          env,
			ExposedPorts: portConfig.ExposedPorts,
			Labels:       getLabels(d.Service, d.Caps),
		},
		&hostConfig,
		&network.NetworkingConfig{}, "")
	if err != nil {
		return nil, fmt.Errorf("create container: %v", err)
	}
	browserContainerStartTime := time.Now()
	browserContainerId := container.ID
	videoContainerId := ""
	log.Printf("[%d] [STARTING_CONTAINER] [%s] [%s]", requestId, image, browserContainerId)
	err = cl.ContainerStart(ctx, browserContainerId, types.ContainerStartOptions{})
	if err != nil {
		removeContainer(ctx, cl, requestId, browserContainerId)
		return nil, fmt.Errorf("start container: %v", err)
	}
	log.Printf("[%d] [CONTAINER_STARTED] [%s] [%s] [%.2fs]", requestId, image, browserContainerId, util.SecondsSince(browserContainerStartTime))

	if len(d.AdditionalNetworks) > 0 {
		for _, networkName := range d.AdditionalNetworks {
			err = cl.NetworkConnect(ctx, networkName, browserContainerId, nil)
			if err != nil {
				return nil, fmt.Errorf("failed to connect container %s to network %s: %v", browserContainerId, networkName, err)
			}
		}
	}

	stat, err := cl.ContainerInspect(ctx, browserContainerId)
	if err != nil {
		removeContainer(ctx, cl, requestId, browserContainerId)
		return nil, fmt.Errorf("inspect container %s: %s", browserContainerId, err)
	}
	_, ok := stat.NetworkSettings.Ports[selenium]
	if !ok {
		removeContainer(ctx, cl, requestId, browserContainerId)
		return nil, fmt.Errorf("no bindings available for %v", selenium)
	}
	servicePort := d.Service.Port
	pc := map[string]nat.Port{
		servicePort:      selenium,
		ports.VNC:        vnc,
		ports.Devtools:   devtools,
		ports.Fileserver: fileserver,
		ports.Clipboard:  clipboard,
	}
	hostPort := getHostPort(d.Environment, servicePort, d.Caps, stat, pc)
	u := &url.URL{Scheme: "http", Host: hostPort.Selenium, Path: d.Service.Path}

	if d.Video {
		videoContainerId, err = startVideoContainer(ctx, cl, requestId, stat, d.Environment, d.ServiceBase, d.Caps)
		if err != nil {
			return nil, fmt.Errorf("start video container: %v", err)
		}
	}

	serviceStartTime := time.Now()
	err = wait(u.String(), d.StartupTimeout)
	if err != nil {
		if videoContainerId != "" {
			stopVideoContainer(ctx, cl, requestId, videoContainerId, d.Environment)
		}
		removeContainer(ctx, cl, requestId, browserContainerId)
		return nil, fmt.Errorf("wait: %v", err)
	}
	log.Printf("[%d] [SERVICE_STARTED] [%s] [%s] [%.2fs]", requestId, image, browserContainerId, util.SecondsSince(serviceStartTime))
	log.Printf("[%d] [PROXY_TO] [%s] [%s]", requestId, browserContainerId, u.String())

	var publishedPortsInfo map[string]string
	if d.Service.PublishAllPorts {
		publishedPortsInfo = getContainerPorts(stat)
	}

	s := StartedService{
		Url: u,
		Container: &session.Container{
			ID:        browserContainerId,
			IPAddress: getContainerIP(d.Environment.Network, stat),
			Ports:     publishedPortsInfo,
		},
		HostPort: hostPort,
		Cancel: func() {
			if videoContainerId != "" {
				stopVideoContainer(ctx, cl, requestId, videoContainerId, d.Environment)
			}
			defer removeContainer(ctx, cl, requestId, browserContainerId)
			if d.LogOutputDir != "" && (d.SaveAllLogs || d.Log) {
				r, err := d.Client.ContainerLogs(ctx, browserContainerId, types.ContainerLogsOptions{
					Timestamps: true,
					ShowStdout: true,
					ShowStderr: true,
				})
				if err != nil {
					log.Printf("[%d] [FAILED_TO_COPY_LOGS] [%s] [Failed to capture container logs: %v]", requestId, browserContainerId, err)
					return
				}
				defer r.Close()
				filename := filepath.Join(d.LogOutputDir, d.LogName)
				f, err := os.Create(filename)
				if err != nil {
					log.Printf("[%d] [FAILED_TO_COPY_LOGS] [%s] [Failed to create log file %s: %v]", requestId, browserContainerId, filename, err)
					return
				}
				defer f.Close()
				_, err = stdcopy.StdCopy(f, f, r)
				if err != nil {
					log.Printf("[%d] [FAILED_TO_COPY_LOGS] [%s] [Failed to copy data to log file %s: %v]", requestId, browserContainerId, filename, err)
				}
			}
		},
	}
	return &s, nil
}

func getPortConfig(service *config.Browser, caps session.Caps, env Environment) (*portConfig, error) {
	selenium, err := nat.NewPort("tcp", service.Port)
	if err != nil {
		return nil, fmt.Errorf("new selenium port: %v", err)
	}
	fileserver, err := nat.NewPort("tcp", ports.Fileserver)
	if err != nil {
		return nil, fmt.Errorf("new fileserver port: %v", err)
	}
	clipboard, err := nat.NewPort("tcp", ports.Clipboard)
	if err != nil {
		return nil, fmt.Errorf("new clipboard port: %v", err)
	}
	exposedPorts := map[nat.Port]struct{}{selenium: {}, fileserver: {}, clipboard: {}}
	var vnc nat.Port
	if caps.VNC {
		vnc, err = nat.NewPort("tcp", ports.VNC)
		if err != nil {
			return nil, fmt.Errorf("new vnc port: %v", err)
		}
		exposedPorts[vnc] = struct{}{}
	}
	devtools, err := nat.NewPort("tcp", ports.Devtools)
	if err != nil {
		return nil, fmt.Errorf("new devtools port: %v", err)
	}
	exposedPorts[devtools] = struct{}{}

	portBindings := nat.PortMap{}
	if env.IP != "" || !env.InDocker {
		portBindings[selenium] = []nat.PortBinding{{HostIP: "0.0.0.0"}}
		portBindings[fileserver] = []nat.PortBinding{{HostIP: "0.0.0.0"}}
		portBindings[clipboard] = []nat.PortBinding{{HostIP: "0.0.0.0"}}
		portBindings[devtools] = []nat.PortBinding{{HostIP: "0.0.0.0"}}
		if caps.VNC {
			portBindings[vnc] = []nat.PortBinding{{HostIP: "0.0.0.0"}}
		}
	}
	return &portConfig{
		SeleniumPort:   selenium,
		FileserverPort: fileserver,
		ClipboardPort:  clipboard,
		VNCPort:        vnc,
		DevtoolsPort:   devtools,
		PortBindings:   portBindings,
		ExposedPorts:   exposedPorts}, nil
}

const (
	tag    = "tag"
	labels = "labels"
)

func getLogConfig(logConfig ctr.LogConfig, caps session.Caps) ctr.LogConfig {
	if logConfig.Config != nil {
		_, ok := logConfig.Config[tag]
		if caps.TestName != "" && !ok {
			logConfig.Config[tag] = caps.TestName
		}
		_, ok = logConfig.Config[labels]
		if len(caps.Labels) > 0 && !ok {
			var joinedLabels []string
			for k, v := range caps.Labels {
				joinedLabels = append(joinedLabels, fmt.Sprintf("%s=%s", k, v))
			}
			logConfig.Config[labels] = strings.Join(joinedLabels, ",")
		}
	}
	return logConfig
}

func getTimeZone(service ServiceBase, caps session.Caps) *time.Location {
	timeZone := time.Local
	if caps.TimeZone != "" {
		tz, err := time.LoadLocation(caps.TimeZone)
		if err != nil {
			log.Printf("[%d] [BAD_TIMEZONE] [%s]", service.RequestId, caps.TimeZone)
		} else {
			timeZone = tz
		}
	}
	return timeZone
}

func getEnv(service ServiceBase, caps session.Caps) []string {
	env := []string{
		fmt.Sprintf("TZ=%s", getTimeZone(service, caps)),
		fmt.Sprintf("SCREEN_RESOLUTION=%s", caps.ScreenResolution),
		fmt.Sprintf("ENABLE_VNC=%v", caps.VNC),
		fmt.Sprintf("ENABLE_VIDEO=%v", caps.Video),
	}
	if caps.Skin != "" {
		env = append(env, fmt.Sprintf("SKIN=%s", caps.Skin))
	}
	if caps.VideoCodec != "" {
		env = append(env, fmt.Sprintf("CODEC=%s", caps.VideoCodec))
	}
	env = append(env, service.Service.Env...)
	env = append(env, caps.Env...)
	return env
}

func getShmSize(service *config.Browser) int64 {
	if service.ShmSize > 0 {
		return service.ShmSize
	}
	return int64(268435456)
}

func getMemory(service *config.Browser, env Environment) (int64, error) {
	if service.Mem != "" {
		var mem MemLimit
		err := mem.Set(service.Mem)
		if err != nil {
			return 0, fmt.Errorf("parse memory limit: %v", err)
		}
		return int64(mem), nil
	}
	return env.Memory, nil
}

func getCpu(service *config.Browser, env Environment) (int64, error) {
	if service.Cpu != "" {
		var cpu CpuLimit
		err := cpu.Set(service.Cpu)
		if err != nil {
			return 0, fmt.Errorf("parse CPU limit: %v", err)
		}
		return int64(cpu), nil
	}
	return env.CPU, nil
}

func getContainerHostname(caps session.Caps) string {
	if caps.ContainerHostname != "" {
		return caps.ContainerHostname
	}
	return "localhost"
}

func getExtraHosts(service *config.Browser, caps session.Caps) []string {
	extraHosts := service.Hosts
	if len(caps.HostsEntries) > 0 {
		extraHosts = append(caps.HostsEntries, extraHosts...)
	}
	return extraHosts
}

func getLabels(service *config.Browser, caps session.Caps) map[string]string {
	labels := make(map[string]string)
	if caps.TestName != "" {
		labels["name"] = caps.TestName
	}
	for k, v := range service.Labels {
		labels[k] = v
	}
	if len(caps.Labels) > 0 {
		for k, v := range caps.Labels {
			labels[k] = v
		}
	}
	return labels
}

func getHostPort(env Environment, servicePort string, caps session.Caps, stat types.ContainerJSON, pc map[string]nat.Port) session.HostPort {
	fn := func(containerPort string, port nat.Port) string {
		return ""
	}
	if env.IP == "" {
		if env.InDocker {
			containerIP := getContainerIP(env.Network, stat)
			fn = func(containerPort string, port nat.Port) string {
				return net.JoinHostPort(containerIP, containerPort)
			}
		} else {
			fn = func(containerPort string, port nat.Port) string {
				return net.JoinHostPort("127.0.0.1", stat.NetworkSettings.Ports[port][0].HostPort)
			}
		}
	} else {
		fn = func(containerPort string, port nat.Port) string {
			return net.JoinHostPort(env.IP, stat.NetworkSettings.Ports[port][0].HostPort)
		}
	}
	hp := session.HostPort{
		Selenium:   fn(servicePort, pc[servicePort]),
		Fileserver: fn(ports.Fileserver, pc[ports.Fileserver]),
		Clipboard:  fn(ports.Clipboard, pc[ports.Clipboard]),
		Devtools:   fn(ports.Devtools, pc[ports.Devtools]),
	}

	if caps.VNC {
		hp.VNC = fn(ports.VNC, pc[ports.VNC])
	}

	return hp
}

func getContainerPorts(stat types.ContainerJSON) map[string]string {
	ns := stat.NetworkSettings

	var exposedPorts = make(map[string]string)

	if len(ns.Ports) > 0 {
		for port, portBindings := range ns.Ports {
			exposedPorts[port.Port()] = portBindings[0].HostPort
		}
	}
	return exposedPorts
}

func getContainerIP(networkName string, stat types.ContainerJSON) string {
	ns := stat.NetworkSettings
	if ns.IPAddress != "" {
		return stat.NetworkSettings.IPAddress
	}
	if len(ns.Networks) > 0 {
		var possibleAddresses []string
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

func startVideoContainer(ctx context.Context, cl *client.Client, requestId uint64, browserContainer types.ContainerJSON, environ Environment, service ServiceBase, caps session.Caps) (string, error) {
	videoContainerStartTime := time.Now()
	videoContainerImage := environ.VideoContainerImage
	env := getEnv(service, caps)
	env = append(env, fmt.Sprintf("FILE_NAME=%s", caps.VideoName))
	videoScreenSize := caps.VideoScreenSize
	if videoScreenSize != "" {
		env = append(env, fmt.Sprintf("VIDEO_SIZE=%s", videoScreenSize))
	}
	videoFrameRate := caps.VideoFrameRate
	if videoFrameRate > 0 {
		env = append(env, fmt.Sprintf("FRAME_RATE=%d", videoFrameRate))
	}
	hostConfig := &ctr.HostConfig{
		Binds:       []string{fmt.Sprintf("%s:/data:rw,z", getVideoOutputDir(environ))},
		AutoRemove:  true,
		NetworkMode: ctr.NetworkMode(environ.Network),
	}
	browserContainerName := getContainerIP(environ.Network, browserContainer)
	if environ.Network == DefaultContainerNetwork {
		const defaultBrowserContainerName = "browser"
		hostConfig.Links = []string{fmt.Sprintf("%s:%s", browserContainer.ID, defaultBrowserContainerName)}
		browserContainerName = defaultBrowserContainerName
	}
	env = append(env, fmt.Sprintf("BROWSER_CONTAINER_NAME=%s", browserContainerName))
	log.Printf("[%d] [CREATING_VIDEO_CONTAINER] [%s]", requestId, videoContainerImage)
	videoContainer, err := cl.ContainerCreate(ctx,
		&ctr.Config{
			Image: videoContainerImage,
			Env:   env,
		},
		hostConfig,
		&network.NetworkingConfig{}, "")
	if err != nil {
		removeContainer(ctx, cl, requestId, browserContainer.ID)
		return "", fmt.Errorf("create video container: %v", err)
	}

	videoContainerId := videoContainer.ID
	log.Printf("[%d] [STARTING_VIDEO_CONTAINER] [%s] [%s]", requestId, videoContainerImage, videoContainerId)
	err = cl.ContainerStart(ctx, videoContainerId, types.ContainerStartOptions{})
	if err != nil {
		removeContainer(ctx, cl, requestId, browserContainer.ID)
		removeContainer(ctx, cl, requestId, videoContainerId)
		return "", fmt.Errorf("start video container: %v", err)
	}
	log.Printf("[%d] [VIDEO_CONTAINER_STARTED] [%s] [%s] [%.2fs]", requestId, videoContainerImage, videoContainerId, util.SecondsSince(videoContainerStartTime))
	return videoContainerId, nil
}

func getVideoOutputDir(env Environment) string {
	videoOutputDirOverride := os.Getenv(overrideVideoOutputDir)
	if videoOutputDirOverride != "" {
		return videoOutputDirOverride
	}
	return env.VideoOutputDir
}

func stopVideoContainer(ctx context.Context, cli *client.Client, requestId uint64, containerId string, env Environment) {
	log.Printf("[%d] [STOPPING_VIDEO_CONTAINER] [%s]", requestId, containerId)
	err := cli.ContainerKill(ctx, containerId, "TERM")
	if err != nil {
		log.Printf("[%d] [FAILED_TO_STOP_VIDEO_CONTAINER] [%s] [%v]", requestId, containerId, err)
		return
	}
	notRunning, doesNotExist := cli.ContainerWait(ctx, containerId, ctr.WaitConditionNotRunning)
	select {
	case <-doesNotExist:
	case <-notRunning:
		removeContainer(ctx, cli, requestId, containerId)
		return
	case <-time.After(env.SessionDeleteTimeout):
		removeContainer(ctx, cli, requestId, containerId)
		return
	}
	log.Printf("[%d] [STOPPED_VIDEO_CONTAINER] [%s]", requestId, containerId)
}

func removeContainer(ctx context.Context, cli *client.Client, requestId uint64, id string) {
	log.Printf("[%d] [REMOVING_CONTAINER] [%s]", requestId, id)
	err := cli.ContainerRemove(ctx, id, types.ContainerRemoveOptions{Force: true, RemoveVolumes: true})
	if err != nil {
		log.Printf("[%d] [FAILED_TO_REMOVE_CONTAINER] [%s] [%v]", requestId, id, err)
		return
	}
	log.Printf("[%d] [CONTAINER_REMOVED] [%s]", requestId, id)
}
