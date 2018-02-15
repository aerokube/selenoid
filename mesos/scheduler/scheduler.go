package scheduler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"log"
	"bufio"
)

var isAccepted bool

var scheduler *Scheduler

const schedulerUrlTemplate = "[MASTER]/api/v1/scheduler"

type Scheduler struct {
	url string
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
			AgentId id `json:"agent_id"`
		}
	}
	Type string
}

func Run(URL string) {
	schedulerUrl := strings.Replace(schedulerUrlTemplate, "[MASTER]", URL, 1)
	scheduler = &Scheduler{schedulerUrl}

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
				if isAccepted == false {
					Accept(streamId, frameworkId, m.Offers.Offers[0].AgentId.Value, string(b))
					isAccepted = true
					fmt.Println(isAccepted)
				} else {
					Decline(streamId, frameworkId, string(b))
				}
			} else if m.Type == "FAILURE" {
				fmt.Println("Все плохо")
			} else if m.Type == "UPDATE" {
				uuid := m.Update.Status.Uuid
				Aknowledge(streamId, frameworkId, m.Update.Status.AgentId.Value, uuid)
			}
		}
	}
}
