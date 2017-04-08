package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"regexp"
	"strings"
	"time"

	"github.com/aandryashin/selenoid/session"
)

type request struct {
	*http.Request
}

type sess struct {
	addr string
	id   string
}

// TODO There is simpler way to do this
func (r request) localaddr() string {
	addr := r.Context().Value(http.LocalAddrContextKey).(net.Addr).String()
	_, port, _ := net.SplitHostPort(addr)
	return net.JoinHostPort("localhost", port)
}

func (r request) session(id string) *sess {
	return &sess{r.localaddr(), id}
}

func (s *sess) url() string {
	return fmt.Sprintf("http://%s/wd/hub/session/%s", s.addr, s.id)
}

func (s *sess) Delete() {
	log.Printf("[SESSION_TIMED_OUT] [%s]\n", s.id)
	r, err := http.NewRequest(http.MethodDelete, s.url(), nil)
	if err != nil {
		log.Fatalf("[DELETE_FAILED] [%s] [%v]\n", s.id, err)
	}
	resp, err := http.DefaultClient.Do(r)
	if resp != nil {
		defer resp.Body.Close()
	}
	if err == nil && resp.StatusCode == http.StatusOK {
		log.Printf("[SESSION_DELETED] [%s]\n", s.id)
		return
	}
	if err != nil {
		log.Fatalf("[DELETE_FAILED] [%s] [%v]\n", s.id, err)
	} else {
		log.Fatalf("[DELETE_FAILED] [%s] [%s]\n", s.id, resp.Status)
	}
}

func create(w http.ResponseWriter, r *http.Request) {
	quota, _, ok := r.BasicAuth()
	if !ok {
		quota = "unknown"
	}
	body, err := ioutil.ReadAll(r.Body)
	r.Body.Close()
	if err != nil {
		log.Printf("[ERROR_READING_REQUEST] [%s]\n", err.Error())
		http.Error(w, err.Error(), http.StatusBadRequest)
		queue.Drop()
		return
	}
	var browser struct {
		Caps struct {
			Name             string `json:"browserName"`
			Version          string `json:"version"`
			ScreenResolution string `json:"screenResolution"`
		} `json:"desiredCapabilities"`
	}
	err = json.Unmarshal(body, &browser)
	if err != nil {
		log.Printf("[BAD_JSON_FORMAT] [%s]\n", err.Error())
		http.Error(w, err.Error(), http.StatusBadRequest)
		queue.Drop()
		return
	}
	if browser.Caps.ScreenResolution != "" {
		exp := regexp.MustCompile(`^[0-9]+x[0-9]+x(8|16|24)$`)
		if !exp.MatchString(browser.Caps.ScreenResolution) {
			http.Error(w, "Malformed screenResolution capability.", http.StatusBadRequest)
			queue.Drop()
			return
		}
	}
	starter, ok := manager.Find(browser.Caps.Name, &browser.Caps.Version, browser.Caps.ScreenResolution)
	if !ok {
		log.Printf("[ENVIRONMENT_NOT_AVAILABLE] [%s-%s]\n", browser.Caps.Name, browser.Caps.Version)
		http.Error(w, "Requested environment is not available", http.StatusBadRequest)
		queue.Drop()
		return
	}
	u, cancel, err := starter.StartWithCancel()
	if err != nil {
		log.Printf("[SERVICE_STARTUP_FAILED] [%s]\n", err.Error())
		http.Error(w, err.Error(), http.StatusInternalServerError)
		queue.Drop()
		return
	}
	r.URL.Host, r.URL.Path = u.Host, u.Path+r.URL.Path
	req, _ := http.NewRequest(http.MethodPost, r.URL.String(), r.Body)
	if r.ContentLength > 0 {
		req.ContentLength = r.ContentLength
	}
	req.Body = ioutil.NopCloser(bytes.NewReader(body))
	log.Printf("[SESSION_ATTEMPTED] [%s]\n", u.String())
	resp, err := http.DefaultClient.Do(req)
	if resp != nil {
		defer resp.Body.Close()
	}
	if err != nil {
		log.Printf("[SESSION_FAILED] [%s] - [%s]\n", u.String(), err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		queue.Drop()
		cancel()
		return
	}
	w.WriteHeader(resp.StatusCode)
	var s struct {
		Value struct {
			ID string `json:"sessionId"`
		}
		ID string `json:"sessionId"`
	}
	tee := io.TeeReader(resp.Body, w)
	json.NewDecoder(tee).Decode(&s)
	if s.ID == "" {
		s.ID = s.Value.ID
	}
	if s.ID == "" {
		log.Printf("[SESSION_FAILED] Bad response from [%s] - [%v]\n", u.String(), resp.Status)
		queue.Drop()
		cancel()
		return
	}
	sessions.Put(s.ID, &session.Session{
		Quota:   quota,
		Browser: browser.Caps.Name,
		Version: browser.Caps.Version,
		URL:     u,
		Cancel:  cancel,
		Timeout: onTimeout(timeout, func() {
			request{r}.session(s.ID).Delete()
		})})
	queue.Create()
	log.Printf("[SESSION_CREATED] [%s] [%s]\n", s.ID, u)
}

func proxy(w http.ResponseWriter, r *http.Request) {
	done := make(chan func())
	go func(w http.ResponseWriter, r *http.Request) {
		cancel := func() {}
		defer func() {
			done <- cancel
		}()
		(&httputil.ReverseProxy{
			Director: func(r *http.Request) {
				fragments := strings.Split(r.URL.Path, "/")
				id := fragments[2]
				sess, ok := sessions.Get(id)
				if ok {
					r.URL.Host, r.URL.Path = sess.URL.Host, sess.URL.Path+r.URL.Path
					close(sess.Timeout)
					if r.Method == http.MethodDelete && len(fragments) == 3 {
						cancel = sess.Cancel
						sessions.Remove(id)
						queue.Release()
					} else {
						sess.Timeout = onTimeout(timeout, func() {
							request{r}.session(id).Delete()
						})
					}
					return
				}
				r.URL.Path = "/error"
			},
		}).ServeHTTP(w, r)
	}(w, r)
	go (<-done)()
}

func onTimeout(t time.Duration, f func()) chan struct{} {
	cancel := make(chan struct{})
	go func(cancel chan struct{}) {
		select {
		case <-time.After(t):
			f()
		case <-cancel:
		}
	}(cancel)
	return cancel
}
