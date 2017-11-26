package main

import (
	"bytes"
	"context"
	"fmt"
	"github.com/aerokube/selenoid/config"
	"io/ioutil"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"encoding/json"
	. "github.com/aandryashin/matchers"
	. "github.com/aandryashin/matchers/httpresp"
)

var (
	srv *httptest.Server
)

func init() {
	enableFileUpload = true
	srv = httptest.NewServer(handler())
}

func TestNewSessionWithGet(t *testing.T) {
	manager = &HTTPTest{Handler: Selenium()}

	rsp, err := http.Get(With(srv.URL).Path("/wd/hub/session"))
	AssertThat(t, err, Is{nil})
	AssertThat(t, rsp, Code{http.StatusMethodNotAllowed})

	AssertThat(t, queue.Used(), EqualTo{0})
}

func TestBadJsonFormat(t *testing.T) {
	rsp, err := http.Post(With(srv.URL).Path("/wd/hub/session"), "", nil)
	AssertThat(t, err, Is{nil})
	AssertThat(t, rsp, Code{http.StatusBadRequest})

	AssertThat(t, queue.Used(), EqualTo{0})
}

func TestServiceStartupFailure(t *testing.T) {
	manager = &StartupError{}

	rsp, err := http.Post(With(srv.URL).Path("/wd/hub/session"), "", bytes.NewReader([]byte("{}")))
	AssertThat(t, err, Is{nil})
	AssertThat(t, rsp, Code{http.StatusInternalServerError})

	AssertThat(t, queue.Used(), EqualTo{0})
}

func TestBrowserNotFound(t *testing.T) {
	manager = &BrowserNotFound{}

	rsp, err := http.Post(With(srv.URL).Path("/wd/hub/session"), "", bytes.NewReader([]byte("{}")))
	AssertThat(t, err, Is{nil})
	AssertThat(t, rsp, Code{http.StatusBadRequest})

	AssertThat(t, queue.Used(), EqualTo{0})
}

func TestGetDefaultScreenResolution(t *testing.T) {
	res, err := getScreenResolution("")
	AssertThat(t, err, Is{nil})
	AssertThat(t, res, EqualTo{"1920x1080x24"})
}

func TestGetFullScreenResolution(t *testing.T) {
	res, err := getScreenResolution("1024x768x24")
	AssertThat(t, err, Is{nil})
	AssertThat(t, res, EqualTo{"1024x768x24"})
}

func TestGetShortScreenResolution(t *testing.T) {
	res, err := getScreenResolution("1024x768")
	AssertThat(t, err, Is{nil})
	AssertThat(t, res, EqualTo{"1024x768x24"})
}

func TestMalformedScreenResolutionCapability(t *testing.T) {
	manager = &BrowserNotFound{}

	rsp, err := http.Post(With(srv.URL).Path("/wd/hub/session"), "", bytes.NewReader([]byte(`{"desiredCapabilities":{"screenResolution":"bad-resolution"}}`)))
	AssertThat(t, err, Is{nil})
	AssertThat(t, rsp, Code{http.StatusBadRequest})

	AssertThat(t, queue.Used(), EqualTo{0})
}

func TestGetVideoScreenSizeFromCapability(t *testing.T) {
	res, err := getVideoScreenSize("1024x768", "anything")
	AssertThat(t, err, Is{nil})
	AssertThat(t, res, EqualTo{"1024x768"})
}

func TestDetermineVideoScreenSizeFromScreenResolution(t *testing.T) {
	res, err := getVideoScreenSize("", "1024x768x24")
	AssertThat(t, err, Is{nil})
	AssertThat(t, res, EqualTo{"1024x768"})
}

func TestMalformedVideoScreenSizeCapability(t *testing.T) {
	manager = &BrowserNotFound{}

	rsp, err := http.Post(With(srv.URL).Path("/wd/hub/session"), "", bytes.NewReader([]byte(`{"desiredCapabilities":{"videoScreenSize":"bad-size"}}`)))
	AssertThat(t, err, Is{nil})
	AssertThat(t, rsp, Code{http.StatusBadRequest})

	AssertThat(t, queue.Used(), EqualTo{0})
}

