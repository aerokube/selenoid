package service

import (
	"fmt"
	"github.com/aerokube/selenoid/mesos/scheduler"
	"github.com/aerokube/selenoid/mesos/zookeeper"
	"github.com/aerokube/selenoid/session"
	ctr "github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/pborman/uuid"
	"net/url"
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
	returnChannel := make(chan *scheduler.DockerInfo)
	task := scheduler.Task{
		TaskId:        taskId,
		Image:         m.Service.Image.(string),
		EnableVNC:     m.Caps.VNC,
		ReturnChannel: returnChannel,
		Environment:   getEnvForMesos(m.ServiceBase, m.Caps)}
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
			if m.Zookeeper != "" {
				zookeeper.DelNode(taskId)
			}
		},
	}
	if m.Caps.VNC {
		s.VNCHostPort = container.NetworkSettings.Ports.VncPort[0].HostPort
	}
	return &s, nil
}

func getEnvForMesos(service ServiceBase, caps session.Caps) scheduler.Env {
	env := make([]scheduler.EnvVariable, 0)
	env = append(env, scheduler.EnvVariable{"TZ", getTimeZone(service, caps).String()})
	env = append(env, scheduler.EnvVariable{"SCREEN_RESOLUTION", caps.ScreenResolution})
	return scheduler.Env{env}
}
