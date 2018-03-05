package scheduler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"log"
	"bufio"
	"encoding/base64"
	"bytes"
)

var IsNeedAccepted bool

var Sched *Scheduler

var MesosContainer *DockerInfo

const schedulerUrlTemplate = "[MASTER]/api/v1/scheduler"

type DockerInfo struct {
	Id string
	NetworkSettings struct {
		Ports struct {
			ContainerPort []struct {
				HostPort string
			} `json:"4444/tcp"`
		}
		IPAddress string
	}
}

type Scheduler struct {
	Url         string
	StreamId    string
	FrameworkId ID
}

type Message struct {
	Offers struct {
		Offers []struct {
			Id      ID
			AgentId ID `json:"agent_id"`
		}
	}
	Subscribed struct {
		FrameworkId              ID    `json:"framework_id"`
		HeartbeatIntervalSeconds int64 `json:"heartbeat_interval_seconds"`
	}
	Update struct {
		Status struct {
			Uuid    string
			AgentId ID     `json:"agent_id"`
			Data    string `json:"data"`
			State   string `json:"state"`
		}
	}
	Type string
}

func Run(URL string) {
	schedulerUrl := strings.Replace(schedulerUrlTemplate, "[MASTER]", URL, 1)

	body, _ := json.Marshal(GetSubscribedMessage("foo", "My first framework", []string{"test"}))

	resp, err := http.Post(schedulerUrl, "application/json", bytes.NewReader(body))
	if err != nil {
		log.Fatal(err)
	}

	defer resp.Body.Close()

	streamId := resp.Header.Get("Mesos-Stream-Id")
	fmt.Println(streamId)

	scanner := bufio.NewScanner(resp.Body)
	scanner.Split(bufio.ScanLines)

	var frameworkId ID
	for scanner.Scan() {
		var line = scanner.Text()
		var m Message

		fmt.Println(line)
		var index = strings.LastIndex(line, "}")
		if index != -1 {
			jsonMessage := line[0:index+1]
			json.Unmarshal([]byte(jsonMessage), &m)
			if m.Type == "SUBSCRIBED" {
				frameworkId = m.Subscribed.FrameworkId
				fmt.Println("Ура, мы подписались! Id = " + frameworkId.Value)
				Sched = &Scheduler{schedulerUrl, streamId, frameworkId}
			} else if m.Type == "HEARTBEAT" {
				fmt.Println("Мезос жил, мезос жив, мезос будет жить!!!")
			} else if m.Type == "OFFERS" {
				var offersIds []ID
				offers := m.Offers.Offers
				for _, n := range offers {
					offersIds = append(offersIds, n.Id)
					fmt.Println(offersIds)
				}
				if IsNeedAccepted == true {
					Sched.Accept(m.Offers.Offers[0].AgentId, offersIds)
					IsNeedAccepted = false
					fmt.Println(IsNeedAccepted)
				} else {
					Sched.Decline(offersIds)
				}
			} else if m.Type == "FAILURE" {
				fmt.Println("Все плохо")
			} else if m.Type == "UPDATE" {
				uuid := m.Update.Status.Uuid
				Sched.Acknowledge(m.Update.Status.AgentId, uuid)
				if m.Update.Status.State == "TASK_RUNNING" {
					n, _ := base64.StdEncoding.DecodeString(m.Update.Status.Data)
					fmt.Println(string(n))
					var data []DockerInfo
					json.Unmarshal(n, &data)
					MesosContainer = &data[0]
				}
			}
		}
	}
}
