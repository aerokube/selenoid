package scheduler

import (
	"fmt"
	. "github.com/aandryashin/matchers"
	"net/http"
	"net/http/httptest"
	"testing"
)

var (
	scheduler     *Scheduler
	cpu           Resource
	mem           Resource
	srv           *httptest.Server
	offer         Offer
	resources     []Resource
	resourcePorts Resource
)

func init() {
	srv = httptest.NewServer(handler())
	scheduler = &Scheduler{
		srv.URL,
		"streamID",
		ID{"testFrameworkID_1"},
	}

	resourcePorts = Resource{
		Type: "RANGES",
		Name: "ports",
		Ranges: &Ranges{
			[]Range{goodRange},
		},
		Role: "*",
	}

	cpu = Resource{
		Type:   "SCALAR",
		Name:   "cpus",
		Scalar: &Scalar{1.0},
	}

	mem = Resource{
		Type:   "SCALAR",
		Name:   "mem",
		Scalar: &Scalar{512.0},
	}

	resources = []Resource{cpu, mem, resourcePorts}

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

func TestGetCapacityOfCurrentOffer(t *testing.T) {
	CpuLimit = 0.2
	MemLimit = 128
	expectCapacity := 4
	actualCapacityOfCurrentOffer, actualResourcesForTasks := getCapacityOfCurrentOffer(offer)
	AssertThat(t, expectCapacity, EqualTo{actualCapacityOfCurrentOffer})
	AssertThat(t, expectCapacity, EqualTo{len(actualResourcesForTasks)})
}

func TestGetTotalOffersCapacity(t *testing.T) {
	CpuLimit = 0.2
	MemLimit = 128
	expectCapacity := 8
	actualTotalOffersCapacity, actualResourcesForTasks := getTotalOffersCapacity([]Offer{offer, offer})
	AssertThat(t, expectCapacity, EqualTo{actualTotalOffersCapacity})
	AssertThat(t, expectCapacity, EqualTo{len(actualResourcesForTasks)})
}