func TestNewSessionNotFound(t *testing.T) {
	manager = &HTTPTest{Handler: Selenium()}

	rsp, err := http.Get(With(srv.URL).Path("/wd/hub/session/123"))
	AssertThat(t, err, Is{nil})
	AssertThat(t, rsp, Code{http.StatusNotFound})

	AssertThat(t, queue.Used(), EqualTo{0})
}

func TestNewSessionHostDown(t *testing.T) {
	canceled := false
	ch := make(chan bool)
	manager = &HTTPTest{
		Handler: Selenium(),
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

	AssertThat(t, queue.Used(), EqualTo{0})
}

func TestNewSessionBadHostResponse(t *testing.T) {
	canceled := false
	ch := make(chan bool)
	manager = &HTTPTest{
		Handler: HTTPResponse("Bad Request", http.StatusBadRequest),
		Cancel:  ch,
	}

	rsp, err := http.Post(With(srv.URL).Path("/wd/hub/session"), "", bytes.NewReader([]byte("{}")))
	AssertThat(t, err, Is{nil})
	AssertThat(t, rsp, Code{http.StatusBadRequest})

	canceled = <-ch
	AssertThat(t, canceled, Is{true})

	AssertThat(t, queue.Used(), EqualTo{0})
}

func TestSessionCreated(t *testing.T) {
	manager = &HTTPTest{Handler: Selenium()}

	resp, err := http.Post(With(srv.URL).Path("/wd/hub/session"), "", bytes.NewReader([]byte(`{"desiredCapabilities": {"enableVideo": true, "enableVNC": true}}`)))
	AssertThat(t, err, Is{nil})
	var sess map[string]string
	AssertThat(t, resp, AllOf{Code{http.StatusOK}, IsJson{&sess}})

	resp, err = http.Get(With(srv.URL).Path("/status"))
	AssertThat(t, err, Is{nil})
	var state config.State
	AssertThat(t, resp, AllOf{Code{http.StatusOK}, IsJson{&state}})
	AssertThat(t, state.Used, EqualTo{1})
	AssertThat(t, queue.Used(), EqualTo{1})
	sessions.Remove(sess["sessionId"])
	queue.Release()
}

func TestSessionCreatedWdHub(t *testing.T) {
	root := http.NewServeMux()
	root.Handle("/wd/hub/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.URL.Path = strings.TrimPrefix(r.URL.Path, "/wd/hub")
		Selenium().ServeHTTP(w, r)
	}))
	manager = &HTTPTest{Handler: root}

	resp, err := http.Post(With(srv.URL).Path("/wd/hub/session"), "", bytes.NewReader([]byte("{}")))
	AssertThat(t, err, Is{nil})
	var sess map[string]string
	AssertThat(t, resp, AllOf{Code{http.StatusOK}, IsJson{&sess}})

	resp, err = http.Get(With(srv.URL).Path("/status"))
	AssertThat(t, err, Is{nil})
	var state config.State
	AssertThat(t, resp, AllOf{Code{http.StatusOK}, IsJson{&state}})
	AssertThat(t, state.Used, EqualTo{1})
	AssertThat(t, queue.Used(), EqualTo{1})
	sessions.Remove(sess["sessionId"])
	queue.Release()
}

func TestSessionFaitedAfterTimeout(t *testing.T) {
	newSessionAttemptTimeout = 10 * time.Millisecond
	manager = &HTTPTest{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-time.After(100 * time.Millisecond)
	})}

	resp, err := http.Post(With(srv.URL).Path("/wd/hub/session"), "", bytes.NewReader([]byte("{}")))
	AssertThat(t, err, Is{nil})
	AssertThat(t, resp, AllOf{Code{http.StatusInternalServerError}})

	resp, err = http.Get(With(srv.URL).Path("/status"))
	AssertThat(t, err, Is{nil})
	var state config.State
	AssertThat(t, resp, AllOf{Code{http.StatusOK}, IsJson{&state}})
	AssertThat(t, state.Used, EqualTo{0})
	AssertThat(t, queue.Used(), EqualTo{0})
}

