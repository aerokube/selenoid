package main

import (
	"archive/zip"
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
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/aerokube/selenoid/service"
	"github.com/aerokube/selenoid/session"
	"github.com/docker/docker/api/types"
	"golang.org/x/net/websocket"
)

var (
	httpClient *http.Client = &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
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
	ctx, cancel := context.WithTimeout(context.Background(), sessionDeleteTimeout)
	defer cancel()
	resp, err := httpClient.Do(r.WithContext(ctx))
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
	sessionStartTime := time.Now()
	requestId := serial()
	quota, _, ok := r.BasicAuth()
	if !ok {
		quota = "unknown"
	}
	body, err := ioutil.ReadAll(r.Body)
	r.Body.Close()
	if err != nil {
		log.Printf("[%d] [ERROR_READING_REQUEST] [%s] [%v]\n", requestId, quota, err)
		jsonError(w, err.Error(), http.StatusBadRequest)
		queue.Drop()
		return
	}
	var browser struct {
		Caps service.Caps `json:"desiredCapabilities"`
	}
	err = json.Unmarshal(body, &browser)
	if err != nil {
		log.Printf("[%d] [BAD_JSON_FORMAT] [%s] [%v]\n", requestId, quota, err)
		jsonError(w, err.Error(), http.StatusBadRequest)
		queue.Drop()
		return
	}
	resolution, err := getScreenResolution(browser.Caps.ScreenResolution)
	if err != nil {
		log.Printf("[%d] [BAD_SCREEN_RESOLUTION] [%s] [%s]\n", requestId, quota, browser.Caps.ScreenResolution)
		jsonError(w, err.Error(), http.StatusBadRequest)
		queue.Drop()
		return
	}
	browser.Caps.ScreenResolution = resolution
	starter, ok := manager.Find(browser.Caps, requestId)
	if !ok {
		log.Printf("[%d] [ENVIRONMENT_NOT_AVAILABLE] [%s] [%s-%s]\n", requestId, quota, browser.Caps.Name, browser.Caps.Version)
		jsonError(w, "Requested environment is not available", http.StatusBadRequest)
		queue.Drop()
		return
	}
	startedService, err := starter.StartWithCancel()
	if err != nil {
		log.Printf("[%d] [SERVICE_STARTUP_FAILED] [%s] [%v]\n", requestId, quota, err)
		jsonError(w, err.Error(), http.StatusInternalServerError)
		queue.Drop()
		return
	}
	u := startedService.Url
	cancel := startedService.Cancel
	var resp *http.Response
	i := 1
	for ; ; i++ {
		r.URL.Host, r.URL.Path = u.Host, path.Join(u.Path, r.URL.Path)
		req, _ := http.NewRequest(http.MethodPost, r.URL.String(), bytes.NewReader(body))
		ctx, done := context.WithTimeout(r.Context(), newSessionAttemptTimeout)
		defer done()
		log.Printf("[%d] [SESSION_ATTEMPTED] [%s] [%s] [%d]\n", requestId, quota, u.String(), i)
		rsp, err := httpClient.Do(req.WithContext(ctx))
		select {
		case <-ctx.Done():
			if rsp != nil {
				rsp.Body.Close()
			}
			switch ctx.Err() {
			case context.DeadlineExceeded:
				log.Printf("[%d] [SESSION_ATTEMPT_TIMED_OUT] [%s]\n", requestId, quota)
				continue
			case context.Canceled:
				log.Printf("[%d] [CLIENT_DISCONNECTED] [%s]\n", requestId, quota)
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
			log.Printf("[%d] [SESSION_FAILED] [%s] - [%s]\n", requestId, u.String(), err)
			jsonError(w, err.Error(), http.StatusInternalServerError)
			queue.Drop()
			cancel()
			return
		}
		if rsp.StatusCode == http.StatusNotFound && u.Path == "" {
			u.Path = "/wd/hub"
			continue
		}
		resp = rsp
		break
	}
	defer resp.Body.Close()
	var s struct {
		Value struct {
			ID string `json:"sessionId"`
		}
		ID string `json:"sessionId"`
	}
	location := resp.Header.Get("Location")
	if location != "" {
		l, err := url.Parse(location)
		if err == nil {
			fragments := strings.Split(l.Path, "/")
			s.ID = fragments[len(fragments)-1]
			u := &url.URL{
				Scheme: "http",
				Host:   hostname,
				Path:   path.Join("/wd/hub/session", s.ID),
			}
			w.Header().Add("Location", u.String())
			w.WriteHeader(resp.StatusCode)
		}
	} else {
		tee := io.TeeReader(resp.Body, w)
		w.WriteHeader(resp.StatusCode)
		json.NewDecoder(tee).Decode(&s)
		if s.ID == "" {
			s.ID = s.Value.ID
		}
	}
	if s.ID == "" {
		log.Printf("[%d] [SESSION_FAILED] [%s] [Bad response from %s - %v]\n", requestId, quota, u.String(), resp.Status)
		queue.Drop()
		cancel()
		return
	}
	sessions.Put(s.ID, &session.Session{
		Quota:     quota,
		Browser:   browser.Caps.Name,
		Version:   browser.Caps.Version,
		URL:       u,
		Container: startedService.ID,
		VNC:       startedService.VNCHostPort,
		Screen:    browser.Caps.ScreenResolution,
		Cancel:    cancel,
		Timeout: onTimeout(timeout, func() {
			request{r}.session(s.ID).Delete()
		})})
	queue.Create()
	log.Printf("[%d] [SESSION_CREATED] [%s] [%s] [%s] [%d] [%v]\n", requestId, quota, s.ID, u, i, time.Since(sessionStartTime))
}

func getScreenResolution(input string) (string, error) {
	if input == "" {
		return "1920x1080x24", nil
	}
	fullFormat := regexp.MustCompile(`^[0-9]+x[0-9]+x(8|16|24)$`)
	shortFormat := regexp.MustCompile(`^[0-9]+x[0-9]+$`)
	if fullFormat.MatchString(input) {
		return input, nil
	}
	if shortFormat.MatchString(input) {
		return fmt.Sprintf("%sx24", input), nil
	}
	return "", fmt.Errorf(
		"Malformed screenResolution capability: %s. Correct format is WxH (1920x1080) or WxHxD (1920x1080x24).",
		input,
	)
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
					close(sess.Timeout)
					if r.Method == http.MethodDelete && len(fragments) == 3 {
						if enableFileUpload {
							os.RemoveAll(filepath.Join(os.TempDir(), id))
						}
						cancel = sess.Cancel
						sessions.Remove(id)
						queue.Release()
						log.Printf("[SESSION_DELETED] [%s]\n", id)
					} else {
						sess.Timeout = onTimeout(timeout, func() {
							request{r}.session(id).Delete()
						})
						if len(fragments) == 4 && fragments[len(fragments)-1] == "file" && enableFileUpload {
							r.Header.Set("X-Selenoid-File", filepath.Join(os.TempDir(), id))
							r.URL.Path = "/file"
							return
						}
					}
					r.URL.Host, r.URL.Path = sess.URL.Host, path.Clean(sess.URL.Path+r.URL.Path)
					return
				}
				r.URL.Path = "/error"
			},
		}).ServeHTTP(w, r)
	}(w, r)
	go (<-done)()
}

