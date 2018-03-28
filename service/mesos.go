package service

import (
	"github.com/aerokube/selenoid/session"
	"github.com/docker/docker/client"
	ctr "github.com/docker/docker/api/types/container"
	"net/url"
	"fmt"
	"github.com/aerokube/selenoid/mesos/scheduler"
	"github.com/pborman/uuid"
	"github.com/aerokube/selenoid/mesos/zookeeper"
)

type Mesos struct {
	ServiceBase
	Environment
	session.Caps
	LogConfig *ctr.LogConfig
	Client    *client.Client
}

func (m *Mesos) StartWithCancel() (*StartedService, error) {
	taskId := "selenoid-" + uuid.New()
	zookeeper.CreateChildNodeZk(taskId)
	returnChannel := make(chan *scheduler.DockerInfo)
	task := scheduler.Task{taskId, m.Service.Image.(string), m.Caps.VNC, returnChannel}
	task.SendToMesos()
	container := <-returnChannel
	fmt.Println(container)
	if container.ErrorMsg != "" {
		return nil, fmt.Errorf(container.ErrorMsg)
	}
	hostPort := container.NetworkSettings.Ports.ContainerPort[0].HostPort
	u := &url.URL{Scheme: "http", Host: "127.0.0.1:" + hostPort, Path: m.Service.Path}
	s := StartedService{
		Url: u,
		Container: &session.Container{
			ID:        container.Id,
			IPAddress: container.NetworkSettings.IPAddress,
		},
		Cancel: func() {
			scheduler.Sched.Kill(taskId)
			zookeeper.DelNodeZk(taskId)
		},
	}
	if m.Caps.VNC {
		s.VNCHostPort = container.NetworkSettings.Ports.VncPort[0].HostPort
	}
	return &s, nil
}
