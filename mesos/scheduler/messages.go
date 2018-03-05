package scheduler

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
		Privileged   bool           `json:"privileged"`
		PortMappings []PortMappings `json:"port_mappings"`
	} `json:"docker"`
}

type PortMappings struct {
	ContainerPort int    `json:"container_port"`
	HostPort      int    `json:"host_port"`
	Protocol      string `json:"protocol"`
	Name          string `json:"name"`
}

//Резервируемые ресурсы
type Resource struct {
	Name   string  `json:"name"`
	Ranges *Ranges  `json:"ranges,omitempty"`
	Role   string  `json:"role,omitempty"`
	Type   string  `json:"type"`
	Scalar *Scalar `json:"scalar,omitempty"`
}

type Scalar struct {
	Value float64 `json:"value,numbers"`
}

type Ranges struct {
	Range [] Range `json:"range"`
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
	Container Container  `json:"container"`
	Resources []Resource `json:"resources"`
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
		Operations []Operation `json:"operations"`
		Filters struct {
			RefuseSeconds float64 `json:"refuse_seconds"`
		} `json:"filters"`
	} `json:"accept"`
}

type Operation struct {
	Type   string `json:"type"`
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
	rangePort.End = 31006

	var ranges = Ranges{[]Range{rangePort}}

	var resourcesPorts = Resource{Type: "RANGES"}
	resourcesPorts.Name = "ports"
	resourcesPorts.Ranges = &ranges
	resourcesPorts.Role = "*"

	var resourcesCpu = Resource{Type: "SCALAR"}
	resourcesCpu.Name = "cpus"
	resourcesCpu.Scalar = &Scalar{1.0}

	var resourcesMem = Resource{Type: "SCALAR"}
	resourcesMem.Name = "mem"
	resourcesMem.Scalar = &Scalar{128.0}

	var taskInfo = TaskInfo{}
	taskInfo.Name = "My Task"
	taskInfo.TaskID.Value = "12220-3440-12532-my-task"
	taskInfo.AgentID = agentId
	taskInfo.Command.Shell = false
	taskInfo.Container = container
	taskInfo.Resources = append(taskInfo.Resources, resourcesPorts, resourcesCpu, resourcesMem)
	//taskInfo.Resources = "__RESOURCE__"

	var launch = Launch{}
	launch.TaskInfos = append(launch.TaskInfos, taskInfo)

	var operations = Operation{}
	operations.Type = "LAUNCH"
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