func fileUpload(w http.ResponseWriter, r *http.Request) {
	var jsonRequest struct {
		File []byte `json:"file"`
	}
	err := json.NewDecoder(r.Body).Decode(&jsonRequest)
	if err != nil {
		jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}
	z, err := zip.NewReader(bytes.NewReader(jsonRequest.File), int64(len(jsonRequest.File)))
	if err != nil {
		jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}
	if len(z.File) != 1 {
		jsonError(w, fmt.Sprintf("Expected there to be only 1 file. There were: %s", len(z.File)), http.StatusBadRequest)
		return
	}
	file := z.File[0]
	src, err := file.Open()
	if err != nil {
		jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}
	defer src.Close()
	dir := r.Header.Get("X-Selenoid-File")
	err = os.Mkdir(dir, 0755)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	fileName := filepath.Join(dir, file.Name)
	dst, err := os.OpenFile(fileName, os.O_WRONLY|os.O_CREATE, 0644)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer dst.Close()
	_, err = io.Copy(dst, src)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	reply := struct {
		V string `json:"value"`
	}{
		V: fileName,
	}
	json.NewEncoder(w).Encode(reply)
}

func vnc(wsconn *websocket.Conn) {
	defer wsconn.Close()
	sid := strings.Split(wsconn.Request().URL.Path, "/")[2]
	sess, ok := sessions.Get(sid)
	if ok {
		if sess.VNC != "" {
			log.Printf("[VNC_ENABLED] [%s]\n", sid)
			var d net.Dialer
			conn, err := d.DialContext(wsconn.Request().Context(), "tcp", sess.VNC)
			if err != nil {
				log.Printf("[VNC_ERROR] [%v]\n", err)
				return
			}
			defer conn.Close()
			wsconn.PayloadType = websocket.BinaryFrame
			go func() {
				io.Copy(wsconn, conn)
				wsconn.Close()
				log.Printf("[VNC_SESSION_CLOSED] [%s]\n", sid)
			}()
			io.Copy(conn, wsconn)
			log.Printf("[VNC_CLIENT_DISCONNECTED] [%s]\n", sid)
		} else {
			log.Printf("[VNC_NOT_ENABLED] [%s]\n", sid)
		}
	} else {
		log.Printf("[SESSION_NOT_FOUND] [%s]\n", sid)
	}
}

func logs(wsconn *websocket.Conn) {
	defer wsconn.Close()
	sid := strings.Split(wsconn.Request().URL.Path, "/")[2]
	sess, ok := sessions.Get(sid)
	if ok && sess.Container != "" {
		log.Printf("[CONTAINER_LOGS] [%s]\n", sess.Container)
		r, err := cli.ContainerLogs(wsconn.Request().Context(), sess.Container, types.ContainerLogsOptions{
			ShowStdout: true,
			ShowStderr: true,
			Follow:     true,
		})
		if err != nil {
			log.Printf("[CONTAINER_LOGS_ERROR] [%v]\n", err)
			return
		}
		defer r.Close()
		wsconn.PayloadType = websocket.BinaryFrame
		io.Copy(wsconn, r)
		log.Printf("[WEBSOCKET_CLIENT_DISCONNECTED] [%s]\n", sid)
	} else {
		log.Printf("[SESSION_NOT_FOUND] [%s]\n", sid)
	}
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
