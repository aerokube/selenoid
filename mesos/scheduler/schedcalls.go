package scheduler

import (
	"fmt"
	"net/http"
	"strings"
	"bytes"
	"log"
	"github.com/pborman/uuid"
)

const (
	frameworkIdHolder = "__FRAMEWORK_ID__"
	offerIdsHolder    = "__OFFER_IDS__"
	agentIdHolder     = "__AGENT_ID__"
	uuidHolder        = "__UUID__"
)

func (s *Scheduler) Decline(offers string) {

	template := `{
  "framework_id"    : {"value" : "__FRAMEWORK_ID__"},
  "type"            : "DECLINE",
  "decline"         : {
    "offer_ids" : __OFFER_IDS__,
    "filters"   : {"refuse_seconds" : 5.0}
  }
}`
	body := strings.Replace(template, frameworkIdHolder, s.FrameworkId, 1)
	bodyWithOffers := strings.Replace(body, offerIdsHolder, offers, 1)

	_, err := s.sendToStream(bodyWithOffers)
	if err != nil {
		panic(err)
	}
}

func (s *Scheduler) Accept(agentId string, offers string) {

	taskId := "selenoid-" + uuid.New()
	fmt.Println("TASK ID: " + taskId)

	template := `{
  "framework_id"   : {"value" : "__FRAMEWORK_ID__"},
  "type"           : "ACCEPT",
  "accept"         : {
    "offer_ids"    : __OFFER_IDS__,
     "operations"  : [
                      {
                       "type"         : "LAUNCH",
                       "launch"       : {
                         "task_infos" : [
                                         {
                                          "name"        : "My Task",
                                          "task_id"     : {"value" : "12220-3440-12532-my-task"},
                                          "agent_id"    : {"value" : "__AGENT_ID__"},
                                          "command": {
                                				"shell": false
                             				 },
										  "container": {
                               					 "type": "DOCKER",
												 "docker": {
                                  					"image": "selenoid/chrome",
													"network": "BRIDGE",
													"privileged": true,
													"port_mappings": [
														{
														  "container_port": 4444,
														  "host_port": 31005,
														  "protocol": "tcp",
														  "name": "http"
														}
													]
                               					 }
                              				},
                                          "resources"   : [
														   {
											"name":"ports",
											"ranges": {
												"range": [
												{"begin":31005,"end":31005}
												]},
											"role":"*",
											"type":"RANGES"
										  },
                                                           {
                                  			"name": "cpus",
                                  			"type": "SCALAR",
                                  			"scalar": {
                                    			"value": 1.0
                                  			}
										  },
                                		  {
                                  			"name": "mem",
                                  			"type": "SCALAR",
                                  			"scalar": {
                                    			"value": 128.0
                                  			}
                               			  }
                                                          ]
                                         }
                                        ]
                       }
                      }
                     ],
     "filters"     : {"refuse_seconds" : 5.0}
  }
}`
	body := strings.Replace(template, frameworkIdHolder, s.FrameworkId, 1)
	bodyWithOffers := strings.Replace(body, offerIdsHolder, offers, 1)
	bodyWithAgent := strings.Replace(bodyWithOffers, agentIdHolder, agentId, 1)

	resp, err := s.sendToStream(bodyWithAgent)
	if err != nil {
		panic(err)
	}

	buf := new(bytes.Buffer)
	buf.ReadFrom(resp.Body)
	fmt.Println(buf.String())
	fmt.Println(resp.Status)
}

type acknowledge struct {
	AgentId id     `json:"agent_id"`
	TaskId  id     `json:"task_id"`
	Uuid    string `json:"uuid"`
}

type AcknowledgeResponse struct {
	FrameworkId id          `json:"framework_id"`
	Type        string      `json:"type"`
	Acknowledge acknowledge `json:"acknowledge"`
}

func (s *Scheduler) Acknowledge(agent_id string, uuid string) {
	template := ` {
                  "framework_id": {
                    "value": "__FRAMEWORK_ID__"
                  },
                  "type": "ACKNOWLEDGE",
                  "acknowledge": {
                    "agent_id": {
                      "value": "__AGENT_ID__"
                    },
                    "task_id": {
                      "value": "12220-3440-12532-my-task"
                    },
                    "uuid": "__UUID__"
                  }
                }`
	body := strings.Replace(template, frameworkIdHolder, s.FrameworkId, 1);
	bodyWithAgent := strings.Replace(body, agentIdHolder, agent_id, 1);
	bodyWithUuid := strings.Replace(bodyWithAgent, uuidHolder, uuid, 1);
	resp, err := s.sendToStream(bodyWithUuid)
	if err != nil {
		panic(err)
	}

	buf := new(bytes.Buffer)
	buf.ReadFrom(resp.Body)
	fmt.Println(buf.String())
	fmt.Println(resp.Status)
}

func (s *Scheduler) Kill() {
	log.Printf("[%d] [REMOVING_CONTAINER] [%s]\n")
	template := ` {
 
  "framework_id": {
    "value": "__FRAMEWORK_ID__"
  },

  "type" : "KILL",
  "kill" : {
    "task_id" : {"value" : "12220-3440-12532-my-task"}
  }
}`
	body := strings.Replace(template, frameworkIdHolder, s.FrameworkId, 1);
	resp, err := s.sendToStream(body)
	if err != nil {
		log.Printf("[FAILED_TO_REMOVE_CONTAINER] [%v]\n", err)
		return
	}
	log.Printf("[%d] [CONTAINER_REMOVED] [%s]\n")

	buf := new(bytes.Buffer)
	buf.ReadFrom(resp.Body)
	fmt.Println(buf.String())
	fmt.Println(resp.Status)
}

func (s *Scheduler) sendToStream(body string) (*http.Response, error) {
	req, err := http.NewRequest("POST", s.Url, strings.NewReader(body))
	if err != nil {
		log.Fatal(err)
	}

	req.Header.Set("Mesos-Stream-Id", s.StreamId)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	return client.Do(req)
}
