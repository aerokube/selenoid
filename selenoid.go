package main

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/aerokube/selenoid/event"
	"github.com/aerokube/selenoid/service"
	"github.com/imdario/mergo"
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
			Caps       session.Caps    `json:"alwaysMatch"`
			FirstMatch []*session.Caps `json:"firstMatch"`
		} `json:"capabilities"`
	}
	err = json.Unmarshal(body, &browser)
	if err != nil {
		log.Printf("[%d] [BAD_JSON_FORMAT] [%v]", requestId, err)
		util.JsonError(w, err.Error(), http.StatusBadRequest)
		queue.Drop()
		return
	}
	if browser.W3CCaps.Caps.BrowserName() != "" && browser.Caps.BrowserName() == "" {
		browser.Caps = browser.W3CCaps.Caps
	}
	firstMatchCaps := browser.W3CCaps.FirstMatch
	if len(firstMatchCaps) == 0 {
		firstMatchCaps = append(firstMatchCaps, &session.Caps{})
	}
	var caps session.Caps
	var starter service.Starter
	var ok bool
	var sessionTimeout time.Duration
	var finalVideoName, finalLogName string
	for _, fmc := range firstMatchCaps {
		caps = browser.Caps
		mergo.Merge(&caps, *fmc)
		caps.ProcessExtensionCapabilities()
		sessionTimeout, err = getSessionTimeout(caps.SessionTimeout, maxTimeout, timeout)
		if err != nil {
			log.Printf("[%d] [BAD_SESSION_TIMEOUT] [%s]", requestId, caps.SessionTimeout)
			util.JsonError(w, err.Error(), http.StatusBadRequest)
			queue.Drop()
			return
		}
		resolution, err := getScreenResolution(caps.ScreenResolution)
		if err != nil {
			log.Printf("[%d] [BAD_SCREEN_RESOLUTION] [%s]", requestId, caps.ScreenResolution)
			util.JsonError(w, err.Error(), http.StatusBadRequest)
			queue.Drop()
			return
		}
		caps.ScreenResolution = resolution
		videoScreenSize, err := getVideoScreenSize(caps.VideoScreenSize, resolution)
		if err != nil {
			log.Printf("[%d] [BAD_VIDEO_SCREEN_SIZE] [%s]", requestId, caps.VideoScreenSize)
			util.JsonError(w, err.Error(), http.StatusBadRequest)
			queue.Drop()
			return
		}
		caps.VideoScreenSize = videoScreenSize
		finalVideoName = caps.VideoName
		if caps.Video && !disableDocker {
			caps.VideoName = getTemporaryFileName(videoOutputDir, videoFileExtension)
		}
		finalLogName = caps.LogName
		if logOutputDir != "" && (saveAllLogs || caps.Log) {
			caps.LogName = getTemporaryFileName(logOutputDir, logFileExtension)
		}
		starter, ok = manager.Find(caps, requestId)
		if ok {
			break
		}
	}
	if !ok {
		log.Printf("[%d] [ENVIRONMENT_NOT_AVAILABLE] [%s] [%s]", requestId, caps.BrowserName(), caps.Version)
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
	sess := &session.Session{
		Quota:     user,
		Caps:      caps,
		URL:       u,
		Container: startedService.Container,
		HostPort:  startedService.HostPort,
		Timeout:   sessionTimeout,
		TimeoutCh: onTimeout(sessionTimeout, func() {
			request{r}.session(s.ID).Delete(requestId)
		}),
		Started: time.Now()}
	cancelAndRenameFiles := func() {
		cancel()
		sessionId := preprocessSessionId(s.ID)
		e := event.Event{
			RequestId: requestId,
			SessionId: sessionId,
			Session:   sess,
		}
		if caps.Video && !disableDocker {
			oldVideoName := filepath.Join(videoOutputDir, caps.VideoName)
			if finalVideoName == "" {
				finalVideoName = sessionId + videoFileExtension
				e.Session.Caps.VideoName = finalVideoName
			}
			newVideoName := filepath.Join(videoOutputDir, finalVideoName)
			err := os.Rename(oldVideoName, newVideoName)
			if err != nil {
				log.Printf("[%d] [VIDEO_ERROR] [%s]", requestId, fmt.Sprintf("Failed to rename %s to %s: %v", oldVideoName, newVideoName, err))
			} else {
				createdFile := event.CreatedFile{
					Event: e,
					Name:  newVideoName,
					Type:  "video",
				}
				event.FileCreated(createdFile)
			}
		}
		if logOutputDir != "" && (saveAllLogs || caps.Log) {
			//The following logic will fail if -capture-driver-logs is enabled and a session is requested in driver mode.
			//Specifying both -log-output-dir and -capture-driver-logs in that case is considered a misconfiguration.
			oldLogName := filepath.Join(logOutputDir, caps.LogName)
			if finalLogName == "" {
				finalLogName = sessionId + logFileExtension
				e.Session.Caps.LogName = finalLogName
			}
			newLogName := filepath.Join(logOutputDir, finalLogName)
			err := os.Rename(oldLogName, newLogName)
			if err != nil {
				log.Printf("[%d] [LOG_ERROR] [%s]", requestId, fmt.Sprintf("Failed to rename %s to %s: %v", oldLogName, newLogName, err))
			} else {
				createdFile := event.CreatedFile{
					Event: e,
					Name:  newLogName,
					Type:  "log",
				}
				event.FileCreated(createdFile)
			}
		}
		event.SessionStopped(event.StoppedSession{e})
	}
	sess.Cancel = cancelAndRenameFiles
	sessions.Put(s.ID, sess)
	queue.Create()
	log.Printf("[%d] [SESSION_CREATED] [%s] [%d] [%.2fs]", requestId, s.ID, i, util.SecondsSince(sessionStartTime))
}

