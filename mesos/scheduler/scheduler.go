package scheduler

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/aerokube/selenoid/mesos/zookeeper"
)

var (
	Sched   *Scheduler
	Channel = make(chan Task)
)

const schedulerUrlTemplate = "[MASTER]/api/v1/scheduler"

type Task struct {
	TaskId        string
	Image         string
	ReturnChannel chan *DockerInfo
}

type DockerInfo struct {
	Id              string
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
		Offers []Offer
	}
	Subscribed struct {
		FrameworkId              ID    `json:"framework_id"`
		HeartbeatIntervalSeconds int64 `json:"heartbeat_interval_seconds"`
	}
	Update struct {
		Status struct {
			Uuid       string
			AgentId    ID     `json:"agent_id"`
			Data       string `json:"data"`
			State      string `json:"state"`
			ExecutorId ID     `json:"executor_id"`
		}
	}
	Type string
}

type Offer struct {
	Id        ID         `json:"id"`
	AgentId   ID         `json:"agent_id"`
	Hostname  string     `json:"hostname"`
	Resources []Resource `json:"resources"`
}

func Run(URL string) {
	//zookeeper.DelZk()
	zookeeper.CreateZk()
	notRunningTasks := make(map[string]chan *DockerInfo)
	schedulerUrl := strings.Replace(schedulerUrlTemplate, "[MASTER]", URL, 1)

	body, _ := json.Marshal(GetSubscribedMessage("foo", "My first framework", []string{"test"}))

	resp, err := http.Post(schedulerUrl, "application/json", bytes.NewReader(body))
	if err != nil {
		log.Fatal(err)
	}

	defer resp.Body.Close()

	streamId := resp.Header.Get("Mesos-Stream-Id")

	scanner := bufio.NewScanner(resp.Body)
	scanner.Split(bufio.ScanLines)

	var frameworkId ID
	for scanner.Scan() {
		var line = scanner.Text()
		var m Message

		fmt.Println(line)
		var index = strings.LastIndex(line, "}")
		if index != -1 {
			jsonMessage := line[0 : index+1]
			json.Unmarshal([]byte(jsonMessage), &m)
			handle(m)
			if m.Type == "SUBSCRIBED" {
				frameworkId = m.Subscribed.FrameworkId
				fmt.Println("Ура, мы подписались! Id = " + frameworkId.Value)
				Sched = &Scheduler{schedulerUrl, streamId, frameworkId}
			} else if m.Type == "OFFERS" {
				var offersIds []ID
				offers := m.Offers.Offers
				for _, n := range offers {
					offersIds = append(offersIds, n.Id)
					fmt.Println(offersIds)
				}
				select {
				case task := <-Channel:
					notRunningTasks[task.TaskId] = task.ReturnChannel
					Sched.Accept(m.Offers.Offers[0], task.TaskId)
				default:
					fmt.Println("nothing ready")
					Sched.Decline(offersIds)
				}
			} else if m.Type == "UPDATE" {
				if m.Update.Status.State == "TASK_RUNNING" {
					n, _ := base64.StdEncoding.DecodeString(m.Update.Status.Data)
					fmt.Println(string(n))
					var data []DockerInfo
					json.Unmarshal(n, &data)
					container := &data[0]
					taskId := m.Update.Status.ExecutorId.Value
					channel, _ := notRunningTasks[taskId]
					channel <- container
					delete(notRunningTasks, taskId)
				}
			}
		}
	}
}

func handle(m Message) {
	if m.Type == "HEARTBEAT" {
		fmt.Println("Мезос жил, мезос жив, мезос будет жить!!!")
	} else if m.Type == "FAILURE" {
		fmt.Println("Все плохо")
	} else if m.Type == "UPDATE" {
		uuid := m.Update.Status.Uuid
		Sched.Acknowledge(m.Update.Status.AgentId, uuid, m.Update.Status.ExecutorId)
	}
}
