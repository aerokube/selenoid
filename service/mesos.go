package service

import (
	"github.com/aerokube/selenoid/session"
	"github.com/docker/docker/client"
	ctr "github.com/docker/docker/api/types/container"
	"net/url"
	"fmt"
	"github.com/aerokube/selenoid/mesos/scheduler"
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
	time.Sleep(10 * time.Second)
	scheduler.IsNeedAccepted = true
	time.Sleep(20 * time.Second)
	fmt.Println("Создаем контейнер мезос")
	hostPort := scheduler.MesosContainer.NetworkSettings.Ports.ContainerPort[0].HostPort
	u := &url.URL{Scheme: "http", Host: "127.0.0.1:" + hostPort, Path: m.Service.Path}
	s := StartedService{
		Url: u,
		Container: &session.Container{
			ID:        scheduler.MesosContainer.Id,
			IPAddress: scheduler.MesosContainer.NetworkSettings.IPAddress,
		},
		Cancel: func() {
			scheduler.Sched.Kill()
		},
	}
	fmt.Println(&s.Container.ID)
	return &s, nil
}
