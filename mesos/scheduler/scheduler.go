package scheduler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"log"
	"bufio"
	"encoding/base64"
)

var IsNeedAccepted bool

var Sched *Scheduler

var MesosContainer *DockerInfo

const schedulerUrlTemplate = "[MASTER]/api/v1/scheduler"

type DockerInfo struct {
	Id string
	NetworkSettings struct {
		IPAddress string
	}
}

type Scheduler struct {
	Url         string
	StreamId    string
	FrameworkId string
}

type id struct {
	Value string `json:"value"`
}

type Message struct {
	Offers struct {
		Offers []struct {
			Id      id
			AgentId id `json:"agent_id"`
		}
	}
	Subscribed struct {
		FrameworkId              id    `json:"framework_id"`
		HeartbeatIntervalSeconds int64 `json:"heartbeat_interval_seconds"`
	}
	Update struct {
		Status struct {
			Uuid    string
			AgentId id     `json:"agent_id"`
			Data    string `json:"data"`
			State   string `json:"state"`
		}
	}
	Type string
}

func Run(URL string) {
	schedulerUrl := strings.Replace(schedulerUrlTemplate, "[MASTER]", URL, 1)

	resp, err := http.Post(schedulerUrl, "application/json", strings.NewReader(`{
   "type"       : "SUBSCRIBE",
   "subscribe"  : {
      "framework_info"  : {
        "user" :  "foo",
        "name" :  "My Best Framework",
        "roles": ["test"]
      }
  }
}`))
	if err != nil {
		log.Fatal(err)
	}

	defer resp.Body.Close()

	streamId := resp.Header.Get("Mesos-Stream-Id")
	fmt.Println(streamId)

	scanner := bufio.NewScanner(resp.Body)
	scanner.Split(bufio.ScanLines)

	var frameworkId string
	for scanner.Scan() {
		var line = scanner.Text()
		var m Message

		fmt.Println(line)
		var index = strings.LastIndex(line, "}")
		if index != -1 {
			jsonMessage := line[0:index+1]
			json.Unmarshal([]byte(jsonMessage), &m)
			if m.Type == "SUBSCRIBED" {
				frameworkId = m.Subscribed.FrameworkId.Value
				fmt.Println("Ура, мы подписались! Id = " + frameworkId)
				Sched = &Scheduler{schedulerUrl, streamId, frameworkId}
			} else if m.Type == "HEARTBEAT" {
				fmt.Println("Мезос жил, мезос жив, мезос будет жить!!!")
			} else if m.Type == "OFFERS" {
				var ids []id
				offers := m.Offers.Offers
				for _, n := range offers {
					ids = append(ids, n.Id)
					fmt.Println(ids)
				}
				b, _ := json.Marshal(ids)
				if IsNeedAccepted == true {
					Sched.Accept(m.Offers.Offers[0].AgentId.Value, string(b))
					IsNeedAccepted = false
					fmt.Println(IsNeedAccepted)
				} else {
					Sched.Decline(string(b))
				}
			} else if m.Type == "FAILURE" {
				fmt.Println("Все плохо")
			} else if m.Type == "UPDATE" {
				uuid := m.Update.Status.Uuid
				Sched.Acknowledge(m.Update.Status.AgentId.Value, uuid)
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