func preprocessSessionId(sid string) string {
	if ggrHost != nil {
		return ggrHost.Sum() + sid
	}
	return sid
}

const (
	videoFileExtension = ".mp4"
	logFileExtension   = ".log"
)

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

func getSessionTimeout(sessionTimeout string, maxTimeout time.Duration, defaultTimeout time.Duration) (time.Duration, error) {
	if sessionTimeout != "" {
		st, err := time.ParseDuration(sessionTimeout)
		if err != nil {
			return 0, fmt.Errorf("Invalid sessionTimeout capability: %v", err)
		}
		if st <= maxTimeout {
			return st, nil
		}
		return maxTimeout, nil
	}
	return defaultTimeout, nil
}

func getTemporaryFileName(dir string, extension string) string {
	filename := ""
	for {
		filename = generateRandomFileName(extension)
		_, err := os.Stat(filepath.Join(dir, filename))
		if err != nil {
			break
		}
	}
	return filename
}

func generateRandomFileName(extension string) string {
	randBytes := make([]byte, 16)
	rand.Read(randBytes)
	return "selenoid" + hex.EncodeToString(randBytes) + extension
}

func proxy(w http.ResponseWriter, r *http.Request) {
	done := make(chan func())
	go func() {
		(<-done)()
	}()
	cancel := func() {}
	defer func() {
		done <- cancel
	}()
	requestId := serial()
	(&httputil.ReverseProxy{
		Director: func(r *http.Request) {
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
			r.URL.Path = paths.Error
		},
		ErrorHandler: defaultErrorHandler(requestId),
	}).ServeHTTP(w, r)
}

func defaultErrorHandler(requestId uint64) func(http.ResponseWriter, *http.Request, error) {
	return func(w http.ResponseWriter, r *http.Request, err error) {
		user, remote := util.RequestInfo(r)
		log.Printf("[%d] [CLIENT_DISCONNECTED] [%s] [%s] [Error: %v]", requestId, user, remote, err)
		w.WriteHeader(http.StatusBadGateway)
	}
}

func reverseProxy(hostFn func(sess *session.Session) string, status string) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		requestId := serial()
		sid, remainingPath := splitRequestPath(r.URL.Path)
		sess, ok := sessions.Get(sid)
		if ok {
			(&httputil.ReverseProxy{
				Director: func(r *http.Request) {
					r.URL.Scheme = "http"
					r.URL.Host = hostFn(sess)
					r.URL.Path = remainingPath
					log.Printf("[%d] [%s] [%s] [%s]", requestId, status, sid, remainingPath)
				},
				ErrorHandler: defaultErrorHandler(requestId),
			}).ServeHTTP(w, r)
		} else {
			util.JsonError(w, fmt.Sprintf("Unknown session %s", sid), http.StatusNotFound)
			log.Printf("[%d] [SESSION_NOT_FOUND] [%s]", requestId, sid)
		}
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
		vncHostPort := sess.HostPort.VNC
		if vncHostPort != "" {
			log.Printf("[%d] [VNC_ENABLED] [%s]", requestId, sid)
			var d net.Dialer
			conn, err := d.DialContext(wsconn.Request().Context(), "tcp", vncHostPort)
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

const (
	jsonParam = "json"
)

func logs(w http.ResponseWriter, r *http.Request) {
	requestId := serial()
	fileNameOrSessionID := strings.TrimPrefix(r.URL.Path, paths.Logs)
	if logOutputDir != "" && (fileNameOrSessionID == "" || strings.HasSuffix(fileNameOrSessionID, logFileExtension)) {
		if r.Method == http.MethodDelete {
			deleteFileIfExists(requestId, w, r, logOutputDir, paths.Logs, "DELETED_LOG_FILE")
			return
		}
		user, remote := util.RequestInfo(r)
		if _, ok := r.URL.Query()[jsonParam]; ok {
			listFilesAsJson(requestId, w, logOutputDir, "LOG_ERROR")
			return
		}
		log.Printf("[%d] [LOG_LISTING] [%s] [%s]", requestId, user, remote)
		fileServer := http.StripPrefix(paths.Logs, http.FileServer(http.Dir(logOutputDir)))
		fileServer.ServeHTTP(w, r)
		return
	}
	websocket.Handler(streamLogs).ServeHTTP(w, r)
}

func listFilesAsJson(requestId uint64, w http.ResponseWriter, dir string, errStatus string) {
	files, err := ioutil.ReadDir(dir)
	if err != nil {
		log.Printf("[%d] [%s] [%s]", requestId, errStatus, fmt.Sprintf("Failed to list directory %s: %v", logOutputDir, err))
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	var ret []string
	for _, f := range files {
		ret = append(ret, f.Name())
	}
	w.Header().Add("Content-Type", "application/json")
	json.NewEncoder(w).Encode(ret)
}

func streamLogs(wsconn *websocket.Conn) {
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

func status(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	ready := limit > sessions.Len()
	json.NewEncoder(w).Encode(
		map[string]interface{}{
			"value": map[string]interface{}{
				"message": fmt.Sprintf("Selenoid %s built at %s", gitRevision, buildStamp),
				"ready":   ready,
			},
		})
}

func welcome(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(fmt.Sprintf("You are using Selenoid %s!", gitRevision)))
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
