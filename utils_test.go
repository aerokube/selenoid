package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"

	"time"

	. "github.com/aandryashin/matchers"
	"github.com/aerokube/selenoid/service"
	"github.com/aerokube/selenoid/session"
	"github.com/pborman/uuid"
	"testing"
)

type HTTPTest struct {
	Handler http.Handler
	Action  func(s *httptest.Server)
	Cancel  chan bool
}

func HTTPResponse(msg string, status int) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, msg, status)
	})
}

func (m *HTTPTest) StartWithCancel() (*service.StartedService, error) {
	log.Println("Starting HTTPTest Service...")
	s := httptest.NewServer(m.Handler)
	u, err := url.Parse(s.URL)
	if err != nil {
		log.Println("Failed to start HTTPTest Service...")
		return nil, err
	}
	log.Println("HTTPTest Service started...")
	if m.Action != nil {
		m.Action(s)
	}
	ss := service.StartedService{
		Url: u,
		Cancel: func() {
			log.Println("Stopping HTTPTest Service...")
			s.Close()
			log.Println("HTTPTest Service stopped...")
			if m.Cancel != nil {
				go func() {
					m.Cancel <- true
				}()
			}
		},
	}
	return &ss, nil
}

func (m *HTTPTest) Find(caps session.Caps, requestId uint64) (service.Starter, bool) {
	return m, true
}

type StartupError struct{}

func (m *StartupError) StartWithCancel() (*service.StartedService, error) {
	log.Println("Starting StartupError Service...")
	log.Println("Failed to start StartupError Service...")
	return nil, errors.New("Failed to start Service")
}

func (m *StartupError) Find(caps session.Caps, requestId uint64) (service.Starter, bool) {
	return m, true
}

type BrowserNotFound struct{}

func (m *BrowserNotFound) Find(caps session.Caps, requestId uint64) (service.Starter, bool) {
	return nil, false
}

type With string

func (r With) Path(p string) string {
	return fmt.Sprintf("%s%s", r, p)
}

func Selenium() http.Handler {
	var lock sync.RWMutex
	sessions := make(map[string]struct{})
	mux := http.NewServeMux()
	mux.HandleFunc("/session", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		u := uuid.New()
		lock.Lock()
		sessions[u] = struct{}{}
		lock.Unlock()
		json.NewEncoder(w).Encode(struct {
			S string `json:"sessionId"`
		}{u})
	})
	mux.HandleFunc("/session/", func(w http.ResponseWriter, r *http.Request) {
		u := strings.Split(r.URL.Path, "/")[2]
		lock.RLock()
		_, ok := sessions[u]
		lock.RUnlock()
		if !ok {
			http.Error(w, "Session not found", http.StatusNotFound)
			return
		}
		d, _ := time.ParseDuration(r.FormValue("timeout"))
		if r.Method != http.MethodDelete {
			<-time.After(d)
			return
		}
		lock.Lock()
		delete(sessions, u)
		lock.Unlock()
	})
	return mux
}

func TestProcessExtensionCapabilities(t *testing.T) {
	capsJson := `{
		"browserName": "firefox", "browserVersion": "57.0",
		"selenoid:options": {
			"name": "ExampleTestName",
			"enableVNC": true,
			"enableVideo": "true",
			"videoFrameRate": 24
		}
	}`
	var caps session.Caps
	err := json.Unmarshal([]byte(capsJson), &caps)
	AssertThat(t, err, Is{nil})
	AssertThat(t, caps.Name, EqualTo{"firefox"})
	AssertThat(t, caps.Version, EqualTo{""})
	AssertThat(t, caps.TestName, EqualTo{""})
	caps.ProcessExtensionCapabilities()
	AssertThat(t, caps.Version, EqualTo{"57.0"})
	AssertThat(t, caps.TestName, EqualTo{"ExampleTestName"})
	AssertThat(t, caps.VNC, EqualTo{true})    //Correct type
	AssertThat(t, caps.Video, EqualTo{false}) //Wrong type in JSON
	AssertThat(t, caps.VideoFrameRate, EqualTo{uint16(24)})
}
