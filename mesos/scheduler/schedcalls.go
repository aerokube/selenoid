package scheduler

import (
	"fmt"
	"net/http"
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

func (s *Scheduler) Accept(agentId ID, offers []ID) {
	//TO DO: запуск тестов со сгенерированным taskId
	taskId := "selenoid-" + uuid.New()
	fmt.Println("TASK ID: " + taskId)
	fmt.Println("offers: ")
	for range offers {
		fmt.Println(offers)
	}
	fmt.Println("agentID: " + agentId.Value)

	body, _ := json.Marshal(GetAcceptMessage(s.FrameworkId, offers, agentId))

	fmt.Println(string(body))

	resp , err := s.sendToStream(body)
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
