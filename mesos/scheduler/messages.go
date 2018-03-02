package scheduler

import (
	//"fmt"
	//"encoding/json"
)

//Универсальная структура для хранения  ID
type ID struct {
	Value string `json:"value"`
}

//Структура для хранения данных о контейнере
type Container struct {
	Type string `json:"type"`
	Docker struct {
		Image        string         `json:"image"`
		Network      string         `json:"network"`
		Privileged   bool			`json:"privileged"`
		PortMappings []PortMappings `json:"portMappings"`
	} `json:"docker"`
}

type PortMappings struct {
	ContainerPort int    `json:"containerPort"`
	HostPort      int    `json:"hostPort"`
	Protocol      string `json:"protocol"`
	Name          string `json:"name"`
}

//Резервируемые ресурсы
type ResourcesPort struct {
	Name string `json:"name"`
	Ranges struct {
		Range []Range `json:"range"`
	} `json:"ranges"`
	Role string `json:"role"`
	Type string `json:"type"`
}

type Resources struct {
	Name string `json:"name"`
	Type string `json:"type"`
	Scalar struct {
		Value float64 `json:"value"`
	} `json:"scalar"`
}

type Range struct {
	Begin int `json:"begin"`
	End   int `json:"end"`
}

//Структура для хранения таски запуска
type Launch struct {
	TaskInfos []TaskInfo `json:"task_infos"`
}
type TaskInfo struct {
	Name    string `json:"name"`
	TaskID  ID     `json:"task_id"`
	AgentID ID     `json:"agent_id"`
	Command struct {
		Shell bool `json:"shell"`
	} `json:"command"`
	Container Container `json:"container"`
	Resources []Resources `json:"resources"`
	ResourcesPort []ResourcesPort `json:"resources"`
}

type SubscribeMessage struct {
	Type string `json:"type"`
	Subscribe struct {
		FrameworkInfo struct {
			User  string   `json:"user"`
			Name  string   `json:"name"`
			Roles []string `json:"roles"`
		} `json:"framework_info"`
	} `json:"subscribe"`
}

type DeclineMessage struct {
	FrameworkID ID     `json:"framework_id"`
	Type        string `json:"type"`
	Decline struct {
		OfferIds []ID `json:"offer_ids"`
		Filters struct {
			RefuseSeconds float64 `json:"refuse_seconds"`
		} `json:"filters"`
	} `json:"decline"`
}

type AcceptMessage struct {
	FrameworkID ID     `json:"framework_id"`
	Type        string `json:"type"`
	Accept struct {
		OfferIds []ID `json:"offer_ids"`
		//тут может быть одна или много тасок, надо подумать как их сюда передать
		Operations []Operation //`json:"operations"`
		Filters struct {
			RefuseSeconds float64 `json:"refuse_seconds"`
		} `json:"filters"`
	} `json:"accept"`
}

type Operation struct {
	Launch Launch `json:"launch"`
}

type AcknowledgeMessage struct {
	FrameworkID ID     `json:"framework_id"`
	Type        string `json:"type"`
	Acknowledge struct {
		AgentID ID     `json:"agent_id"`
		TaskID  ID     `json:"task_id"`
		UUID    string `json:"uuid"`
	} `json:"acknowledge"`
}

type KillMessage struct {
	FrameworkID ID     `json:"framework_id"`
	Type        string `json:"type"`
	Kill struct {
		TaskID ID `json:"task_id"`
	} `json:"kill"`
}

func GetAcceptMessage(frameworkId ID, offers []ID, agentId ID) (AcceptMessage) {

	var portMappings = PortMappings{ContainerPort: 4444}
	portMappings.Name = "http"
	portMappings.ContainerPort = 4444
	portMappings.HostPort = 31005
	portMappings.Protocol = "tcp"

	var container = Container{Type: "DOCKER"}
	container.Docker.Image = "selenoid/chrome"
	container.Docker.Network = "BRIDGE"
	container.Docker.Privileged = true
	container.Docker.PortMappings = append(container.Docker.PortMappings, portMappings)

	var rangePort = Range{}
	rangePort.Begin = 31005
	rangePort.End = 31005

	var resourcesPorts = ResourcesPort{Type: "RANGES"}
	resourcesPorts.Name = "ports"
	resourcesPorts.Ranges.Range = append(resourcesPorts.Ranges.Range, rangePort)
	resourcesPorts.Role = "*"

	var resourcesCpu = Resources{Type: "SCALAR"}
	resourcesCpu.Name = "cpus"
	resourcesCpu.Scalar.Value = 1.0

	var resourcesMem = Resources{Type: "SCALAR"}
	resourcesMem.Name = "mem"
	resourcesMem.Scalar.Value = 128.0

	var taskInfo = TaskInfo{}
	taskInfo.Name = "My Task"
	taskInfo.TaskID.Value = "12220-3440-12532-my-task"
	taskInfo.AgentID = agentId
	taskInfo.Command.Shell = false
	taskInfo.Container = container
	taskInfo.Resources = append(taskInfo.Resources, resourcesCpu, resourcesMem)
	taskInfo.ResourcesPort = append(taskInfo.ResourcesPort, resourcesPorts)

	var launch = Launch{}
	launch.TaskInfos = append(launch.TaskInfos, taskInfo)

	var operations = Operation{}
	operations.Launch = launch

	var message = AcceptMessage{FrameworkID: frameworkId,
		Type: "ACCEPT"}
	message.Accept.OfferIds = offers
	message.Accept.Operations = append(message.Accept.Operations, operations)
	message.Accept.Filters.RefuseSeconds = float64(5.0)

	return message
}

func GetSubscribedMessage(user string, name string, roles []string) (SubscribeMessage) {
	var message = SubscribeMessage{Type: "SUBSCRIBE"}
	message.Subscribe.FrameworkInfo.User = user
	message.Subscribe.FrameworkInfo.Name = name
	message.Subscribe.FrameworkInfo.Roles = roles
	return message
}

func GetAcknowledgeMessage(frameworkId ID, agentId ID, UUID string) (AcknowledgeMessage) {
	var message = AcknowledgeMessage{FrameworkID: frameworkId,
		Type: "ACKNOWLEDGE"}
	message.Acknowledge.AgentID = agentId
	message.Acknowledge.TaskID = ID{"12220-3440-12532-my-task"}
	message.Acknowledge.UUID = UUID
	return message
}

func GetDeclineMessage(frameworkId ID, offerId []ID) (DeclineMessage) {
	var message = DeclineMessage{FrameworkID: frameworkId,
		Type: "DECLINE"}
	message.Decline.OfferIds = offerId
	message.Decline.Filters.RefuseSeconds = 5.0
	return message
}

func GetKillMessage(frameworkId ID) (KillMessage) {
	var message = KillMessage{
		FrameworkID: frameworkId,
		Type:        "KILL"}
	message.Kill.TaskID = ID{"12220-3440-12532-my-task"}
	return message
}
