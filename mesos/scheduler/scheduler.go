package scheduler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"log"
	"bufio"
	"bytes"
)

var isAccepted bool
var scheduler *Scheduler
var frameworkId ID

const schedulerUrlTemplate = "[MASTER]/api/v1/scheduler"

type Scheduler struct {
	url string
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
			AgentId ID `json:"agent_id"`
		}
	}
	Type string
}

func Run(URL string) {
	schedulerUrl := strings.Replace(schedulerUrlTemplate, "[MASTER]", URL, 1)
	scheduler = &Scheduler{schedulerUrl}

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

	for scanner.Scan() {
		var line = scanner.Text()
		var m Message

		fmt.Println(line)
		var index = strings.LastIndex(line, "}")
		if index != -1 {
			jsonMessage := line[0:index+1]
			json.Unmarshal([]byte(jsonMessage), &m)
			if m.Type == "SUBSCRIBED" {
				frameworkId = ID{m.Subscribed.FrameworkId.Value}
				fmt.Println("Ура, мы подписались! Id = " + frameworkId.Value)
			} else if m.Type == "HEARTBEAT" {
				fmt.Println("Мезос жил, мезос жив, мезос будет жить!!!")
			} else if m.Type == "OFFERS" {
				var offersIds []ID
				offers := m.Offers.Offers
				for _, n := range offers {
					offersIds = append(offersIds, n.Id)
					fmt.Println(offersIds)
				}
				b, _ := json.Marshal(offersIds)
				if isAccepted == false {
					Accept(streamId, frameworkId.Value, m.Offers.Offers[0].AgentId.Value, string(b))
					isAccepted = true
					fmt.Println(isAccepted)
				} else {
					Decline(streamId, frameworkId, offersIds)
				}
			} else if m.Type == "FAILURE" {
				fmt.Println("Все плохо")
			} else if m.Type == "UPDATE" {
				uuid := m.Update.Status.Uuid
				Acknowledge(streamId, frameworkId, ID{m.Update.Status.AgentId.Value}, uuid)
			}
		}
	}
}
