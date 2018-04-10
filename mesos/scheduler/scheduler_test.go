package scheduler

import (
	"fmt"
	. "github.com/aandryashin/matchers"
	"net/http"
	"net/http/httptest"
	"testing"
)

var (
	scheduler *Scheduler
	cpu       Resource
	mem       Resource
	srv       *httptest.Server
	offer     Offer
	resources []Resource
)

func init() {
	srv = httptest.NewServer(handler())
	scheduler = &Scheduler{
		srv.URL,
		"streamID",
		ID{"testFrameworkID_1"},
	}
	offer = Offer{offerID[0],
		agentID,
		srv.URL,
		resources}
}

func handler() http.Handler {
	root := http.NewServeMux()
	root.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Println(r)
		fmt.Println(w)
	})
	return root
}

func TestGetResourcesForTasks(t *testing.T) {
	expectResourcesForTasks := []ResourcesForOneTask{{offerID[0],
		agentID,
		Range{8000, 8001}},
		{offerID[0],
			agentID,
			Range{8002, 8003}}}

	actualGetResourcesForTasks := getResourcesForTasks(offer, 2, []Range{goodRange})
	AssertThat(t, expectResourcesForTasks, EqualTo{actualGetResourcesForTasks})

}
