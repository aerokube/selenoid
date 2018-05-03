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
	"github.com/aerokube/selenoid/util"
	"log"
	"time"
)

type Mesos struct {
	ServiceBase
	Environment
	session.Caps
	LogConfig *ctr.LogConfig
	Client    *client.Client
}

func (m *Mesos) StartWithCancel() (*StartedService, error) {
	serviceStartTime := time.Now()
	requestId := m.RequestId
	taskId := "selenoid-" + uuid.New()
	returnChannel := make(chan *scheduler.DockerInfo)
	image := m.Service.Image.(string)
	log.Printf("[%d] [CREATING_CONTAINER] [%s]", requestId, image)
	task := scheduler.Task{
		TaskId:        taskId,
		Image:         image,
		EnableVNC:     m.Caps.VNC,
		ReturnChannel: returnChannel,
		Environment:   getEnvForTask(m.ServiceBase, m.Caps)}
	task.SendToMesos()
	container := <-returnChannel
	if container.ErrorMsg != "" {
		return nil, fmt.Errorf(container.ErrorMsg)
	}
	hostPort := container.NetworkSettings.Ports.ContainerPort[0].HostPort
	u := &url.URL{Scheme: "http", Host: container.AgentHost + ":" + hostPort, Path: m.Service.Path}
	log.Printf("[%d] [SERVICE_STARTED] [%s] [%s] [%.2fs]", requestId, image, taskId, util.SecondsSince(serviceStartTime))
	log.Printf("[%d] [PROXY_TO] [%s] [%s]", requestId, taskId, u.String())
	s := StartedService{
		Url: u,
		Container: &session.Container{
			ID:        container.Id,
			IPAddress: container.NetworkSettings.IPAddress,
		},
		Cancel: func() {
			scheduler.Sched.Kill(requestId, taskId)
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

func getEnvForTask(service ServiceBase, caps session.Caps) scheduler.Env {
	env := make([]scheduler.EnvVariable, 0)
	env = append(env, scheduler.EnvVariable{"TZ", getTimeZone(service, caps).String()})
	env = append(env, scheduler.EnvVariable{"SCREEN_RESOLUTION", caps.ScreenResolution})
	return scheduler.Env{env}
}
