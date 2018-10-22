package service

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/url"
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
	vncPort                = "5900"
	fileserverPort         = "8080"
	clipboardPort          = "9090"
)

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
	selenium := portConfig.SeleniumPort
	fileserver := portConfig.FileserverPort
	clipboard := portConfig.ClipboardPort
	vnc := portConfig.VNCPort
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
			Memory:   d.Memory,
			NanoCPUs: d.CPU,
		},
		ExtraHosts: getExtraHosts(d.Service, d.Caps),
	}
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
		servicePort:    selenium,
		vncPort:        vnc,
		fileserverPort: fileserver,
		clipboardPort:  clipboard,
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
	s := StartedService{
		Url: u,
		Container: &session.Container{
			ID:        browserContainerId,
			IPAddress: getContainerIP(d.Environment.Network, stat),
		},
		HostPort: hostPort,
		Cancel: func() {
			if videoContainerId != "" {
				stopVideoContainer(ctx, cl, requestId, videoContainerId, d.Environment)
			}
			defer removeContainer(ctx, cl, requestId, browserContainerId)
			if d.LogOutputDir != "" {
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
	fileserver, err := nat.NewPort("tcp", fileserverPort)
	if err != nil {
		return nil, fmt.Errorf("new fileserver port: %v", err)
	}
	clipboard, err := nat.NewPort("tcp", clipboardPort)
	if err != nil {
		return nil, fmt.Errorf("new clipboard port: %v", err)
	}
	exposedPorts := map[nat.Port]struct{}{selenium: {}, fileserver: {}}
	var vnc nat.Port
	if caps.VNC {
		vnc, err = nat.NewPort("tcp", vncPort)
		if err != nil {
			return nil, fmt.Errorf("new vnc port: %v", err)
		}
		exposedPorts[vnc] = struct{}{}
	}
	portBindings := nat.PortMap{}
	if env.IP != "" || !env.InDocker {
		portBindings[selenium] = []nat.PortBinding{{HostIP: "0.0.0.0"}}
		portBindings[fileserver] = []nat.PortBinding{{HostIP: "0.0.0.0"}}
		portBindings[clipboard] = []nat.PortBinding{{HostIP: "0.0.0.0"}}
		if caps.VNC {
			portBindings[vnc] = []nat.PortBinding{{HostIP: "0.0.0.0"}}
		}
	}
	return &portConfig{
		SeleniumPort:   selenium,
		FileserverPort: fileserver,
		ClipboardPort:  clipboard,
		VNCPort:        vnc,
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
		Fileserver: fn(fileserverPort, pc[fileserverPort]),
		Clipboard:  fn(clipboardPort, pc[clipboardPort]),
	}

	if caps.VNC {
		hp.VNC = fn(vncPort, pc[vncPort])
	}

	return hp
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
		Binds:       []string{fmt.Sprintf("%s:/data:rw", getVideoOutputDir(environ))},
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
	case <-notRunning:
	case <-doesNotExist:
	case <-time.After(env.SessionDeleteTimeout):
		err = cli.ContainerRemove(ctx, containerId, types.ContainerRemoveOptions{Force: true})
		if err != nil {
			log.Printf("[%d] [FAILED_TO_KILL_VIDEO_CONTAINER] [%s] [%v]", requestId, containerId, err)
		}
		log.Printf("[%d] [KILLED_VIDEO_CONTAINER] [%s] [%v]", requestId, containerId, err)
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
