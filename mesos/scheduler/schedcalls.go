package scheduler

import (
	"fmt"
	"net/http"
	"strings"
	"bytes"
	"encoding/json"
)

const (
	frameworkIdHolder = "__FRAMEWORK_ID__"
	offerIdsHolder    = "__OFFER_IDS__"
	agentIdHolder     = "__AGENT_ID__"
)

func Decline(mesosStreamId string, frameworkId ID, offersIDs []ID) {

	body, _ := json.Marshal(GetDeclineMessage(frameworkId, offersIDs))
	req, err := http.NewRequest("POST", scheduler.url, bytes.NewReader(body))

	req.Header.Set("Mesos-Stream-Id", mesosStreamId)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		panic(err)
	}

	buf := new(bytes.Buffer)
	buf.ReadFrom(resp.Body)
	fmt.Println(buf.String())
	fmt.Println(resp.Status)
}

func Accept(mesosStreamId string, frameworkId string, agent_id string, offers string) {

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
                                  					"image": "docker.moscow.alfaintra.net/selenoid/chrome",
													"network": "BRIDGE",
													"portMappings": [
														{
														  "containerPort": 4444,
														  "hostPort": 0,
														  "protocol": "tcp",
														  "name": "http"
														}
													]
                               					 }
                              				},
                                          "resources"   : [
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
	body := strings.Replace(template, frameworkIdHolder, frameworkId, 1)
	bodyWithOffers := strings.Replace(body, offerIdsHolder, offers, 1)
	bodyWithAgent := strings.Replace(bodyWithOffers, agentIdHolder, agent_id, 1)
	req, err := http.NewRequest("POST", scheduler.url, strings.NewReader(bodyWithAgent))

	req.Header.Set("Mesos-Stream-Id", mesosStreamId)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		panic(err)
	}

	buf := new(bytes.Buffer)
	buf.ReadFrom(resp.Body)
	fmt.Println(buf.String())
	fmt.Println(resp.Status)
}

type acknowledge struct {
	AgentId ID     `json:"agent_id"`
	TaskId  ID     `json:"task_id"`
	Uuid    string `json:"uuid"`
}

type AcknowledgeResponse struct {
	FrameworkId ID          `json:"framework_id"`
	Type        string      `json:"type"`
	Acknowledge acknowledge `json:"acknowledge"`
}

func Acknowledge(mesosStreamId string, frameworkId ID, agent_id ID, uuid string) {

	body, _ := json.Marshal(GetAcknowledgeMessage(frameworkId, agent_id, uuid))
	req, err := http.NewRequest("POST", scheduler.url, bytes.NewReader(body))

	req.Header.Set("Mesos-Stream-Id", mesosStreamId)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		panic(err)
	}

	buf := new(bytes.Buffer)
	buf.ReadFrom(resp.Body)
	fmt.Println(buf.String())
	fmt.Println(resp.Status)
}