func TestClientDisconnected(t *testing.T) {
	manager = &HTTPTest{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-time.After(1000 * time.Millisecond)
	})}

	req, _ := http.NewRequest(http.MethodPost, With(srv.URL).Path("/wd/hub/session"), bytes.NewReader([]byte("{}")))
	ctx, cancel := context.WithCancel(req.Context())
	go http.DefaultClient.Do(req.WithContext(ctx))
	<-time.After(10 * time.Millisecond)
	cancel()

	resp, err := http.Get(With(srv.URL).Path("/status"))
	AssertThat(t, err, Is{nil})
	var state config.State
	AssertThat(t, resp, AllOf{Code{http.StatusOK}, IsJson{&state}})
	AssertThat(t, state.Used, EqualTo{0})
	AssertThat(t, queue.Used(), EqualTo{0})
}

func TestSessionFaitedAfterTwoTimeout(t *testing.T) {
	retryCount = 2
	newSessionAttemptTimeout = 10 * time.Millisecond
	manager = &HTTPTest{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-time.After(100 * time.Millisecond)
	})}

	resp, err := http.Post(With(srv.URL).Path("/wd/hub/session"), "", bytes.NewReader([]byte("{}")))
	AssertThat(t, err, Is{nil})
	AssertThat(t, resp, AllOf{Code{http.StatusInternalServerError}})

	resp, err = http.Get(With(srv.URL).Path("/status"))
	AssertThat(t, err, Is{nil})
	var state config.State
	AssertThat(t, resp, AllOf{Code{http.StatusOK}, IsJson{&state}})
	AssertThat(t, state.Used, EqualTo{0})
	AssertThat(t, queue.Used(), EqualTo{0})
}

func TestSessionCreatedRedirect(t *testing.T) {
	httpClient := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	root := http.NewServeMux()
	root.Handle("/wd/hub/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, With(srv.URL).Path("/wd/hub/session/123"), http.StatusFound)
	}))
	manager = &HTTPTest{Handler: root}

	resp, err := httpClient.Post(With(srv.URL).Path("/wd/hub/session"), "", bytes.NewReader([]byte("{}")))
	AssertThat(t, err, Is{nil})
	AssertThat(t, resp.StatusCode, Is{http.StatusFound})
	location := resp.Header.Get("Location")
	AssertThat(t, resp.StatusCode, Is{Not{""}})
	fragments := strings.Split(location, "/")
	sid := fragments[len(fragments)-1]

	resp, err = http.Get(With(srv.URL).Path("/status"))
	AssertThat(t, err, Is{nil})
	var state config.State
	AssertThat(t, resp, AllOf{Code{http.StatusOK}, IsJson{&state}})
	AssertThat(t, state.Used, EqualTo{1})
	AssertThat(t, queue.Used(), EqualTo{1})
	sessions.Remove(sid)
	queue.Release()
}

func TestProxySession(t *testing.T) {
	manager = &HTTPTest{Handler: Selenium()}

	resp, err := http.Post(With(srv.URL).Path("/wd/hub/session"), "", bytes.NewReader([]byte("{}")))
	AssertThat(t, err, Is{nil})
	var sess map[string]string
	AssertThat(t, resp, AllOf{Code{http.StatusOK}, IsJson{&sess}})

	resp, err = http.Get(With(srv.URL).Path(fmt.Sprintf("/wd/hub/session/%s/url", sess["sessionId"])))
	AssertThat(t, err, Is{nil})
	AssertThat(t, resp, Code{http.StatusOK})

	AssertThat(t, queue.Used(), EqualTo{1})
	sessions.Remove(sess["sessionId"])
	queue.Release()
}

