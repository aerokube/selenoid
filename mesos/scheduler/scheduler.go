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
	"sort"
)

var (
	Sched    *Scheduler
	channel  = make(chan Task)
	CpuLimit float64
	MemLimit float64
)

const schedulerUrlTemplate = "[MASTER]/api/v1/scheduler"

type Task struct {
	TaskId        string
	Image         string
	ReturnChannel chan *DockerInfo
}

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
	Url           string
	StreamId      string
	FrameworkId   ID
	CurrentOffers []Offer
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

func Run(URL string, cpu float64, mem float64) {
	setResourceLimits(cpu, mem)
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
			jsonMessage := line[0:index+1]
			json.Unmarshal([]byte(jsonMessage), &m)
			handle(m)
			if m.Type == "SUBSCRIBED" {
				frameworkId = m.Subscribed.FrameworkId
				fmt.Println("Ура, мы подписались! Id = " + frameworkId.Value)
				Sched = &Scheduler{
					Url: schedulerUrl,
					StreamId: streamId,
					FrameworkId: frameworkId}
			} else if m.Type == "OFFERS" {
				var offersIds []ID
				offers := m.Offers.Offers
				for _, n := range offers {
					offersIds = append(offersIds, n.Id)
					fmt.Println(offersIds)
				}
				tasksCanRun, offersCapacity := getTotalOffersCapacity(offers)
				log.Printf("[MESOS CONTAINERS CAN BE RUN NOW] [%d]\n", tasksCanRun)
				log.Printf("[CURRENT MESOS CONTAINERS CAPACITY BY OFFERS] [%v]\n", offersCapacity)
				var tasks []Task
				if tasksCanRun == 0 {
					fmt.Println("nothing ready")
					Sched.Decline(offersIds)
				} else {
				Loop:
					for i := 0; i < tasksCanRun; i++ {
						select {
						case task := <-channel:
							notRunningTasks[task.TaskId] = task.ReturnChannel
							tasks = append(tasks, task)
						default:
							fmt.Println("Задачки закончились")
							break Loop
						}
					}
					if len(tasks) == 0 {
						Sched.Decline(offersIds)
					} else {
						fmt.Println("====================")
						fmt.Println(len(tasks))
						//Sched.DeprecatedAccept(m.Offers.Offers[0], tasks[0].TaskId)
						Sched.CurrentOffers = m.Offers.Offers
						Sched.Accept(offersCapacity, tasks)
					}
				}
			} else if m.Type == "UPDATE" {
				if m.Update.Status.State == "TASK_RUNNING" {
					n, _ := base64.StdEncoding.DecodeString(m.Update.Status.Data)
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

func setResourceLimits(cpu float64, mem float64) {
	if cpu > 0 {
		CpuLimit = cpu / 1000000000
	} else {
		CpuLimit = 0.2
	}

	if mem > 0 {
		MemLimit = mem
	} else {
		MemLimit = 128
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

func getTotalOffersCapacity(offers []Offer) (int, map[string][]Range) {
	tasksCanRun := 0
	totalOffersCapacity := make(map[string][]Range)
	for _, offer := range offers {
		offerCapacity, offersPortsRanges := getCapacityOfCurrentOffer(offer.Resources)
		totalOffersCapacity[offer.Id.Value] = offersPortsRanges
		tasksCanRun = tasksCanRun + offerCapacity
	}

	return tasksCanRun, totalOffersCapacity
}

func getCapacityOfCurrentOffer(resources []Resource) (int, []Range) {
	cpuCapacity := 0
	memCapacity := 0
	portsCapacity := 0
	var offersPortsResources []Range
	for _, resource := range resources {
		switch resource.Name {
		case "cpus":
			cpuCapacity = int(resource.Scalar.Value / CpuLimit)
		case "mem":
			memCapacity = int(resource.Scalar.Value / MemLimit)
		case "ports":
			offersPortsResources = resource.Ranges.Range
			for _, ports := range offersPortsResources {
				portsCapacity = int(portsCapacity + ((ports.End - ports.Begin) / 2))
			}
		}
	}
	allResourcesCapacity := []int{cpuCapacity, memCapacity, portsCapacity}
	sort.Ints(allResourcesCapacity)
	totalCapacity := allResourcesCapacity[0]
	offersPortsRanges := getPortsRanges(totalCapacity, offersPortsResources)
	return totalCapacity, offersPortsRanges
}

func getPortsRanges(offerCapacity int, ranges []Range) ([]Range) {
	portsRanges := make([]Range, 0)
	for i := 0; len(ranges) > i && len(portsRanges) != offerCapacity; i++ {
		portsBegin := ranges[i].Begin
		portsEnd := ranges[i].End
		for ; portsEnd-portsBegin >= 1 && len(portsRanges) != offerCapacity; {
			portsRanges = append(portsRanges, Range{portsBegin, portsBegin + 1})
			portsBegin = portsBegin + 2
		}

	}
	return portsRanges
}

func (task Task) SendToMesos() {
	channel <- task
}
