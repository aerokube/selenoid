package main

import (
	"bytes"
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
	"github.com/aandryashin/selenoid/config"
	"github.com/aandryashin/selenoid/handler"
	"github.com/aandryashin/selenoid/service"
)

type HttpTest struct {
	Handler http.Handler
	Action  func(s *httptest.Server)
	Cancel  chan bool
}

func (m *HttpTest) StartWithCancel() (*url.URL, func(), error) {
	log.Println("Starting HttpTest Service...")
	s := httptest.NewServer(m.Handler)
	u, err := url.Parse(s.URL)
	if err != nil {
		log.Println("Failed to start HttpTest Service...")
		return nil, func() {}, err
	}
	log.Println("HttpTest Service started...")
	if m.Action != nil {
		m.Action(s)
	}
	return u, func() {
		log.Println("Stopping HttpTest Service...")
		s.Close()
		log.Println("HttpTest Service stopped...")
		if m.Cancel != nil {
			go func() {
				m.Cancel <- true
			}()
		}
	}, nil
}

func (m *HttpTest) Find(s string, v *string) (service.Starter, bool) {
	return m, true
}

type StartupError struct{}

func (m *StartupError) StartWithCancel() (*url.URL, func(), error) {
	log.Println("Starting StartupError Service...")
	log.Println("Failed to start StartupError Service...")
	return nil, nil, errors.New("Failed to start Service")
}

func (m *StartupError) Find(s string, v *string) (service.Starter, bool) {
	return m, true
}

type BrowserNotFound struct{}

func (m *BrowserNotFound) Find(s string, v *string) (service.Starter, bool) {
	return nil, false
}

type With string

func (r With) Path(p string) string {
	return fmt.Sprintf("%s%s", r, p)
}

var (
	srv *httptest.Server
)

func init() {
	queue = make(chan struct{}, 1)
	srv = httptest.NewServer(Handler())
}

func TestNewSessionWithGet(t *testing.T) {
	manager = &HttpTest{Handler: handler.Selenium()}

	rsp, err := http.Get(With(srv.URL).Path("/wd/hub/session"))
	AssertThat(t, err, Is{nil})
	AssertThat(t, rsp, Code{http.StatusMethodNotAllowed})

	AssertThat(t, len(queue), EqualTo{0})
}

func TestBadJsonFormat(t *testing.T) {
	rsp, err := http.Post(With(srv.URL).Path("/wd/hub/session"), "", nil)
	AssertThat(t, err, Is{nil})
	AssertThat(t, rsp, Code{http.StatusBadRequest})

	AssertThat(t, len(queue), EqualTo{0})
}

func TestServiceStartupFailure(t *testing.T) {
	manager = &StartupError{}

	rsp, err := http.Post(With(srv.URL).Path("/wd/hub/session"), "", bytes.NewReader([]byte("{}")))
	AssertThat(t, err, Is{nil})
	AssertThat(t, rsp, Code{http.StatusInternalServerError})

	AssertThat(t, len(queue), EqualTo{0})
}

func TestBrowserNotFound(t *testing.T) {
	manager = &BrowserNotFound{}

	rsp, err := http.Post(With(srv.URL).Path("/wd/hub/session"), "", bytes.NewReader([]byte("{}")))
	AssertThat(t, err, Is{nil})
	AssertThat(t, rsp, Code{http.StatusBadRequest})

	AssertThat(t, len(queue), EqualTo{0})
}

func TestNewSessionNotFound(t *testing.T) {
	manager = &HttpTest{Handler: handler.Selenium()}

	rsp, err := http.Get(With(srv.URL).Path("/wd/hub/session/123"))
	AssertThat(t, err, Is{nil})
	AssertThat(t, rsp, Code{http.StatusNotFound})

	AssertThat(t, len(queue), EqualTo{0})
}

func TestNewSessionHostDown(t *testing.T) {
	canceled := false
	ch := make(chan bool)
	manager = &HttpTest{
		Handler: handler.Selenium(),
		Action: func(s *httptest.Server) {
			log.Println("Host is going down...")
			s.Close()
			log.Println("Now Host is down...")
		},
		Cancel: ch,
	}

	rsp, err := http.Post(With(srv.URL).Path("/wd/hub/session"), "", bytes.NewReader([]byte("{}")))
	AssertThat(t, err, Is{nil})
	AssertThat(t, rsp, Code{http.StatusInternalServerError})

	canceled = <-ch
	AssertThat(t, canceled, Is{true})

	AssertThat(t, len(queue), EqualTo{0})
}

