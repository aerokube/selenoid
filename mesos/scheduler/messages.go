package scheduler

//Универсальная структура для хранения  ID
type ID struct {
	Value string `json:"value"`
}

type Docker struct {
	Image        string          `json:"image"`
	Network      string          `json:"network"`
	Privileged   bool            `json:"privileged"`
	PortMappings *[]PortMappings `json:"port_mappings"`
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

type Tasks struct {
	TaskID  ID `json:"task_id"`
	AgentID ID `json:"agent_id"`
}

type Reconcile struct {
	Task []Tasks `json:"tasks"`
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

type ReconcileMessage struct {
	FrameworkID ID        `json:"framework_id"`
	Type        string    `json:"type"`
	Reconcile   Reconcile `json:"reconcile"`
}

func GetPortMappings(portRange Range) *[]PortMappings {
	var portMappings = PortMappings{ContainerPort: 4444}
	portMappings.Name = "http"
	portMappings.ContainerPort = 4444
	portMappings.HostPort = portRange.Begin
	portMappings.Protocol = "tcp"
	return &[]PortMappings{portMappings}
}

func NewContainer(portRange Range) *Container {
	return &Container{
		Type: "DOCKER",
		Docker: Docker{
			Image:        "selenoid/chrome",
			Network:      "BRIDGE",
			Privileged:   true,
			PortMappings: GetPortMappings(portRange),
		},
	}
}

func NewResourcePorts(portRange Range) Resource {
	var rangePort = Range{
		Begin: portRange.Begin,
		End:   portRange.Begin,
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

func NewLaunchTaskInfo(offer Offer, taskId string) *Launch {
	portRange := offer.Resources[0].Ranges.Range[0]
	var taskInfo = TaskInfo{
		Name:      "My Task",
		TaskID:    ID{taskId},
		AgentID:   offer.AgentId,
		Command:   Command{false},
		Container: NewContainer(portRange),
		Resources: []Resource{
			NewResourcePorts(portRange),
			NewResourcesContainer("cpus", CpuLimit),
			NewResourcesContainer("mem", MemLimit),
		},
	}

	return &Launch{TaskInfos: []TaskInfo{taskInfo}}
}

func NewOperations(offer Offer, taskId string) *[]Operation {
	return &[]Operation{{
		Type:   "LAUNCH",
		Launch: NewLaunchTaskInfo(offer, taskId),
	},
	}
}

func GetAcceptMessage(frameworkId ID, offer Offer, taskId string) (AcceptMessage) {
	offerIds := []ID{offer.Id}
	return AcceptMessage{
		FrameworkID: frameworkId,
		Type:        "ACCEPT",
		Accept: Accept{
			offerIds,
			NewOperations(offer, taskId),
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

func GetAcknowledgeMessage(frameworkId ID, agentId ID, UUID string, taskId ID) (AcknowledgeMessage) {
	return AcknowledgeMessage{
		FrameworkID: frameworkId,
		Type:        "ACKNOWLEDGE",
		Acknowledge: Acknowledge{
			AgentID: agentId,
			TaskID:  taskId,
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

func GetKillMessage(frameworkId ID, taskId string) (KillMessage) {
	return KillMessage{
		FrameworkID: frameworkId,
		Type:        "KILL",
		Kill: Kill{
			TaskID: ID{taskId},
		},
	}
}

func GetReconcileMessage(frameworkId ID, tasks []Tasks) (ReconcileMessage) {
	return ReconcileMessage{
		FrameworkID: frameworkId,
		Type:        "RECONCILE",
		Reconcile: Reconcile {
			Task: tasks,
		},
	}
}
