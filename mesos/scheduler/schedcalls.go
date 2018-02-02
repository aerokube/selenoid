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
