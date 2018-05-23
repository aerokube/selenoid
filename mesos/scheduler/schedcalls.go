package scheduler

import (
	"fmt"
	"net/http"
	"bytes"
	"encoding/json"
	"log"
)

func (s *Scheduler) Decline(offers []ID) {
	log.Printf("[-] [SELENOID_DECLINED_CURRENT_OFFERS]")
	body, _ := json.Marshal(newDeclineMessage(s.FrameworkId, offers))
	_, err := s.sendToStream(body)
	if err != nil {
		panic(err)
	}
}

func (s *Scheduler) Accept(resources []ResourcesForOneTask, tasks []Task) map[string] string {
	acceptMessage, hostMap := s.newAcceptMessage(resources, tasks)
	body, _ := json.Marshal(acceptMessage)

	fmt.Println(string(body))

	resp, err := s.sendToStream(body)
	if err != nil {
		panic(err)
	}

	buf := new(bytes.Buffer)
	buf.ReadFrom(resp.Body)
	fmt.Println(buf.String())
	fmt.Println(resp.Status)
	return hostMap
}

func (s *Scheduler) Acknowledge(agentId ID, uuid string, taskId ID) {
	body, _ := json.Marshal(newAcknowledgeMessage(s.FrameworkId, agentId, uuid, taskId))
	_, err := s.sendToStream(body)
	if err != nil {
		panic(err)
	}
}

func (s *Scheduler) Kill(requestId uint64, taskId string) {
	log.Printf("[%d] [REMOVING_CONTAINER] [%s]\n", requestId, taskId)
	body, _ := json.Marshal(newKillMessage(s.FrameworkId, taskId))
	resp, err := s.sendToStream(body)
	if err != nil {
		log.Printf("[FAILED_TO_REMOVE_CONTAINER] [%v]\n", err)
		return
	}
	log.Printf("[%d] [CONTAINER_REMOVED] [%s]\n", requestId, taskId)

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

func (s *Scheduler) Reconcile(taskId ID, agentId ID) {
	body, _ := json.Marshal(newReconcileMessage(s.FrameworkId, taskId, agentId))
	fmt.Println(string(body))
	_, err := s.sendToStream(body)
	if err != nil {
		panic(err)
	}
}
