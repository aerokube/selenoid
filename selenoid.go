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

	"golang.org/x/net/websocket"

	"github.com/aerokube/selenoid/session"
	"sync"
)

var (
	num     uint64
	numLock sync.Mutex
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

func serial() uint64 {
	numLock.Lock()
	defer numLock.Unlock()
	id := num
	num++
	return id
}

func create(w http.ResponseWriter, r *http.Request) {
	id := serial()
	quota, _, ok := r.BasicAuth()
	if !ok {
		quota = "unknown"
	}
	body, err := ioutil.ReadAll(r.Body)
	r.Body.Close()
	if err != nil {
		log.Printf("[%d] [ERROR_READING_REQUEST] [%s] [%v]\n", id, quota, err)
		jsonError(w, err.Error(), http.StatusBadRequest)
		queue.Drop()
		return
	}
	var browser struct {
		Caps struct {
			Name             string `json:"browserName"`
			Version          string `json:"version"`
			ScreenResolution string `json:"screenResolution"`
			VNC              bool   `json:"enableVNC"`
		} `json:"desiredCapabilities"`
	}
	err = json.Unmarshal(body, &browser)
	if err != nil {
		log.Printf("[%d] [BAD_JSON_FORMAT] [%s] [%v]\n", id, quota, err)
		jsonError(w, err.Error(), http.StatusBadRequest)
		queue.Drop()
		return
	}
	if browser.Caps.ScreenResolution != "" {
		exp := regexp.MustCompile(`^[0-9]+x[0-9]+x(8|16|24)$`)
		if !exp.MatchString(browser.Caps.ScreenResolution) {
			log.Printf("[%d] [BAD_SCREEN_RESOLUTION] [%s] [%s]\n", id, quota, browser.Caps.ScreenResolution)
			jsonError(w, fmt.Sprintf("Malformed screenResolution capability: %s. Correct format is WxHxD, e.g. 1920x1080x24.",
				browser.Caps.ScreenResolution), http.StatusBadRequest)
			queue.Drop()
			return
		}
	} else {
		browser.Caps.ScreenResolution = "1920x1080x24"
	}
	starter, ok := manager.Find(browser.Caps.Name, &browser.Caps.Version, browser.Caps.ScreenResolution, browser.Caps.VNC, id)
	if !ok {
		log.Printf("[%d] [ENVIRONMENT_NOT_AVAILABLE] [%s] [%s-%s]\n", id, quota, browser.Caps.Name, browser.Caps.Version)
		jsonError(w, "Requested environment is not available", http.StatusBadRequest)
		queue.Drop()
		return
	}
	u, vnc, cancel, err := starter.StartWithCancel()
	if err != nil {
		log.Printf("[%d] [SERVICE_STARTUP_FAILED] [%s] [%v]\n", id, quota, err)
		jsonError(w, err.Error(), http.StatusInternalServerError)
		queue.Drop()
		return
	}
	r.URL.Host, r.URL.Path = u.Host, path.Clean(u.Path+r.URL.Path)
	var resp *http.Response
	i := 1
	for ; ; i++ {
		req, _ := http.NewRequest(http.MethodPost, r.URL.String(), bytes.NewReader(body))
		ctx, done := context.WithTimeout(r.Context(), 10*time.Second)
		defer done()
		log.Printf("[%d] [SESSION_ATTEMPTED] [%s] [%s] [%d]\n", id, quota, u.String(), i)
		rsp, err := http.DefaultClient.Do(req.WithContext(ctx))
		select {
		case <-ctx.Done():
			if rsp != nil {
				rsp.Body.Close()
			}
			switch ctx.Err() {
			case context.DeadlineExceeded:
				log.Printf("[%d] [SESSION_ATTEMPT_TIMED_OUT] [%s]\n", id, quota)
				continue
			case context.Canceled:
				log.Printf("[%d] [CLIENT_DISCONNECTED] [%s]\n", id, quota)
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
			log.Printf("[%d] [SESSION_FAILED] [%s] - [%s]\n", id, u.String(), err)
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
		log.Printf("[%d] [SESSION_FAILED] [%s] [Bad response from %s - %v]\n", id, quota, u.String(), resp.Status)
		queue.Drop()
		cancel()
		return
	}
	sessions.Put(s.ID, &session.Session{
		Quota:   quota,
		Browser: browser.Caps.Name,
		Version: browser.Caps.Version,
		URL:     u,
		VNC:     vnc,
		Screen:  browser.Caps.ScreenResolution,
		Cancel:  cancel,
		Timeout: onTimeout(timeout, func() {
			request{r}.session(s.ID).Delete()
		})})
	queue.Create()
	log.Printf("[%d] [SESSION_CREATED] [%s] [%s] [%s] [%d]\n", id, quota, s.ID, u, i)
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

func vnc(wsconn *websocket.Conn) {
	defer wsconn.Close()
	sid := strings.Split(wsconn.Request().URL.Path, "/")[2]
	sess, ok := sessions.Get(sid)
	if ok && sess.VNC != "" {
		log.Printf("[VNC_ENABLED] [%s]\n", sid)
		conn, err := net.Dial("tcp", sess.VNC)
		if err != nil {
			log.Printf("[VNC_ERROR] [%v]\n", err)
			return
		}
		defer conn.Close()
		wsconn.PayloadType = websocket.BinaryFrame
		go io.Copy(wsconn, conn)
		io.Copy(conn, wsconn)
	}
	log.Printf("[VNC_CLIENT_DISCONNECTED] [%s]\n", sid)
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
