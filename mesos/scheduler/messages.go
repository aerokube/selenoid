package scheduler

//Универсальная структура для хранения  ID
type ID struct {
	Value string `json:"value"`
}

//Структура для хранения данных о контейнере
type Container struct {
	Type string `json:"type"`
	Docker struct {
		Image   string `json:"image"`
		Network string `json:"network"`
		PortMappings []struct {
			ContainerPort int    `json:"containerPort"`
			HostPort      int    `json:"hostPort"`
			Protocol      string `json:"protocol"`
			Name          string `json:"name"`
		} `json:"portMappings"`
	} `json:"docker"`
}

//Резервируемые ресурсы
type Resources []struct {
	Name string `json:"name"`
	Type string `json:"type"`
	Scalar struct {
		Value float64 `json:"value"`
	} `json:"scalar"`
}

//Структура для хранения таски запуска
type Launch struct {
	TaskInfos []struct {
		Name    string `json:"name"`
		TaskID  ID     `json:"task_id"`
		AgentID ID     `json:"agent_id"`
		Command struct {
			Shell bool `json:"shell"`
		} `json:"command"`
		Container Container `json:"container"`
		Resources Resources `json:"resources"`
	} `json:"task_infos"`
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
		OfferIds string `json:"offer_ids"`
		Filters struct {
			RefuseSeconds float64 `json:"refuse_seconds"`
		} `json:"filters"`
	} `json:"decline"`
}

type AcceptMessage struct {
	FrameworkID ID     `json:"framework_id"`
	Type        string `json:"type"`
	Accept struct {
		OfferIds string `json:"offer_ids"`
		//тут может быть одна или много тасок, надо подумать как их сюда передать
		Operations []struct {
			Type   string `json:"type"`
			Launch Launch `json:"launch"`
		} `json:"operations"`
		Filters struct {
			RefuseSeconds float64 `json:"refuse_seconds"`
		} `json:"filters"`
	} `json:"accept"`
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
