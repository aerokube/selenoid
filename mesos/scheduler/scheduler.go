package scheduler

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"github.com/aerokube/selenoid/mesos/zookeeper"
	"log"
	"net/http"
	"net/url"
	"sort"
	"strings"
)

var (
	Sched    *Scheduler
	channel  = make(chan Task)
	CpuLimit float64
	MemLimit float64
)

const schedulerPath = "/api/v1/scheduler"

type Task struct {
	TaskId        string
	Image         string
	EnableVNC     bool
	ReturnChannel chan *DockerInfo
	Environment   Env
}

type DockerInfo struct {
	Id string
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
	ErrorMsg  string
	AgentHost string
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
			Source     string `json:"source"`
			Message    string `json:"message"`
			TaskId     ID     `json:"task_id"`
			Reason     string `json:"reason"`
		}
	}
	Error struct {
		Message string `json:"message"`
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
	OfferId   ID
	AgentId   ID
	AgentHost string
	Range
}

type Info struct {
	ReturnChannel chan *DockerInfo
	AgentHost     string
}

func Run(URL string, zookeeperUrl string, cpu float64, mem float64) {
	if zookeeperUrl != "" {
		zookeeper.Zk = &zookeeper.Zoo{
			Url: zookeeperUrl,
		}
		zookeeper.Create()
	}
	schedulerUrl := createSchedulerUrl(URL)
	log.Printf("[-] [MESOS_URL_DETECTED] [%s]", schedulerUrl)
	Sched = &Scheduler{Url: schedulerUrl}
	setResourceLimits(cpu, mem)
	notRunningTasks := make(map[string]*Info)

	frameworkId := ""
	if zookeeperUrl != "" && zookeeper.GetFrameworkInfo() != nil {
		frameworkId = zookeeper.GetFrameworkInfo()[0]
	}
	body, _ := json.Marshal(newSubscribedMessage("root", "Selenoid", ID{frameworkId}))
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
			jsonMessage := line[0 : index+1]
			json.Unmarshal([]byte(jsonMessage), &m)
			switch m.Type {
			case "SUBSCRIBED":
				frameworkId := m.Subscribed.FrameworkId
				if zookeeperUrl != "" {
					frameworkInfo := zookeeper.GetFrameworkInfo()
					if frameworkInfo == nil || !contains(frameworkInfo, frameworkId.Value) {
						zookeeper.CreateFrameworkNode(frameworkId.Value)
					}
				}
				log.Printf("[-] [SELENOID_SUBSCRIBED_AS_FRAMEWORK_ON_MESOS_MASTER] [%s]", frameworkId)
				Sched.FrameworkId = frameworkId
				break
			case "OFFERS":
				processOffers(m, notRunningTasks)
				break
			case "UPDATE":
				processUpdate(m, notRunningTasks, zookeeperUrl)
				break
			case "FAILURE":
				log.Fatal("Mesos return FAILURE with message: " + m.Error.Message)
				break
			case "ERROR":
				log.Fatal("Mesos return ERROR with message: " + m.Error.Message)
				break
			default:
				break
			}
		}
	}
}

func processUpdate(m Message, notRunningTasks map[string]*Info, zookeeperUrl string) {
	status := m.Update.Status
	state := status.State
	taskId := status.TaskId.Value
	Sched.Acknowledge(status.AgentId, status.Uuid, status.ExecutorId)
	log.Printf("[-] [MESOS_TASK_HAVE_STATE] [%s] [%s]\n", taskId, state)
	if state == "TASK_RUNNING" {
		n, _ := base64.StdEncoding.DecodeString(status.Data)
		var data []DockerInfo
		json.Unmarshal(n, &data)
		container := &data[0]
		container.AgentHost = notRunningTasks[taskId].AgentHost
		channel := notRunningTasks[taskId].ReturnChannel
		channel <- container
		delete(notRunningTasks, taskId)
		if zookeeperUrl != "" {
			zookeeper.CreateTaskNode(status.TaskId.Value, status.AgentId.Value)
		}
	} else if state == "TASK_FAILED" || state == "TASK_ERROR" {
		processFailedTask(taskId, notRunningTasks, m)
	} else if state == "TASK_LOST" {
		if zookeeperUrl != "" && notRunningTasks[taskId] != nil {
			var agentId = zookeeper.GetAgentIdForTask(taskId)
			Sched.Reconcile(status.TaskId, ID{agentId})
			log.Printf("[-] [SELENOID_MAKES_RECONCILIATION] [%s] [%s]", taskId, agentId)
		} else if status.Reason == "REASON_RECONCILIATION" {
			processFailedTask(taskId, notRunningTasks, m)
		}
	}
}

func processFailedTask(taskId string, notRunningTasks map[string]*Info, m Message) {
	status := m.Update.Status
	msg := "Task with id [" + taskId + "] has been failed by the reason [" + status.Source + "-" + status.State + "-" + status.Message + "]"
	if notRunningTasks[taskId] != nil {
		container := &DockerInfo{ErrorMsg: msg}
		channel := notRunningTasks[taskId].ReturnChannel
		channel <- container
		delete(notRunningTasks, taskId)
	} else {
		log.Panic(msg)
	}
}

func processOffers(m Message, notRunningTasks map[string]*Info) {
	var offersIds []ID
	offers := m.Offers.Offers
	for _, n := range offers {
		offersIds = append(offersIds, n.Id)
	}
	tasksCanRun, resourcesForTasks := getTotalOffersCapacity(offers)
	log.Printf("[-] [MESOS_HAS_RESOURCES_FOR_TASKS] [%d]\n", tasksCanRun)
	var tasks []Task
	if tasksCanRun == 0 {
		log.Println("[-] [MESOS_HAS_NO_RESOURCES_FOR_RUNNING_TASKS]")
		Sched.Decline(offersIds)
	} else {
	Loop:
		for i := 0; i < tasksCanRun; i++ {
			select {
			case task := <-channel:
				notRunningTasks[task.TaskId] = &Info{ReturnChannel: task.ReturnChannel}
				tasks = append(tasks, task)
			default:
				break Loop
			}
		}
		if len(tasks) == 0 {
			Sched.Decline(offersIds)
		} else {
			hostMap := Sched.Accept(resourcesForTasks[:len(tasks)], tasks)
			for k, v := range hostMap {
				notRunningTasks[k].AgentHost = v
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
		MemLimit = 256
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
			resourcesForTasks = append(resourcesForTasks, ResourcesForOneTask{offer.Id, offer.AgentId, offer.Hostname, portRange})
			portsBegin = portsBegin + 2
		}

	}
	return resourcesForTasks
}

func (task Task) SendToMesos() {
	channel <- task
}

func createSchedulerUrl(rawurl string) string {
	flagUrl, err := url.Parse(rawurl)
	if err != nil {
		log.Fatal("Error while parsing url " + rawurl)
	}

	if flagUrl.Scheme == "zk" {
		return zookeeper.DetectMaster(flagUrl) + schedulerPath
	} else {
		return rawurl + schedulerPath
	}
}

func contains(s []string, e string) bool {
	for _, a := range s {
		if a == e {
			return true
		}
	}
	return false
}