func TestSessionDeleted(t *testing.T) {
	canceled := false
	ch := make(chan bool)
	manager = &HTTPTest{
		Handler: Selenium(),
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

	AssertThat(t, queue.Used(), EqualTo{0})
}

func TestSessionOnClose(t *testing.T) {
	manager = &HTTPTest{Handler: Selenium()}

	resp, err := http.Post(With(srv.URL).Path("/wd/hub/session"), "", bytes.NewReader([]byte("{}")))
	AssertThat(t, err, Is{nil})
	var sess map[string]string
	AssertThat(t, resp, AllOf{Code{http.StatusOK}, IsJson{&sess}})

	req, _ := http.NewRequest(http.MethodDelete,
		With(srv.URL).Path(fmt.Sprintf("/wd/hub/session/%s/window", sess["sessionId"])), nil)
	http.DefaultClient.Do(req)

	AssertThat(t, queue.Used(), EqualTo{1})
	sessions.Remove(sess["sessionId"])
	queue.Release()
}

func TestProxySessionCanceled(t *testing.T) {
	canceled := false
	ch := make(chan bool)
	manager = &HTTPTest{
		Handler: Selenium(),
		Cancel:  ch,
	}

	timeout = 100 * time.Millisecond
	resp, err := http.Post(With(srv.URL).Path("/wd/hub/session"), "", bytes.NewReader([]byte("{}")))
	AssertThat(t, err, Is{nil})
	var sess map[string]string
	AssertThat(t, resp, AllOf{Code{http.StatusOK}, IsJson{&sess}})

	_, ok := sessions.Get(sess["sessionId"])
	AssertThat(t, ok, Is{true})

	req, _ := http.NewRequest(http.MethodGet, With(srv.URL).Path(fmt.Sprintf("/wd/hub/session/%s/url?timeout=1s", sess["sessionId"])), nil)
	ctx, cancel := context.WithCancel(context.Background())
	req = req.WithContext(ctx)
	go func() {
		http.DefaultClient.Do(req)
	}()
	<-time.After(50 * time.Millisecond)
	cancel()
	<-time.After(100 * time.Millisecond)
	_, ok = sessions.Get(sess["sessionId"])
	AssertThat(t, ok, Is{false})

	canceled = <-ch
	AssertThat(t, canceled, Is{true})

	AssertThat(t, queue.Used(), EqualTo{0})
}

func TestNewSessionTimeout(t *testing.T) {
	canceled := false
	ch := make(chan bool)
	manager = &HTTPTest{
		Handler: Selenium(),
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

	AssertThat(t, queue.Used(), EqualTo{0})
}

func TestProxySessionTimeout(t *testing.T) {
	canceled := false
	ch := make(chan bool)
	manager = &HTTPTest{
		Handler: Selenium(),
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
	AssertThat(t, ok, Is{true})

	<-time.After(50 * time.Millisecond)
	_, ok = sessions.Get(sess["sessionId"])
	AssertThat(t, ok, Is{false})

	canceled = <-ch
	AssertThat(t, canceled, Is{true})

	AssertThat(t, queue.Used(), EqualTo{0})
}

func TestFileUpload(t *testing.T) {
	manager = &HTTPTest{Handler: Selenium()}

	resp, err := http.Post(With(srv.URL).Path("/wd/hub/session"), "", bytes.NewReader([]byte("{}")))
	AssertThat(t, err, Is{nil})

	var sess map[string]string
	AssertThat(t, resp, AllOf{Code{http.StatusOK}, IsJson{&sess}})

	fileContents := []byte(`{"file":"UEsDBBQACAgIAJiC4koAAAAAAAAAAAAAAAAJAAAAaGVsbG8udHh080jNyclXCM8vyklRBABQSwcIoxwpHA4AAAAMAAAAUEsBAhQAFAAICAgAmILiSqMcKRwOAAAADAAAAAkAAAAAAAAAAAAAAAAAAAAAAGhlbGxvLnR4dFBLBQYAAAAAAQABADcAAABFAAAAAAA="}`)

	//Doing two times to test sequential upload
	resp, err = http.Post(With(srv.URL).Path(fmt.Sprintf("/wd/hub/session/%s/file", sess["sessionId"])), "", bytes.NewReader(fileContents))
	AssertThat(t, err, Is{nil})
	AssertThat(t, resp, Code{http.StatusOK})

	resp, err = http.Post(With(srv.URL).Path(fmt.Sprintf("/wd/hub/session/%s/file", sess["sessionId"])), "", bytes.NewReader(fileContents))
	AssertThat(t, err, Is{nil})
	AssertThat(t, resp, Code{http.StatusOK})

	var jsonResponse map[string]string

	AssertThat(t, resp, AllOf{Code{http.StatusOK}, IsJson{&jsonResponse}})

	f, err := os.Open(jsonResponse["value"])
	AssertThat(t, err, Is{nil})

	content, err := ioutil.ReadAll(f)
	AssertThat(t, err, Is{nil})

	AssertThat(t, string(content), EqualTo{"Hello World!"})

	sessions.Remove(sess["sessionId"])
	queue.Release()
}

func TestFileUploadBadJson(t *testing.T) {
	manager = &HTTPTest{Handler: Selenium()}

	resp, err := http.Post(With(srv.URL).Path("/wd/hub/session"), "", bytes.NewReader([]byte("{}")))
	AssertThat(t, err, Is{nil})

	var sess map[string]string
	AssertThat(t, resp, AllOf{Code{http.StatusOK}, IsJson{&sess}})

	resp, err = http.Post(With(srv.URL).Path(fmt.Sprintf("/wd/hub/session/%s/file", sess["sessionId"])), "", bytes.NewReader([]byte(`malformed json`)))
	AssertThat(t, err, Is{nil})
	AssertThat(t, resp, Code{http.StatusBadRequest})

	sessions.Remove(sess["sessionId"])
	queue.Release()
}

func TestFileUploadNoFile(t *testing.T) {
	manager = &HTTPTest{Handler: Selenium()}

	resp, err := http.Post(With(srv.URL).Path("/wd/hub/session"), "", bytes.NewReader([]byte("{}")))
	AssertThat(t, err, Is{nil})

	var sess map[string]string
	AssertThat(t, resp, AllOf{Code{http.StatusOK}, IsJson{&sess}})

	resp, err = http.Post(With(srv.URL).Path(fmt.Sprintf("/wd/hub/session/%s/file", sess["sessionId"])), "", bytes.NewReader([]byte(`{}`)))
	AssertThat(t, err, Is{nil})
	AssertThat(t, resp, Code{http.StatusBadRequest})

	sessions.Remove(sess["sessionId"])
	queue.Release()
}

func TestFileUploadTwoFiles(t *testing.T) {
	manager = &HTTPTest{Handler: Selenium()}

	resp, err := http.Post(With(srv.URL).Path("/wd/hub/session"), "", bytes.NewReader([]byte("{}")))
	AssertThat(t, err, Is{nil})

	var sess map[string]string
	AssertThat(t, resp, AllOf{Code{http.StatusOK}, IsJson{&sess}})

	resp, err = http.Post(With(srv.URL).Path(fmt.Sprintf("/wd/hub/session/%s/file", sess["sessionId"])), "", bytes.NewReader([]byte(`{"file":"UEsDBAoAAAAAAKGJ4koAAAAAAAAAAAAAAAAHABwAb25lLnR4dFVUCQADbv9YWZT/WFl1eAsAAQT1AQAABBQAAABQSwMECgAAAAAApIniSgAAAAAAAAAAAAAAAAcAHAB0d28udHh0VVQJAANz/1hZc/9YWXV4CwABBPUBAAAEFAAAAFBLAQIeAwoAAAAAAKGJ4koAAAAAAAAAAAAAAAAHABgAAAAAAAAAAACkgQAAAABvbmUudHh0VVQFAANu/1hZdXgLAAEE9QEAAAQUAAAAUEsBAh4DCgAAAAAApIniSgAAAAAAAAAAAAAAAAcAGAAAAAAAAAAAAKSBQQAAAHR3by50eHRVVAUAA3P/WFl1eAsAAQT1AQAABBQAAABQSwUGAAAAAAIAAgCaAAAAggAAAAAA"}`)))
	AssertThat(t, err, Is{nil})
	AssertThat(t, resp, Code{http.StatusBadRequest})

	sessions.Remove(sess["sessionId"])
	queue.Release()
}

func TestPing(t *testing.T) {
	rsp, err := http.Get(With(srv.URL).Path("/ping"))

	AssertThat(t, err, Is{nil})
	AssertThat(t, rsp, Code{http.StatusOK})
	AssertThat(t, rsp.Body, Is{Not{nil}})

	var data map[string]interface{}
	bt, readErr := ioutil.ReadAll(rsp.Body)
	AssertThat(t, readErr, Is{nil})
	jsonErr := json.Unmarshal(bt, &data)
	AssertThat(t, jsonErr, Is{nil})
	_, hasUptime := data["uptime"]
	AssertThat(t, hasUptime, Is{true})
	_, hasLastReloadTime := data["lastReloadTime"]
	AssertThat(t, hasLastReloadTime, Is{true})
	_, hasNumRequests := data["numRequests"]
	AssertThat(t, hasNumRequests, Is{true})
}
