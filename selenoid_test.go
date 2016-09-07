package main

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	. "github.com/aandryashin/matchers"
	. "github.com/aandryashin/matchers/httpresp"
	"github.com/aandryashin/selenoid/handler"
	"github.com/aandryashin/selenoid/service"
)

type StartupError struct{}

func (m *StartupError) StartWithCancel() (*url.URL, func(), error) {
	log.Println("Starting StartupError Service...")
	log.Println("Failed to start StartupError Service...")
	return nil, func() {}, errors.New("Failed to start Service")
}

type With string

func (r With) Path(p string) string {
	return fmt.Sprintf("%s%s", r, p)
}

func TestNewSessionWithGet(t *testing.T) {
	srv := httptest.NewServer(Handler(&service.HttpTest{Handler: handler.Selenium()}, 1))
	defer srv.Close()

	rsp, err := http.Get(With(srv.URL).Path("/wd/hub/session"))
	AssertThat(t, err, Is{nil})
	AssertThat(t, rsp, Code{http.StatusMethodNotAllowed})

	AssertThat(t, len(queue), EqualTo{0})
}

func TestServiceStartupFailure(t *testing.T) {
	srv := httptest.NewServer(Handler(&StartupError{}, 1))
	defer srv.Close()

	rsp, err := http.Post(With(srv.URL).Path("/wd/hub/session"), "", nil)
	AssertThat(t, err, Is{nil})
	AssertThat(t, rsp, Code{http.StatusInternalServerError})

	AssertThat(t, len(queue), EqualTo{0})
}

func TestNewSessionNotFound(t *testing.T) {
	srv := httptest.NewServer(Handler(&service.HttpTest{Handler: handler.Selenium()}, 1))
	defer srv.Close()

	rsp, err := http.Get(With(srv.URL).Path("/wd/hub/session/123"))
	AssertThat(t, err, Is{nil})
	AssertThat(t, rsp, Code{http.StatusNotFound})

	AssertThat(t, len(queue), EqualTo{0})
}

func TestNewSessionHostDown(t *testing.T) {
	canceled := false
	ch := make(chan bool)
	srv := httptest.NewServer(Handler(&service.HttpTest{
		Handler: handler.Selenium(),
		Action: func(s *httptest.Server) {
			log.Println("Host is going down...")
			s.Close()
			log.Println("Now Host is down...")
		},
		Cancel: ch,
	}, 1))
	defer srv.Close()

	rsp, err := http.Post(With(srv.URL).Path("/wd/hub/session"), "", nil)
	AssertThat(t, err, Is{nil})
	AssertThat(t, rsp, Code{http.StatusInternalServerError})

	canceled = <-ch
	AssertThat(t, canceled, Is{true})

	AssertThat(t, len(queue), EqualTo{0})
}

func TestNewSessionBadHostResponse(t *testing.T) {
	canceled := false
	ch := make(chan bool)
	srv := httptest.NewServer(Handler(&service.HttpTest{
		Handler: handler.HttpResponse("Bad Request", http.StatusBadRequest),
		Cancel:  ch,
	}, 1))
	defer srv.Close()

	rsp, err := http.Post(With(srv.URL).Path("/wd/hub/session"), "", nil)
	AssertThat(t, err, Is{nil})
	AssertThat(t, rsp, Code{http.StatusBadRequest})

	canceled = <-ch
	AssertThat(t, canceled, Is{true})

	AssertThat(t, len(queue), EqualTo{0})
}

func TestSessionCreated(t *testing.T) {
	ch := make(chan string)
	srv := httptest.NewServer(Handler(&service.HttpTest{
		Handler: handler.Selenium(),
		Action: func(s *httptest.Server) {
			go func() {
				ch <- s.URL
			}()
		},
	}, 1))
	defer srv.Close()

	resp, err := http.Post(With(srv.URL).Path("/wd/hub/session"), "", nil)
	driverUrl := <-ch
	AssertThat(t, err, Is{nil})
	var sess map[string]string
	AssertThat(t, resp, AllOf{Code{http.StatusOK}, IsJson{&sess}})

	resp, err = http.Get(With(srv.URL).Path("/status"))
	AssertThat(t, err, Is{nil})
	var stat map[string]string
	AssertThat(t, resp, AllOf{Code{http.StatusOK}, IsJson{&stat}})
	AssertThat(t, stat[sess["sessionId"]], EqualTo{driverUrl})

	AssertThat(t, len(queue), EqualTo{1})
}

func TestProxySession(t *testing.T) {
	srv := httptest.NewServer(Handler(&service.HttpTest{Handler: handler.Selenium()}, 1))
	defer srv.Close()

	resp, err := http.Post(With(srv.URL).Path("/wd/hub/session"), "", nil)
	AssertThat(t, err, Is{nil})
	var sess map[string]string
	AssertThat(t, resp, AllOf{Code{http.StatusOK}, IsJson{&sess}})

	resp, err = http.Get(With(srv.URL).Path(fmt.Sprintf("/wd/hub/session/%s/url", sess["sessionId"])))
	AssertThat(t, err, Is{nil})
	AssertThat(t, resp, Code{http.StatusOK})

	AssertThat(t, len(queue), EqualTo{1})
}

