package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/aerokube/selenoid/protect"
	"github.com/gorilla/websocket"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"

	"time"

	"testing"

	. "github.com/aandryashin/matchers"
	"github.com/aerokube/selenoid/service"
	"github.com/aerokube/selenoid/session"
	"github.com/pborman/uuid"
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
		HostPort: session.HostPort{
			Fileserver: u.Host,
			Clipboard:  u.Host,
			VNC:        u.Host,
			Devtools:   u.Host,
		},
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
		if r.FormValue("abort-handler") != "" {
			out := "this call was relayed by the reverse proxy"
			// Setting wrong Content-Length leads to abort handler error
			w.Header().Add("Content-Length", strconv.Itoa(2*len(out)))
			fmt.Fprintln(w, out)
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
	mux.HandleFunc("/testfile", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("test-data"))
	})
	upgrader := websocket.Upgrader{
		CheckOrigin: func(_ *http.Request) bool {
			return true
		},
	}
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Upgrade") != "" {
			c, err := upgrader.Upgrade(w, r, nil)
			if err != nil {
				panic(err)
			}
			defer c.Close()
			for {
				mt, message, err := c.ReadMessage()
				if err != nil {
					break
				}
				type req struct {
					ID uint64 `json:"id"`
				}
				var r req
				err = json.Unmarshal(message, &r)
				if err != nil {
					panic(err)
				}
				output, err := json.Marshal(r)
				if err != nil {
					panic(err)
				}
				err = c.WriteMessage(mt, output)
				if err != nil {
					break
				}
			}
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("test-clipboard-value"))
	})
	return mux
}

func TestProcessExtensionCapabilities(t *testing.T) {
	capsJson := `{
		"version": "57.0",
		"browserName": "firefox",
		"selenoid:options": {
			"name": "ExampleTestName",
			"enableVNC": true,
			"videoFrameRate": 24,
			"env": ["LANG=de_DE.UTF-8"],
			"labels": {"key": "value"}
		}
	}`
	var caps session.Caps
	err := json.Unmarshal([]byte(capsJson), &caps)
	AssertThat(t, err, Is{nil})
	AssertThat(t, caps.Name, EqualTo{"firefox"})
	AssertThat(t, caps.Version, EqualTo{"57.0"})
	AssertThat(t, caps.TestName, EqualTo{""})

	caps.ProcessExtensionCapabilities()
	AssertThat(t, caps.Name, EqualTo{"firefox"})
	AssertThat(t, caps.Version, EqualTo{"57.0"})
	AssertThat(t, caps.TestName, EqualTo{"ExampleTestName"})
	AssertThat(t, caps.VNC, EqualTo{true})
	AssertThat(t, caps.VideoFrameRate, EqualTo{uint16(24)})
	AssertThat(t, caps.Env, EqualTo{[]string{"LANG=de_DE.UTF-8"}})
	AssertThat(t, caps.Labels, EqualTo{map[string]string{"key": "value"}})
}

func TestSumUsedTotalGreaterThanPending(t *testing.T) {
	queue := protect.New(2, false)

	hf := func(_ http.ResponseWriter, _ *http.Request) {
		time.Sleep(50 * time.Millisecond)
	}
	queuedHandlerFunc := queue.Try(queue.Check(queue.Protect(hf)))
	mux := http.NewServeMux()
	mux.HandleFunc("/", queuedHandlerFunc)

	srv := httptest.NewServer(mux)
	defer srv.Close()
	u := srv.URL + "/"

	_, err := http.Get(u)
	AssertThat(t, err, Is{nil})
	AssertThat(t, queue.Pending(), EqualTo{1})
	queue.Create()
	AssertThat(t, queue.Pending(), EqualTo{0})
	AssertThat(t, queue.Used(), EqualTo{1})

	_, err = http.Get(u)
	AssertThat(t, err, Is{nil})
	AssertThat(t, queue.Pending(), EqualTo{1})
	queue.Create()
	AssertThat(t, queue.Pending(), EqualTo{0})
	AssertThat(t, queue.Used(), EqualTo{2})

	req, _ := http.NewRequest(http.MethodGet, u, nil)
	ctx, _ := context.WithTimeout(context.Background(), 100*time.Millisecond)
	req = req.WithContext(ctx)

	_, err = http.DefaultClient.Do(req)
	AssertThat(t, err, Not{nil})
	AssertThat(t, queue.Pending(), EqualTo{0})
	AssertThat(t, queue.Used(), EqualTo{2})
}
