package scheduler

import (
	"fmt"
	"net/http"
	"strings"
	"bytes"
)

const frameworkIdHolder  = "__FRAMEWORK_ID__"

func Decline(mesosStreamId string, frameworkId string)  {

	body := "{\n"+
		"  \"framework_id\"    : {\"value\" : \"__FRAMEWORK_ID__\"},\n"+
		"  \"type\"            : \"DECLINE\",\n"+
		"  \"decline\"         : {\n"+
		"    \"offer_ids\" : [\n"+
		"                   {\"value\" : \"12220-3440-12532-O12\"},\n"+
		"                   {\"value\" : \"12220-3440-12532-O13\"}\n"+
		"                  ],\n"+
		"    \"filters\"   : {\"refuse_seconds\" : 5.0}\n"+
		"  }\n"+
		"}"
	strings.Replace(body, frameworkIdHolder, frameworkId, 1)
	fmt.Println(frameworkId + "frameworkId")
	req, err := http.NewRequest("POST", schedulerUrl, strings.NewReader(body))

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
