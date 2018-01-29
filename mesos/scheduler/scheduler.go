package scheduler

import (
	"net/http"
	"strings"
	"log"
	"fmt"
	"bufio"
	"encoding/json"
	"container/list"
)

const schedulerUrl = "http://localhost:5050/api/v1/scheduler"

type frameworkId struct {
	Value string
}

type subscribed struct {
	Framework_id               frameworkId
	Heartbeat_interval_seconds int64
}

type offers struct{
	Offers list.List
}

type Message struct {
	Offers     offers
	Subscribed subscribed
	Type       string
}

func Run() {

	resp, err := http.Post(schedulerUrl, "application/json", strings.NewReader("{\n"+
		"   \"type\"       : \"SUBSCRIBE\","+
		"   \"subscribe\"  : {"+
		"      \"framework_info\"  : {"+
		"        \"user\" :  \"foo\","+
		"        \"name\" :  \"My Best Framework\","+
		"        \"roles\": [\"test\"]"+
		"      }"+
		"  }"+
		"}"))
	if err != nil {
		log.Fatal(err)
	}

	defer resp.Body.Close()

	streamId := resp.Header.Get("Mesos-Stream-Id")
	fmt.Println(streamId)

	scanner := bufio.NewScanner(resp.Body)
	scanner.Split(bufio.ScanLines)

	var frameworkId string
	for scanner.Scan() {
		var line = scanner.Text()
		var m Message

		fmt.Println(line)
		var index = strings.LastIndex(line, "}")
		if index != -1 {
			jsonMessage := line[0:index+1]
			json.Unmarshal([]byte(jsonMessage), &m)
			if m.Type == "SUBSCRIBED" {
				frameworkId = m.Subscribed.Framework_id.Value
				fmt.Println("Ура, мы подписались! Id = " + frameworkId)
			} else if m.Type == "HEARTBEAT" {
				fmt.Println("Мезос жил, мезос жив, мезос будет жить!!!")
			} else if m.Type == "OFFERS" {
				Decline(streamId, frameworkId)
			}
		}
	}
}
