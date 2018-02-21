package scheduler

import (
	"fmt"
	"net/http"
	"strings"
	"bytes"
	"encoding/json"
	"log"
	"github.com/pborman/uuid"
)

const (
	frameworkIdHolder = "__FRAMEWORK_ID__"
	offerIdsHolder    = "__OFFER_IDS__"
	agentIdHolder     = "__AGENT_ID__"
)

func (s *Scheduler) Decline(offers []ID) {
	body, _ := json.Marshal(GetDeclineMessage(s.FrameworkId, offers))
	_, err := s.sendToStream(body)
	if err != nil {
		panic(err)
	}
}

func (s *Scheduler) Accept(agentId string, offers string) {
	//TO DO: запуск тестов со сгенерированным taskId
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
	body := strings.Replace(template, frameworkIdHolder, s.FrameworkId.Value, 1)
	bodyWithOffers := strings.Replace(body, offerIdsHolder, offers, 1)
	bodyWithAgent := strings.Replace(bodyWithOffers, agentIdHolder, agentId, 1)

	resp, err := s.sendToStream([]byte(bodyWithAgent))
	if err != nil {
		panic(err)
	}

	buf := new(bytes.Buffer)
	buf.ReadFrom(resp.Body)
	fmt.Println(buf.String())
	fmt.Println(resp.Status)
}

func (s *Scheduler) Acknowledge(agentId ID, uuid string) {
	body, _ := json.Marshal(GetAcknowledgeMessage(s.FrameworkId, agentId, uuid))
	_, err := s.sendToStream(body)
	if err != nil {
		panic(err)
	}
}

func (s *Scheduler) Kill() {
	log.Printf("[%d] [REMOVING_CONTAINER] [%s]\n")
	body, _ := json.Marshal(GetKillMessage(s.FrameworkId))
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

func (s *Scheduler) sendToStream(body []byte) (*http.Response, error) {
	req, err := http.NewRequest("POST", s.Url, bytes.NewReader(body))
	if err != nil {
		log.Fatal(err)
	}
	req.Header.Set("Mesos-Stream-Id", s.StreamId)
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{}
	return client.Do(req)
}
