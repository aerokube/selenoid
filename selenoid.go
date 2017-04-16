package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"path"
	"regexp"
	"strings"
	"time"

	"github.com/aerokube/selenoid/session"
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
	return net.JoinHostPort("127.0.0.1", port)
}

func (r request) session(id string) *sess {
	return &sess{r.localaddr(), id}
}

func (s *sess) url() string {
	return fmt.Sprintf("http://%s/wd/hub/session/%s", s.addr, s.id)
}

func jsonError(w http.ResponseWriter, msg string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(
		map[string]interface{}{
			"value": map[string]string{
				"message": msg,
			},
			"status": 13,
		})
}

func (s *sess) Delete() {
	log.Printf("[SESSION_TIMED_OUT] [%s]\n", s.id)
	r, err := http.NewRequest(http.MethodDelete, s.url(), nil)
	if err != nil {
		log.Printf("[DELETE_FAILED] [%s] [%v]\n", s.id, err)
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	resp, err := http.DefaultClient.Do(r.WithContext(ctx))
	if resp != nil {
		defer resp.Body.Close()
	}
	if err == nil && resp.StatusCode == http.StatusOK {
		return
	}
	if err != nil {
		log.Printf("[DELETE_FAILED] [%s] [%v]\n", s.id, err)
	} else {
		log.Printf("[DELETE_FAILED] [%s] [%s]\n", s.id, resp.Status)
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
		jsonError(w, err.Error(), http.StatusBadRequest)
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
		jsonError(w, err.Error(), http.StatusBadRequest)
		queue.Drop()
		return
	}
	if browser.Caps.ScreenResolution != "" {
		exp := regexp.MustCompile(`^[0-9]+x[0-9]+x(8|16|24)$`)
		if !exp.MatchString(browser.Caps.ScreenResolution) {
			jsonError(w, fmt.Sprintf("Malformed screenResolution capability: %s. Correct format is WxHxD, e.g. 1920x1080x24.",
				browser.Caps.ScreenResolution), http.StatusBadRequest)
			queue.Drop()
			return
		}
	}
	starter, ok := manager.Find(browser.Caps.Name, &browser.Caps.Version, browser.Caps.ScreenResolution)
	if !ok {
		log.Printf("[ENVIRONMENT_NOT_AVAILABLE] [%s-%s]\n", browser.Caps.Name, browser.Caps.Version)
		jsonError(w, "Requested environment is not available", http.StatusBadRequest)
		queue.Drop()
		return
	}
	u, cancel, err := starter.StartWithCancel()
	if err != nil {
		log.Printf("[SERVICE_STARTUP_FAILED] [%s]\n", err.Error())
		jsonError(w, err.Error(), http.StatusInternalServerError)
		queue.Drop()
		return
	}
	r.URL.Host, r.URL.Path = u.Host, path.Clean(u.Path+r.URL.Path)
	var resp *http.Response
	for i := 1; ; i++ {
		req, _ := http.NewRequest(http.MethodPost, r.URL.String(), bytes.NewReader(body))
		ctx, _ := context.WithTimeout(r.Context(), 10*time.Second)
		log.Printf("[SESSION_ATTEMPTED] [%s] [%d]\n", u.String(), i)
		rsp, err := http.DefaultClient.Do(req.WithContext(ctx))
		select {
		case <-ctx.Done():
			if rsp != nil {
				rsp.Body.Close()
			}
			switch ctx.Err() {
			case context.DeadlineExceeded:
				log.Printf("[SESSION_ATTEMPT_TIMED_OUT]\n")
				continue
			case context.Canceled:
				log.Printf("[CLIENT_DISCONNECTED]\n")
				queue.Drop()
				cancel()
				return
			}
		default:
		}
		if err != nil {
			if rsp != nil {
				rsp.Body.Close()
			}
			log.Printf("[SESSION_FAILED] [%s] - [%s]\n", u.String(), err)
			jsonError(w, err.Error(), http.StatusInternalServerError)
			queue.Drop()
			cancel()
			return
		} else {
			resp = rsp
			break
		}
	}
	defer resp.Body.Close()
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
					sess.Lock.Lock()
					defer sess.Lock.Unlock()
					r.URL.Host, r.URL.Path = sess.URL.Host, path.Clean(sess.URL.Path+r.URL.Path)
					close(sess.Timeout)
					if r.Method == http.MethodDelete && len(fragments) == 3 {
						cancel = sess.Cancel
						sessions.Remove(id)
						queue.Release()
						log.Printf("[SESSION_DELETED] [%s]\n", id)
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