func TestSessionDeleted(t *testing.T) {
	canceled := false
	ch := make(chan bool)
	srv := httptest.NewServer(Handler(&service.HttpTest{
		Handler: handler.Selenium(),
		Cancel:  ch,
	}, 1))
	defer srv.Close()

	resp, err := http.Post(With(srv.URL).Path("/wd/hub/session"), "", nil)
	AssertThat(t, err, Is{nil})
	var sess map[string]string
	AssertThat(t, resp, AllOf{Code{http.StatusOK}, IsJson{&sess}})

	req, _ := http.NewRequest(http.MethodDelete,
		With(srv.URL).Path(fmt.Sprintf("/wd/hub/session/%s", sess["sessionId"])), nil)
	http.DefaultClient.Do(req)

	resp, err = http.Get(With(srv.URL).Path("/status"))
	AssertThat(t, err, Is{nil})
	var stat map[string]string
	AssertThat(t, resp, AllOf{Code{http.StatusOK}, IsJson{&stat}})
	_, ok := stat[sess["sessionId"]]
	AssertThat(t, ok, Is{false})

	canceled = <-ch
	AssertThat(t, canceled, Is{true})

	AssertThat(t, len(queue), EqualTo{0})
}

func TestNewSessionTimeout(t *testing.T) {
	canceled := false
	ch := make(chan bool)
	srv := httptest.NewServer(Handler(&service.HttpTest{
		Handler: handler.Selenium(),
		Cancel:  ch,
	}, 1))
	defer srv.Close()

	timeout = 30 * time.Millisecond
	resp, err := http.Post(With(srv.URL).Path("/wd/hub/session"), "", nil)
	AssertThat(t, err, Is{nil})
	var sess map[string]string
	AssertThat(t, resp, AllOf{Code{http.StatusOK}, IsJson{&sess}})

	_, ok := sessions.Get(sess["sessionId"])
	AssertThat(t, ok, Is{true})

	<-time.After(50 * time.Millisecond)
	_, ok = sessions.Get(sess["sessionId"])
	AssertThat(t, ok, Is{false})

	canceled = <-ch
	AssertThat(t, canceled, Is{true})

	AssertThat(t, len(queue), EqualTo{0})
}

func TestProxySessionTimeout(t *testing.T) {
	canceled := false
	ch := make(chan bool)
	srv := httptest.NewServer(Handler(&service.HttpTest{
		Handler: handler.Selenium(),
		Cancel:  ch,
	}, 1))
	defer srv.Close()

	timeout = 30 * time.Millisecond
	resp, err := http.Post(With(srv.URL).Path("/wd/hub/session"), "", nil)
	AssertThat(t, err, Is{nil})
	var sess map[string]string
	AssertThat(t, resp, AllOf{Code{http.StatusOK}, IsJson{&sess}})

	_, ok := sessions.Get(sess["sessionId"])
	AssertThat(t, ok, Is{true})

	<-time.After(20 * time.Millisecond)
	_, ok = sessions.Get(sess["sessionId"])
	AssertThat(t, ok, Is{true})
	http.Get(With(srv.URL).Path(fmt.Sprintf("/wd/hub/session/%s/url", sess["sessionId"])))

	<-time.After(20 * time.Millisecond)
	_, ok = sessions.Get(sess["sessionId"])
	_, ok = sessions.Get(sess["sessionId"])
	AssertThat(t, ok, Is{true})

	<-time.After(50 * time.Millisecond)
	_, ok = sessions.Get(sess["sessionId"])
	AssertThat(t, ok, Is{false})

	canceled = <-ch
	AssertThat(t, canceled, Is{true})

	AssertThat(t, len(queue), EqualTo{0})
}

func TestForceDeleteSession(t *testing.T) {
	canceled := false
	ch := make(chan bool)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/wd/hub/session/") {
			http.Error(w, "Delete session failed", http.StatusInternalServerError)
			return
		}
		Handler(&service.HttpTest{
			Handler: handler.Selenium(),
			Cancel:  ch,
		}, 1).ServeHTTP(w, r)
	}))
	defer srv.Close()

	timeout = 30 * time.Millisecond
	resp, err := http.Post(With(srv.URL).Path("/wd/hub/session"), "", nil)
	AssertThat(t, err, Is{nil})
	var sess map[string]string
	AssertThat(t, resp, AllOf{Code{http.StatusOK}, IsJson{&sess}})

	_, ok := sessions.Get(sess["sessionId"])
	AssertThat(t, ok, Is{true})

	<-time.After(50 * time.Millisecond)
	_, ok = sessions.Get(sess["sessionId"])
	AssertThat(t, ok, Is{false})

	canceled = <-ch
	AssertThat(t, canceled, Is{true})

	AssertThat(t, len(queue), EqualTo{0})
}
