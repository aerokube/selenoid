package service

import (
	"github.com/aerokube/selenoid/session"
	"github.com/docker/docker/client"
	ctr "github.com/docker/docker/api/types/container"
	"net/url"
	"fmt"
	"github.com/aerokube/selenoid/mesos/scheduler"
	"log"
	"strings"
	"net/http"
	"bytes"
)

type Mesos struct {
	ServiceBase
	Environment
	session.Caps
	LogConfig *ctr.LogConfig
	Client    *client.Client
}

const frameworkIdHolder  = "__FRAMEWORK_ID__"



func (m *Mesos) StartWithCancel() (*StartedService, error) {
	fmt.Println("Создаем контейнер мезос")
	fmt.Println(scheduler.ContainerId)
	u := &url.URL{Scheme: "http", Host: "127.0.0.1:31005", Path: m.Service.Path}
	s := StartedService{
		Url: u,
		Container: &session.Container{
			ID:        scheduler.ContainerId,
			IPAddress: "172.17.0.2",
		},
		Cancel: func() {
			removeMesosContainer(scheduler.CurrentScheduler, scheduler.ContainerId)
		},
	}
	fmt.Println(&s.Container.ID)
	return &s, nil
}

func removeMesosContainer(scheduler *scheduler.Scheduler,  containerId string) {
	log.Printf("[%d] [REMOVING_CONTAINER] [%s]\n")
	template := ` {
 
  "framework_id": {
    "value": "__FRAMEWORK_ID__"
  },

  "type" : "KILL",
  "kill" : {
    "task_id" : {"value" : "12220-3440-12532-my-task"}
  }
}`
	body := strings.Replace(template, frameworkIdHolder, scheduler.FrameworkId, 1);

	req, err := http.NewRequest("POST", scheduler.Url, strings.NewReader(body))

	req.Header.Set("Mesos-Stream-Id", scheduler.StreamId)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("[FAILED_TO_REMOVE_CONTAINER] [%v]\n", err)
		return
	}
	log.Printf("[%d] [CONTAINER_REMOVED] [%s]\n")

	buf := new(bytes.Buffer)
	buf.ReadFrom(resp.Body)
	fmt.Println(buf.String())
	fmt.Println(resp.Status)
}