package scheduler

import (
	"fmt"
	"net/http"
	"strings"
	"bytes"
)

const (
	frameworkIdHolder  = "__FRAMEWORK_ID__"
	offerIdsHolder = "__OFFER_IDS__"
	agentIdHolder = "__AGENT_ID__"
	uuidHolder = "__UUID__"
)


func Decline(mesosStreamId string, frameworkId string, offers string)  {

	template := `{
  "framework_id"    : {"value" : "__FRAMEWORK_ID__"},
  "type"            : "DECLINE",
  "decline"         : {
    "offer_ids" : __OFFER_IDS__,
    "filters"   : {"refuse_seconds" : 5.0}
  }
}`
	body := strings.Replace(template, frameworkIdHolder, frameworkId, 1)
	bodyWithOffers := strings.Replace(body, offerIdsHolder, offers, 1)
	req, err := http.NewRequest("POST", schedulerUrl, strings.NewReader(bodyWithOffers))

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

func Accept(mesosStreamId string, frameworkId string, agent_id string, offers string)  {

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
                                  					"image": "docker.moscow.alfaintra.net/selenoid/chrome"
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
	req, err := http.NewRequest("POST", schedulerUrl, strings.NewReader(bodyWithAgent))

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

func Aknowledge(mesosStreamId string, frameworkId string, agent_id string, uuid string)  {
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
    body := strings.Replace(template, frameworkIdHolder, frameworkId, 1);
    bodyWithAgent := strings.Replace(body, agentIdHolder, agent_id, 1);
    bodyWithUuid := strings.Replace(bodyWithAgent, uuidHolder, uuid, 1);
	req, err := http.NewRequest("POST", schedulerUrl, strings.NewReader(bodyWithUuid))

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
