package main

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
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

	"github.com/aerokube/selenoid/session"
	"github.com/aerokube/util"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/pkg/stdcopy"
	"golang.org/x/net/websocket"
)

const slash = "/"

var (
	httpClient = &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	num     uint64
	numLock sync.RWMutex
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

func (s *sess) Delete(requestId uint64) {
	log.Printf("[%d] [SESSION_TIMED_OUT] [%s]", requestId, s.id)
	r, err := http.NewRequest(http.MethodDelete, s.url(), nil)
	if err != nil {
		log.Printf("[%d] [DELETE_FAILED] [%s] [%v]", requestId, s.id, err)
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
		log.Printf("[%d] [DELETE_FAILED] [%s] [%v]", requestId, s.id, err)
	} else {
		log.Printf("[%d] [DELETE_FAILED] [%s] [%s]", requestId, s.id, resp.Status)
	}
}

func serial() uint64 {
	numLock.Lock()
	defer numLock.Unlock()
	id := num
	num++
	return id
}

func getSerial() uint64 {
	numLock.RLock()
	defer numLock.RUnlock()
	return num
}

func create(w http.ResponseWriter, r *http.Request) {
	sessionStartTime := time.Now()
	requestId := serial()
	user, remote := util.RequestInfo(r)
	body, err := ioutil.ReadAll(r.Body)
	r.Body.Close()
	if err != nil {
		log.Printf("[%d] [ERROR_READING_REQUEST] [%v]", requestId, err)
		util.JsonError(w, err.Error(), http.StatusBadRequest)
		queue.Drop()
		return
	}
	var browser struct {
		Caps    session.Caps `json:"desiredCapabilities"`
		W3CCaps struct {
			Caps session.Caps `json:"alwaysMatch"`
		} `json:"capabilities"`
	}
	err = json.Unmarshal(body, &browser)
	if err != nil {
		log.Printf("[%d] [BAD_JSON_FORMAT] [%v]", requestId, err)
		util.JsonError(w, err.Error(), http.StatusBadRequest)
		queue.Drop()
		return
	}
	if browser.W3CCaps.Caps.Name != "" && browser.Caps.Name == "" {
		browser.Caps = browser.W3CCaps.Caps
	}
	browser.Caps.ProcessExtensionCapabilities()
	sessionTimeout, err := getSessionTimeout(browser.Caps.SessionTimeout, maxTimeout, timeout)
	if err != nil {
		log.Printf("[%d] [BAD_SESSION_TIMEOUT] [%ds]", requestId, browser.Caps.SessionTimeout)
		util.JsonError(w, err.Error(), http.StatusBadRequest)
		queue.Drop()
		return
	}
	resolution, err := getScreenResolution(browser.Caps.ScreenResolution)
	if err != nil {
		log.Printf("[%d] [BAD_SCREEN_RESOLUTION] [%s]", requestId, browser.Caps.ScreenResolution)
		util.JsonError(w, err.Error(), http.StatusBadRequest)
		queue.Drop()
		return
	}
	browser.Caps.ScreenResolution = resolution
	videoScreenSize, err := getVideoScreenSize(browser.Caps.VideoScreenSize, resolution)
	if err != nil {
		log.Printf("[%d] [BAD_VIDEO_SCREEN_SIZE] [%s]", requestId, browser.Caps.VideoScreenSize)
		util.JsonError(w, err.Error(), http.StatusBadRequest)
		queue.Drop()
		return
	}
	browser.Caps.VideoScreenSize = videoScreenSize
	finalVideoName := browser.Caps.VideoName
	if browser.Caps.Video {
		browser.Caps.VideoName = getVideoFileName(videoOutputDir)
	}
	starter, ok := manager.Find(browser.Caps, requestId)
	if !ok {
		log.Printf("[%d] [ENVIRONMENT_NOT_AVAILABLE] [%s] [%s]", requestId, browser.Caps.Name, browser.Caps.Version)
		util.JsonError(w, "Requested environment is not available", http.StatusBadRequest)
		queue.Drop()
		return
	}
	startedService, err := starter.StartWithCancel()
	if err != nil {
		log.Printf("[%d] [SERVICE_STARTUP_FAILED] [%v]", requestId, err)
		util.JsonError(w, err.Error(), http.StatusInternalServerError)
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
		log.Printf("[%d] [SESSION_ATTEMPTED] [%s] [%d]", requestId, u.String(), i)
		rsp, err := httpClient.Do(req.WithContext(ctx))
		select {
		case <-ctx.Done():
			if rsp != nil {
				rsp.Body.Close()
			}
			switch ctx.Err() {
			case context.DeadlineExceeded:
				log.Printf("[%d] [SESSION_ATTEMPT_TIMED_OUT] [%s]", requestId, newSessionAttemptTimeout)
				if i < retryCount {
					continue
				}
				err := fmt.Errorf("New session attempts retry count exceeded")
				log.Printf("[%d] [SESSION_FAILED] [%s] [%s]", requestId, u.String(), err)
				util.JsonError(w, err.Error(), http.StatusInternalServerError)
			case context.Canceled:
				log.Printf("[%d] [CLIENT_DISCONNECTED] [%s] [%s] [%.2fs]", requestId, user, remote, util.SecondsSince(sessionStartTime))
			}
			queue.Drop()
			cancel()
			return
		default:
		}
		if err != nil {
			if rsp != nil {
				rsp.Body.Close()
			}
			log.Printf("[%d] [SESSION_FAILED] [%s] [%s]", requestId, u.String(), err)
			util.JsonError(w, err.Error(), http.StatusInternalServerError)
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
			fragments := strings.Split(l.Path, slash)
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
		log.Printf("[%d] [SESSION_FAILED] [%s] [%s]", requestId, u.String(), resp.Status)
		queue.Drop()
		cancel()
		return
	}
	cancelAndRenameVideo := func() {
		cancel()
		if browser.Caps.Video {
			oldVideoName := filepath.Join(videoOutputDir, browser.Caps.VideoName)
			if finalVideoName == "" {
				finalVideoName = s.ID + videoFileExtension
			}
			newVideoName := filepath.Join(videoOutputDir, finalVideoName)
			err := os.Rename(oldVideoName, newVideoName)
			if err != nil {
				log.Printf("[%d] [VIDEO_ERROR] [%s]", requestId, fmt.Sprintf("Failed to rename %s to %s: %v", oldVideoName, newVideoName, err))
			}
		}
	}
	sessions.Put(s.ID, &session.Session{
		Quota:      user,
		Caps:       browser.Caps,
		URL:        u,
		Container:  startedService.Container,
		Fileserver: startedService.FileserverHostPort,
		VNC:        startedService.VNCHostPort,
		Cancel:     cancelAndRenameVideo,
		Timeout:    sessionTimeout,
		TimeoutCh: onTimeout(sessionTimeout, func() {
			request{r}.session(s.ID).Delete(requestId)
		})})
	queue.Create()
	log.Printf("[%d] [SESSION_CREATED] [%s] [%d] [%.2fs]", requestId, s.ID, i, util.SecondsSince(sessionStartTime))
}

const videoFileExtension = ".mp4"

var (
	fullFormat  = regexp.MustCompile(`^([0-9]+x[0-9]+)x(8|16|24)$`)
	shortFormat = regexp.MustCompile(`^[0-9]+x[0-9]+$`)
)

func getScreenResolution(input string) (string, error) {
	if input == "" {
		return "1920x1080x24", nil
	}
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

func shortenScreenResolution(screenResolution string) string {
	return fullFormat.FindStringSubmatch(screenResolution)[1]
}

func getVideoScreenSize(videoScreenSize string, screenResolution string) (string, error) {
	if videoScreenSize != "" {
		if shortFormat.MatchString(videoScreenSize) {
			return videoScreenSize, nil
		}
		return "", fmt.Errorf(
			"Malformed videoScreenSize capability: %s. Correct format is WxH (1920x1080).",
			videoScreenSize,
		)
	}
	return shortenScreenResolution(screenResolution), nil
}

func getSessionTimeout(sessionTimeout uint32, maxTimeout time.Duration, defaultTimeout time.Duration) (time.Duration, error) {
	if sessionTimeout > 0 {
		std := time.Duration(sessionTimeout) * time.Second
		if std <= maxTimeout {
			return std, nil
		} else {
			return 0, fmt.Errorf("Invalid sessionTimeout capability: should be <= %s", maxTimeout)
		}
	}
	return defaultTimeout, nil
}

func getVideoFileName(videoOutputDir string) string {
	filename := ""
	for {
		filename = generateRandomFileName()
		_, err := os.Stat(filepath.Join(videoOutputDir, filename))
		if err != nil {
			break
		}
	}
	return filename
}

func generateRandomFileName() string {
	randBytes := make([]byte, 16)
	rand.Read(randBytes)
	return "selenoid" + hex.EncodeToString(randBytes) + videoFileExtension
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
				requestId := serial()
				fragments := strings.Split(r.URL.Path, slash)
				id := fragments[2]
				sess, ok := sessions.Get(id)
				if ok {
					sess.Lock.Lock()
					defer sess.Lock.Unlock()
					select {
					case <-sess.TimeoutCh:
					default:
						close(sess.TimeoutCh)
					}
					if r.Method == http.MethodDelete && len(fragments) == 3 {
						if enableFileUpload {
							os.RemoveAll(filepath.Join(os.TempDir(), id))
						}
						cancel = sess.Cancel
						sessions.Remove(id)
						queue.Release()
						log.Printf("[%d] [SESSION_DELETED] [%s]", requestId, id)
					} else {
						sess.TimeoutCh = onTimeout(sess.Timeout, func() {
							request{r}.session(id).Delete(requestId)
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

func fileDownload(w http.ResponseWriter, r *http.Request) {
	requestId := serial()
	sid, remainingPath := splitRequestPath(r.URL.Path)
	sess, ok := sessions.Get(sid)
	if ok {
		(&httputil.ReverseProxy{
			Director: func(r *http.Request) {
				r.URL.Scheme = "http"
				r.URL.Host = sess.Fileserver
				r.URL.Path = remainingPath
				log.Printf("[%d] [DOWNLOADING_FILE] [%s] [%s]", requestId, sid, remainingPath)
			},
		}).ServeHTTP(w, r)
	} else {
		util.JsonError(w, fmt.Sprintf("Unknown session %s", sid), http.StatusNotFound)
		log.Printf("[%d] [SESSION_NOT_FOUND] [%s]", requestId, sid)
	}
}

func splitRequestPath(p string) (string, string) {
	fragments := strings.Split(p, slash)
	return fragments[2], slash + strings.Join(fragments[3:], slash)
}

func fileUpload(w http.ResponseWriter, r *http.Request) {
	var jsonRequest struct {
		File []byte `json:"file"`
	}
	err := json.NewDecoder(r.Body).Decode(&jsonRequest)
	if err != nil {
		util.JsonError(w, err.Error(), http.StatusBadRequest)
		return
	}
	z, err := zip.NewReader(bytes.NewReader(jsonRequest.File), int64(len(jsonRequest.File)))
	if err != nil {
		util.JsonError(w, err.Error(), http.StatusBadRequest)
		return
	}
	if len(z.File) != 1 {
		util.JsonError(w, fmt.Sprintf("Expected there to be only 1 file. There were: %d", len(z.File)), http.StatusBadRequest)
		return
	}
	file := z.File[0]
	src, err := file.Open()
	if err != nil {
		util.JsonError(w, err.Error(), http.StatusBadRequest)
		return
	}
	defer src.Close()
	dir := r.Header.Get("X-Selenoid-File")
	err = os.MkdirAll(dir, 0755)
	if err != nil {
		util.JsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	fileName := filepath.Join(dir, file.Name)
	dst, err := os.OpenFile(fileName, os.O_WRONLY|os.O_CREATE, 0644)
	if err != nil {
		util.JsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer dst.Close()
	_, err = io.Copy(dst, src)
	if err != nil {
		util.JsonError(w, err.Error(), http.StatusInternalServerError)
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
	requestId := serial()
	sid, _ := splitRequestPath(wsconn.Request().URL.Path)
	sess, ok := sessions.Get(sid)
	if ok {
		if sess.VNC != "" {
			log.Printf("[%d] [VNC_ENABLED] [%s]", requestId, sid)
			var d net.Dialer
			conn, err := d.DialContext(wsconn.Request().Context(), "tcp", sess.VNC)
			if err != nil {
				log.Printf("[%d] [VNC_ERROR] [%v]", requestId, err)
				return
			}
			defer conn.Close()
			wsconn.PayloadType = websocket.BinaryFrame
			go func() {
				io.Copy(wsconn, conn)
				wsconn.Close()
				log.Printf("[%d] [VNC_SESSION_CLOSED] [%s]", requestId, sid)
			}()
			io.Copy(conn, wsconn)
			log.Printf("[%d] [VNC_CLIENT_DISCONNECTED] [%s]", requestId, sid)
		} else {
			log.Printf("[%d] [VNC_NOT_ENABLED] [%s]", requestId, sid)
		}
	} else {
		log.Printf("[%d] [SESSION_NOT_FOUND] [%s]", requestId, sid)
	}
}

func logs(wsconn *websocket.Conn) {
	defer wsconn.Close()
	requestId := serial()
	sid, _ := splitRequestPath(wsconn.Request().URL.Path)
	sess, ok := sessions.Get(sid)
	if ok && sess.Container != nil {
		log.Printf("[%d] [CONTAINER_LOGS] [%s]", requestId, sess.Container.ID)
		r, err := cli.ContainerLogs(wsconn.Request().Context(), sess.Container.ID, types.ContainerLogsOptions{
			ShowStdout: true,
			ShowStderr: true,
			Follow:     true,
		})
		if err != nil {
			log.Printf("[%d] [CONTAINER_LOGS_ERROR] [%v]", requestId, err)
			return
		}
		defer r.Close()
		wsconn.PayloadType = websocket.BinaryFrame
		stdcopy.StdCopy(wsconn, wsconn, r)
		log.Printf("[%d] [CONTAINER_LOGS_DISCONNECTED] [%s]", requestId, sid)
	} else {
		log.Printf("[%d] [SESSION_NOT_FOUND] [%s]", requestId, sid)
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