func TestNewSessionBadHostResponse(t *testing.T) {
	canceled := false
	ch := make(chan bool)
	manager = &HttpTest{
		Handler: handler.HttpResponse("Bad Request", http.StatusBadRequest),
		Cancel:  ch,
	}

	rsp, err := http.Post(With(srv.URL).Path("/wd/hub/session"), "", bytes.NewReader([]byte("{}")))
	AssertThat(t, err, Is{nil})
	AssertThat(t, rsp, Code{http.StatusBadRequest})

	canceled = <-ch
	AssertThat(t, canceled, Is{true})

	AssertThat(t, len(queue), EqualTo{0})
}

func TestSessionCreated(t *testing.T) {
	manager = &HttpTest{Handler: handler.Selenium()}

	resp, err := http.Post(With(srv.URL).Path("/wd/hub/session"), "", bytes.NewReader([]byte("{}")))
	AssertThat(t, err, Is{nil})
	var sess map[string]string
	AssertThat(t, resp, AllOf{Code{http.StatusOK}, IsJson{&sess}})

	resp, err = http.Get(With(srv.URL).Path("/status"))
	AssertThat(t, err, Is{nil})
	var state config.State
	AssertThat(t, resp, AllOf{Code{http.StatusOK}, IsJson{&state}})
	AssertThat(t, state.Used, EqualTo{1})
	AssertThat(t, len(queue), EqualTo{1})
	sessions.Remove(sess["sessionId"])
	<-queue
}

func TestProxySession(t *testing.T) {
	manager = &HttpTest{Handler: handler.Selenium()}

	resp, err := http.Post(With(srv.URL).Path("/wd/hub/session"), "", bytes.NewReader([]byte("{}")))
	AssertThat(t, err, Is{nil})
	var sess map[string]string
	AssertThat(t, resp, AllOf{Code{http.StatusOK}, IsJson{&sess}})

	resp, err = http.Get(With(srv.URL).Path(fmt.Sprintf("/wd/hub/session/%s/url", sess["sessionId"])))
	AssertThat(t, err, Is{nil})
	AssertThat(t, resp, Code{http.StatusOK})

	AssertThat(t, len(queue), EqualTo{1})
	sessions.Remove(sess["sessionId"])
	<-queue
}

func TestSessionDeleted(t *testing.T) {
	canceled := false
	ch := make(chan bool)
	manager = &HttpTest{
		Handler: handler.Selenium(),
		Cancel:  ch,
	}

	resp, err := http.Post(With(srv.URL).Path("/wd/hub/session"), "", bytes.NewReader([]byte("{}")))
	AssertThat(t, err, Is{nil})
	var sess map[string]string
	AssertThat(t, resp, AllOf{Code{http.StatusOK}, IsJson{&sess}})

	req, _ := http.NewRequest(http.MethodDelete,
		With(srv.URL).Path(fmt.Sprintf("/wd/hub/session/%s", sess["sessionId"])), nil)
	http.DefaultClient.Do(req)

	resp, err = http.Get(With(srv.URL).Path("/status"))
	AssertThat(t, err, Is{nil})
	var state config.State
	AssertThat(t, resp, AllOf{Code{http.StatusOK}, IsJson{&state}})
	AssertThat(t, state.Used, EqualTo{0})

	canceled = <-ch
	AssertThat(t, canceled, Is{true})

	AssertThat(t, len(queue), EqualTo{0})
}

func TestNewSessionTimeout(t *testing.T) {
	canceled := false
	ch := make(chan bool)
	manager = &HttpTest{
		Handler: handler.Selenium(),
		Cancel:  ch,
	}

	timeout = 30 * time.Millisecond
	resp, err := http.Post(With(srv.URL).Path("/wd/hub/session"), "", bytes.NewReader([]byte("{}")))
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
	manager = &HttpTest{
		Handler: handler.Selenium(),
		Cancel:  ch,
	}

	timeout = 30 * time.Millisecond
	resp, err := http.Post(With(srv.URL).Path("/wd/hub/session"), "", bytes.NewReader([]byte("{}")))
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
	manager = &HttpTest{
		Handler: handler.Selenium(),
		Cancel:  ch,
	}
	selenium := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/wd/hub/session/") {
			http.Error(w, "Delete session failed", http.StatusInternalServerError)
			return
		}
		Handler().ServeHTTP(w, r)
	}))
	defer selenium.Close()

	timeout = 30 * time.Millisecond
	resp, err := http.Post(With(selenium.URL).Path("/wd/hub/session"), "", bytes.NewReader([]byte("{}")))
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
