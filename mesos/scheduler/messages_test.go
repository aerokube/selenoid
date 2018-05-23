package scheduler

import (
	. "github.com/aandryashin/matchers"
	"testing"
)

var (
	frameworkID          ID
	taskID               ID
	offerID              []ID
	UUID                 string
	agentID              ID
	tasks                []Task
	goodRange            Range
	containerPortMapping PortMappings
	vncPortMapping       PortMappings
	vncContainer         Container
)

func init() {
	frameworkID = ID{"testID_1"}
	taskID = ID{"taskID_1"}
	offerID = []ID{{"offerID_1"}, {"offerID_2"}}
	UUID = "123456ID"
	agentID = ID{"agentID_1"}
	goodRange = Range{8000, 8010}
	initVncPortMapping()
	initContainerPortMapping()
	initTasks()
	initVncContainer()
}

func initVncPortMapping() {
	vncPortMapping = PortMappings{
		5900,
		goodRange.End,
		"tcp",
		"http",
	}
}

func initContainerPortMapping() {
	containerPortMapping = PortMappings{
		4444,
		goodRange.Begin,
		"tcp",
		"http",
	}
}

func initTasks() {
	tasks = []Task{{"taskForTestWithVNC",
		"imageForTest",
		true,
		make(chan *DockerInfo), Env{}},
		{"taskForTestWithoutVNC",
			"imageForTest",
			false,
			make(chan *DockerInfo), Env{}}}
}

func initVncContainer() {
	vncContainer = Container{
		"DOCKER",
		Docker{
			tasks[0].Image,
			"BRIDGE",
			true,
			&[]PortMappings{containerPortMapping, vncPortMapping},
		},
	}
}

func TestNewKillMessage(t *testing.T) {
	expectKillMessage := KillMessage{frameworkID,
		"KILL",
		Kill{taskID}}

	actualKillMsg := newKillMessage(frameworkID, taskID.Value)
	AssertThat(t, expectKillMessage, EqualTo{actualKillMsg})

}

func TestNewDeclineMessage(t *testing.T) {
	expectDeclineMessage := DeclineMessage{frameworkID,
		"DECLINE",
		Decline{offerID,
			Filters{1.0}}}

	actualDeclineMessage := newDeclineMessage(frameworkID, offerID)
	AssertThat(t, expectDeclineMessage, EqualTo{actualDeclineMessage})
}

func TestNewAcknowledgeMessage(t *testing.T) {
	expectAcknowledgeMessage := AcknowledgeMessage{
		frameworkID,
		"ACKNOWLEDGE",
		Acknowledge{
			agentID,
			taskID,
			UUID,
		},
	}

	actualAcknowledgeMessage := newAcknowledgeMessage(frameworkID, agentID, UUID, taskID)
	AssertThat(t, expectAcknowledgeMessage, EqualTo{actualAcknowledgeMessage})

}

func TestNewSubscribedMessage(t *testing.T) {
	user := "testUser"
	name := "testName"

	expectSubscribedMessage := SubscribeMessage{
		"SUBSCRIBE",
		Subscribe{
			FrameworkInfo{
				user,
				name,
			},
		},
	}

	actualSubscribeMessage := newSubscribedMessage(user, name)
	AssertThat(t, expectSubscribedMessage, EqualTo{actualSubscribeMessage})
}

func TestNewPortMappingsWithVNC(t *testing.T) {
	expectPortMapping := &[]PortMappings{containerPortMapping, vncPortMapping}
	actualPortMapping := newPortMappings(goodRange, true)
	AssertThat(t, expectPortMapping, EqualTo{actualPortMapping})
}

func TestNewPortMappingsWithoutVNC(t *testing.T) {
	expectPortMapping := &[]PortMappings{containerPortMapping}
	actualPortMapping := newPortMappings(goodRange, false)
	AssertThat(t, expectPortMapping, EqualTo{actualPortMapping})
}

func TestNewContainer(t *testing.T) {
	expectContainer := &vncContainer
	actualContainer := newContainer(goodRange, tasks[0])
	AssertThat(t, expectContainer, EqualTo{actualContainer})
}
