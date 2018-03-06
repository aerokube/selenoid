package scheduler

//Универсальная структура для хранения  ID
type ID struct {
	Value string `json:"value"`
}

type Docker struct {
	Image        string         `json:"image"`
	Network      string         `json:"network"`
	Privileged   bool           `json:"privileged"`
	PortMappings []PortMappings `json:"port_mappings"`
}

//Структура для хранения данных о контейнере
type Container struct {
	Type   string `json:"type"`
	Docker Docker `json:"docker"`
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
	Ranges *Ranges `json:"ranges,omitempty"`
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

type Command struct {
	Shell bool `json:"shell"`
}

type TaskInfo struct {
	Name      string     `json:"name"`
	TaskID    ID         `json:"task_id"`
	AgentID   ID         `json:"agent_id"`
	Command   Command    `json:"command"`
	Container *Container `json:"container"`
	Resources []Resource `json:"resources"`
}

type FrameworkInfo struct {
	User  string   `json:"user"`
	Name  string   `json:"name"`
	Roles []string `json:"roles"`
}

type Subscribe struct {
	FrameworkInfo FrameworkInfo `json:"framework_info"`
}

type SubscribeMessage struct {
	Type      string    `json:"type"`
	Subscribe Subscribe `json:"subscribe"`
}

type Decline struct {
	OfferIds []ID    `json:"offer_ids"`
	Filters  Filters `json:"filters"`
}

type DeclineMessage struct {
	FrameworkID ID      `json:"framework_id"`
	Type        string  `json:"type"`
	Decline     Decline `json:"decline"`
}

type Filters struct {
	RefuseSeconds float64 `json:"refuse_seconds"`
}

type Accept struct {
	OfferIds   []ID         `json:"offer_ids"`
	Operations *[]Operation `json:"operations"`
	Filters    Filters      `json:"filters"`
}

type AcceptMessage struct {
	FrameworkID ID     `json:"framework_id"`
	Type        string `json:"type"`
	Accept      Accept `json:"accept"`
}

type Operation struct {
	Type   string  `json:"type"`
	Launch *Launch `json:"launch"`
}

type AcknowledgeMessage struct {
	FrameworkID ID          `json:"framework_id"`
	Type        string      `json:"type"`
	Acknowledge Acknowledge `json:"acknowledge"`
}

type Acknowledge struct {
	AgentID ID     `json:"agent_id"`
	TaskID  ID     `json:"task_id"`
	UUID    string `json:"uuid"`
}

type Kill struct {
	TaskID ID `json:"task_id"`
}

type KillMessage struct {
	FrameworkID ID     `json:"framework_id"`
	Type        string `json:"type"`
	Kill        Kill   `json:"kill"`
}

func GetPortMappings() ([]PortMappings) {
	var portMappings = PortMappings{ContainerPort: 4444}
	portMappings.Name = "http"
	portMappings.ContainerPort = 4444
	portMappings.HostPort = 31005
	portMappings.Protocol = "tcp"
	return []PortMappings{portMappings}
}

func NewContainer() *Container {
	return &Container{
		Type: "DOCKER",
		Docker: Docker{
			Image:        "selenoid/chrome",
			Network:      "BRIDGE",
			Privileged:   true,
			PortMappings: GetPortMappings(),
		},
	}
}

func NewResourcePorts() Resource {
	var rangePort = Range{
		Begin: 31005,
		End:   31006,
	}

	return Resource{
		Type: "RANGES",
		Name: "ports",
		Ranges: &Ranges{
			[]Range{rangePort},
		},
		Role: "*",
	}
}

func NewResourcesContainer(name string, value float64) Resource {
	return Resource{
		Type:   "SCALAR",
		Name:   name,
		Scalar: &Scalar{value},
	}
}

func NewLaunchTaskInfo(agentId ID) *Launch {
	var taskInfo = TaskInfo{
		Name:      "My Task",
		TaskID:    ID{"12220-3440-12532-my-task"},
		AgentID:   agentId,
		Command:   Command{false},
		Container: NewContainer(),
		Resources: []Resource{
			NewResourcePorts(),
			NewResourcesContainer("cpus", 1.0),
			NewResourcesContainer("mem", 128.0),
		},
	}

	return &Launch{TaskInfos: []TaskInfo{taskInfo}}
}

func NewOperations(agentId ID) *[]Operation {
	return &[]Operation{{
		Type:   "LAUNCH",
		Launch: NewLaunchTaskInfo(agentId),
	},
	}
}

func GetAcceptMessage(frameworkId ID, offers []ID, agentId ID) (AcceptMessage) {
	return AcceptMessage{
		FrameworkID: frameworkId,
		Type:        "ACCEPT",
		Accept: Accept{
			offers,
			NewOperations(agentId),
			Filters{RefuseSeconds: 5.0},
		},
	}
}

func GetSubscribedMessage(user string, name string, roles []string) (SubscribeMessage) {
	return SubscribeMessage{
		Type: "SUBSCRIBE",
		Subscribe: Subscribe{
			FrameworkInfo{
				User:  user,
				Name:  name,
				Roles: roles,
			},
		},
	}
}

func GetAcknowledgeMessage(frameworkId ID, agentId ID, UUID string) (AcknowledgeMessage) {
	return AcknowledgeMessage{
		FrameworkID: frameworkId,
		Type:        "ACKNOWLEDGE",
		Acknowledge: Acknowledge{
			AgentID: agentId,
			TaskID:  ID{"12220-3440-12532-my-task"},
			UUID:    UUID,
		},
	}
}

func GetDeclineMessage(frameworkId ID, offerId []ID) (DeclineMessage) {
	return DeclineMessage{
		FrameworkID: frameworkId,
		Type:        "DECLINE",
		Decline: Decline{
			OfferIds: offerId,
			Filters: Filters{
				RefuseSeconds: 5.0,
			},
		},
	}
}

func GetKillMessage(frameworkId ID) (KillMessage) {
	return KillMessage{
		FrameworkID: frameworkId,
		Type:        "KILL",
		Kill: Kill{
			TaskID: ID{"12220-3440-12532-my-task"},
		},
	}
}
