package scheduler

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sort"
	"strings"
	"github.com/aerokube/selenoid/mesos/zookeeper"
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
	EnableVNC     bool
	ReturnChannel chan *DockerInfo
}

type DockerInfo struct {
	Id              string
	NetworkSettings struct {
		Ports struct {
			ContainerPort []struct {
				HostPort string
			} `json:"4444/tcp"`
			VncPort []struct {
				HostPort string
			} `json:"5900/tcp"`
		}
		IPAddress string
	}
	ErrorMsg string
	AgentHost string
}

type Scheduler struct {
	Url           string
	StreamId      string
	FrameworkId   ID
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
			Source     string `json:"source"`
			Message    string `json:"message"`
			TaskId     ID     `json:"task_id"`
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

type ResourcesForOneTask struct {
	OfferId ID
	AgentId ID
	Range
}

func Run(URL string, zookeeperUrl string, cpu float64, mem float64) {
	if zookeeperUrl != "" {
		zookeeper.Zk = &zookeeper.Zoo{
			Url:zookeeperUrl,
		}
		Sched = &Scheduler{
			Url: zookeeper.DetectMaster() + "/api/v1/scheduler"}

		zookeeper.Create()
	} else {
		Sched = &Scheduler{
			Url: strings.Replace(schedulerUrlTemplate, "[MASTER]", URL, 1)}
	}
	setResourceLimits(cpu, mem)
	notRunningTasks := make(map[string]chan *DockerInfo)

	body, _ := json.Marshal(newSubscribedMessage("root", "My first framework", []string{"test"}))

	resp, err := http.Post(Sched.Url, "application/json", bytes.NewReader(body))
	if err != nil {
		log.Fatal(err)
	}

	defer resp.Body.Close()

	Sched.StreamId = resp.Header.Get("Mesos-Stream-Id")

	scanner := bufio.NewScanner(resp.Body)
	scanner.Split(bufio.ScanLines)

	for scanner.Scan() {
		var line = scanner.Text()
		var m Message

		fmt.Println(line)
		var index = strings.LastIndex(line, "}")
		if index != -1 {
			jsonMessage := line[0: index+1]
			json.Unmarshal([]byte(jsonMessage), &m)
			switch m.Type {
			case "SUBSCRIBED":
				Sched.FrameworkId = m.Subscribed.FrameworkId
				break
			case "OFFERS":
				processOffers(m, notRunningTasks)
				break
			case "UPDATE":
				processUpdate(m, notRunningTasks, zookeeperUrl)
				break
			case "FAILURE":
				fmt.Println("Bce плохо")
				break
			default:
				break
			}
		}
	}
}

func processUpdate(m Message, notRunningTasks map[string]chan *DockerInfo, zookeeperUrl string) {
	status := m.Update.Status
	state := status.State
	taskId := status.TaskId.Value
	Sched.Acknowledge(status.AgentId, status.Uuid, status.ExecutorId)
	if state == "TASK_RUNNING" {
		n, _ := base64.StdEncoding.DecodeString(status.Data)
		var data []DockerInfo
		json.Unmarshal(n, &data)
		container := &data[0]
		channel, _ := notRunningTasks[taskId]
		channel <- container
		delete(notRunningTasks, taskId)
		if zookeeperUrl!= "" {
			zookeeper.CreateTaskNode(status.TaskId.Value, status.AgentId.Value)
		}
	} else if state == "TASK_KILLED" {
		fmt.Println("Exterminate! Exterminate! Exterminate!")
	} else if state == "TASK_LOST" {
		fmt.Println("Здесь должен быть reconcile или типа того")
	} else {
		msg := "Галактика в опасности! Задача " + taskId + " непредвиденно упала по причине " + status.Source + "-" + status.State + "-" + status.Message
		if notRunningTasks[taskId] != nil {
			container := &DockerInfo{ErrorMsg: msg}
			channel, _ := notRunningTasks[taskId]
			channel <- container
			delete(notRunningTasks, taskId)
		} else {
			log.Panic(msg)
		}
	}
}

func processOffers(m Message, notRunningTasks map[string]chan *DockerInfo) {
	var offersIds []ID
	offers := m.Offers.Offers
	for _, n := range offers {
		offersIds = append(offersIds, n.Id)
		fmt.Println(offersIds)
	}
	tasksCanRun, resourcesForTasks := getTotalOffersCapacity(offers)
	log.Printf("[MESOS CONTAINERS CAN BE RUN NOW] [%d]\n", tasksCanRun)
	log.Printf("[CURRENT FREE MESOS CONTAINERS RESOURCES] [%v]\n", resourcesForTasks)
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
			Sched.Accept(resourcesForTasks[:len(tasks)], tasks)
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

func getTotalOffersCapacity(offers []Offer) (int, []ResourcesForOneTask) {
	tasksCanRun := 0
	var resourcesForTasks []ResourcesForOneTask
	for _, offer := range offers {
		offerCapacity, resources := getCapacityOfCurrentOffer(offer)
		tasksCanRun = tasksCanRun + offerCapacity
		resourcesForTasks = append(resourcesForTasks, resources...)
	}

	return tasksCanRun, resourcesForTasks
}

func getCapacityOfCurrentOffer(offer Offer) (int, []ResourcesForOneTask) {
	cpuCapacity := 0
	memCapacity := 0
	portsCapacity := 0
	var offersPortsResources []Range
	for _, resource := range offer.Resources {
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
	totalCapacityOfCurrentOffer := allResourcesCapacity[0]
	resourcesForTasks := getResourcesForTasks(offer, totalCapacityOfCurrentOffer, offersPortsResources)
	return totalCapacityOfCurrentOffer, resourcesForTasks
}

func getResourcesForTasks(offer Offer, offerCapacity int, ranges []Range) []ResourcesForOneTask {
	resourcesForTasks := make([]ResourcesForOneTask, 0)
	for i := 0; len(ranges) > i && len(resourcesForTasks) != offerCapacity; i++ {
		portsBegin := ranges[i].Begin
		portsEnd := ranges[i].End
		for portsEnd-portsBegin >= 1 && len(resourcesForTasks) != offerCapacity {
			portRange := Range{portsBegin, portsBegin + 1}
			resourcesForTasks = append(resourcesForTasks, ResourcesForOneTask{offer.Id, offer.AgentId, portRange})
			portsBegin = portsBegin + 2
		}

	}
	return resourcesForTasks
}

func (task Task) SendToMesos() {
	channel <- task
}
