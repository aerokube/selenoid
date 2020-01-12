package main

import (
	"bytes"
	"context"
	"fmt"
	"github.com/mafredri/cdp"
	"github.com/mafredri/cdp/rpcc"
	"io/ioutil"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/aerokube/selenoid/config"

	"encoding/json"
	"path/filepath"

	. "github.com/aandryashin/matchers"
	. "github.com/aandryashin/matchers/httpresp"
	ggr "github.com/aerokube/ggr/config"
)

var (
	srv *httptest.Server
)

func init() {
	enableFileUpload = true
	videoOutputDir, _ = ioutil.TempDir("", "selenoid-test")
	logOutputDir, _ = ioutil.TempDir("", "selenoid-test")
	saveAllLogs = true
	gitRevision = "test-revision"
	ggrHost = &ggr.Host{
		Name: "some-host.example.com",
		Port: 4444,
	}
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

func TestTooBigSessionTimeoutCapability(t *testing.T) {
	testBadSessionTimeoutCapability(t, "1h1m")
}

func TestInvalidSessionTimeoutCapability(t *testing.T) {
	testBadSessionTimeoutCapability(t, "wrong-value")
}

func testBadSessionTimeoutCapability(t *testing.T, timeoutValue string) {
	manager = &BrowserNotFound{}

	rsp, err := http.Post(With(srv.URL).Path("/wd/hub/session"), "", bytes.NewReader([]byte(fmt.Sprintf(`{"desiredCapabilities":{"sessionTimeout":"%s"}}`, timeoutValue))))
	AssertThat(t, err, Is{nil})
	AssertThat(t, rsp, Code{http.StatusBadRequest})

	AssertThat(t, queue.Used(), EqualTo{0})
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
	timeout = 5 * time.Second

	resp, err := http.Post(With(srv.URL).Path("/wd/hub/session"), "", bytes.NewReader([]byte(`{"desiredCapabilities": {"enableVideo": true, "enableVNC": true, "sessionTimeout": "3s"}}`)))
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

func TestSessionCreatedW3C(t *testing.T) {
	manager = &HTTPTest{Handler: Selenium()}

	resp, err := http.Post(With(srv.URL).Path("/wd/hub/session"), "", bytes.NewReader([]byte(`{"capabilities":{"alwaysMatch":{"acceptInsecureCerts":true, "browserName":"firefox", "browserVersion":"latest", "selenoid:options":{"enableVNC": true}}}}`)))
	AssertThat(t, err, Is{nil})
	var sess map[string]string
	AssertThat(t, resp, AllOf{Code{http.StatusOK}, IsJson{&sess}})

	resp, err = http.Get(With(srv.URL).Path("/status"))
	AssertThat(t, err, Is{nil})
	var state config.State
	AssertThat(t, resp, AllOf{Code{http.StatusOK}, IsJson{&state}})
	AssertThat(t, state.Used, EqualTo{1})
	AssertThat(t, queue.Used(), EqualTo{1})

	versions, firefoxPresent := state.Browsers["firefox"]
	AssertThat(t, firefoxPresent, Is{true})
	users, versionPresent := versions["latest"]
	AssertThat(t, versionPresent, Is{true})
	userInfo, userPresent := users["unknown"]
	AssertThat(t, userPresent, Is{true})
	AssertThat(t, userInfo, Not{nil})
	AssertThat(t, len(userInfo.Sessions), EqualTo{1})
	AssertThat(t, userInfo.Sessions[0].VNC, EqualTo{true})

	sessions.Remove(sess["sessionId"])
	queue.Release()
}

func TestSessionCreatedFirstMatchOnly(t *testing.T) {
	manager = &HTTPTest{Handler: Selenium()}

	resp, err := http.Post(With(srv.URL).Path("/wd/hub/session"), "", bytes.NewReader([]byte(`{"capabilities":{"firstMatch":[{"browserName":"firefox", "browserVersion":"latest", "selenoid:options":{"enableVNC": true}}]}}`)))
	AssertThat(t, err, Is{nil})
	var sess map[string]string
	AssertThat(t, resp, AllOf{Code{http.StatusOK}, IsJson{&sess}})

	resp, err = http.Get(With(srv.URL).Path("/status"))
	AssertThat(t, err, Is{nil})
	var state config.State
	AssertThat(t, resp, AllOf{Code{http.StatusOK}, IsJson{&state}})
	AssertThat(t, state.Used, EqualTo{1})
	AssertThat(t, queue.Used(), EqualTo{1})

	versions, firefoxPresent := state.Browsers["firefox"]
	AssertThat(t, firefoxPresent, Is{true})
	users, versionPresent := versions["latest"]
	AssertThat(t, versionPresent, Is{true})
	userInfo, userPresent := users["unknown"]
	AssertThat(t, userPresent, Is{true})
	AssertThat(t, userInfo, Not{nil})
	AssertThat(t, len(userInfo.Sessions), EqualTo{1})
	AssertThat(t, userInfo.Sessions[0].VNC, EqualTo{true})

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

func TestSessionFailedAfterTimeout(t *testing.T) {
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

func TestSessionFailedAfterTwoTimeout(t *testing.T) {
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

func TestProxySessionPanicOnAbortHandler(t *testing.T) {

	manager = &HTTPTest{Handler: Selenium()}

	resp, err := http.Post(With(srv.URL).Path("/wd/hub/session"), "", bytes.NewReader([]byte("{}")))
	AssertThat(t, err, Is{nil})
	var sess map[string]string
	AssertThat(t, resp, AllOf{Code{http.StatusOK}, IsJson{&sess}})

	req, _ := http.NewRequest(http.MethodGet, With(srv.URL).Path(fmt.Sprintf("/wd/hub/session/%s/url?abort-handler=true", sess["sessionId"])), nil)
	resp, err = http.DefaultClient.Do(req)
	AssertThat(t, err, Not{nil})

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

	resp, err := http.Post(With(srv.URL).Path("/wd/hub/session"), "", bytes.NewReader([]byte(`{"desiredCapabilities": {"enableVideo": true}}`)))
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
	version, hasVersion := data["version"]
	AssertThat(t, hasVersion, Is{true})
	AssertThat(t, version, EqualTo{"test-revision"})
}

func TestStatus(t *testing.T) {
	rsp, err := http.Get(With(srv.URL).Path("/wd/hub/status"))

	AssertThat(t, err, Is{nil})
	AssertThat(t, rsp, Code{http.StatusOK})
	AssertThat(t, rsp.Body, Is{Not{nil}})

	var data map[string]interface{}
	bt, readErr := ioutil.ReadAll(rsp.Body)
	AssertThat(t, readErr, Is{nil})
	jsonErr := json.Unmarshal(bt, &data)
	AssertThat(t, jsonErr, Is{nil})
	value, hasValue := data["value"]
	AssertThat(t, hasValue, Is{true})
	valueMap := value.(map[string]interface{})
	ready, hasReady := valueMap["ready"]
	AssertThat(t, hasReady, Is{true})
	AssertThat(t, ready, Is{true})
	_, hasMessage := valueMap["message"]
	AssertThat(t, hasMessage, Is{true})
}

func TestServeAndDeleteVideoFile(t *testing.T) {
	fileName := "testfile"
	filePath := filepath.Join(videoOutputDir, fileName)
	ioutil.WriteFile(filePath, []byte("test-data"), 0644)

	rsp, err := http.Get(With(srv.URL).Path("/video/testfile"))
	AssertThat(t, err, Is{nil})
	AssertThat(t, rsp, Code{http.StatusOK})

	rsp, err = http.Get(With(srv.URL).Path("/video/?json"))
	AssertThat(t, err, Is{nil})
	AssertThat(t, rsp, Code{http.StatusOK})
	var files []string
	AssertThat(t, rsp, IsJson{&files})
	AssertThat(t, files, EqualTo{[]string{"testfile"}})

	deleteReq, _ := http.NewRequest(http.MethodDelete, With(srv.URL).Path("/video/testfile"), nil)
	rsp, err = http.DefaultClient.Do(deleteReq)
	AssertThat(t, err, Is{nil})
	AssertThat(t, rsp, Code{http.StatusOK})

	//Deleting already deleted file
	rsp, err = http.DefaultClient.Do(deleteReq)
	AssertThat(t, err, Is{nil})
	AssertThat(t, rsp, Code{http.StatusNotFound})
}

func TestServeAndDeleteLogFile(t *testing.T) {
	fileName := "logfile.log"
	filePath := filepath.Join(logOutputDir, fileName)
	ioutil.WriteFile(filePath, []byte("test-data"), 0644)

	rsp, err := http.Get(With(srv.URL).Path("/logs/logfile.log"))
	AssertThat(t, err, Is{nil})
	AssertThat(t, rsp, Code{http.StatusOK})

	rsp, err = http.Get(With(srv.URL).Path("/logs/?json"))
	AssertThat(t, err, Is{nil})
	AssertThat(t, rsp, Code{http.StatusOK})
	var files []string
	AssertThat(t, rsp, IsJson{&files})
	AssertThat(t, len(files) > 0, Is{true})

	deleteReq, _ := http.NewRequest(http.MethodDelete, With(srv.URL).Path("/logs/logfile.log"), nil)
	rsp, err = http.DefaultClient.Do(deleteReq)
	AssertThat(t, err, Is{nil})
	AssertThat(t, rsp, Code{http.StatusOK})

	rsp, err = http.DefaultClient.Do(deleteReq)
	AssertThat(t, err, Is{nil})
	AssertThat(t, rsp, Code{http.StatusNotFound})
}

func TestFileDownload(t *testing.T) {
	manager = &HTTPTest{Handler: Selenium()}

	resp, err := http.Post(With(srv.URL).Path("/wd/hub/session"), "", bytes.NewReader([]byte("{}")))
	AssertThat(t, err, Is{nil})

	var sess map[string]string
	AssertThat(t, resp, AllOf{Code{http.StatusOK}, IsJson{&sess}})

	rsp, err := http.Get(With(srv.URL).Path(fmt.Sprintf("/download/%s/testfile", sess["sessionId"])))
	AssertThat(t, err, Is{nil})
	AssertThat(t, rsp, Code{http.StatusOK})
	data, err := ioutil.ReadAll(rsp.Body)
	AssertThat(t, err, Is{nil})
	AssertThat(t, string(data), EqualTo{"test-data"})

	sessions.Remove(sess["sessionId"])
	queue.Release()
}

func TestFileDownloadMissingSession(t *testing.T) {
	rsp, err := http.Get(With(srv.URL).Path("/download/missing-session/testfile"))
	AssertThat(t, err, Is{nil})
	AssertThat(t, rsp, Code{http.StatusNotFound})
}

func TestClipboard(t *testing.T) {
	manager = &HTTPTest{Handler: Selenium()}

	resp, err := http.Post(With(srv.URL).Path("/wd/hub/session"), "", bytes.NewReader([]byte("{}")))
	AssertThat(t, err, Is{nil})

	var sess map[string]string
	AssertThat(t, resp, AllOf{Code{http.StatusOK}, IsJson{&sess}})

	rsp, err := http.Get(With(srv.URL).Path(fmt.Sprintf("/clipboard/%s", sess["sessionId"])))
	AssertThat(t, err, Is{nil})
	AssertThat(t, rsp, Code{http.StatusOK})
	data, err := ioutil.ReadAll(rsp.Body)
	AssertThat(t, err, Is{nil})
	AssertThat(t, string(data), EqualTo{"test-clipboard-value"})

	rsp, err = http.Post(With(srv.URL).Path(fmt.Sprintf("/clipboard/%s", sess["sessionId"])), "text/plain", bytes.NewReader([]byte("any-data")))
	AssertThat(t, err, Is{nil})
	AssertThat(t, rsp, Code{http.StatusOK})

	sessions.Remove(sess["sessionId"])
	queue.Release()
}

func TestClipboardMissingSession(t *testing.T) {
	rsp, err := http.Get(With(srv.URL).Path("/clipboard/missing-session"))
	AssertThat(t, err, Is{nil})
	AssertThat(t, rsp, Code{http.StatusNotFound})
}

func TestDevtools(t *testing.T) {
	manager = &HTTPTest{Handler: Selenium()}

	resp, err := http.Post(With(srv.URL).Path("/wd/hub/session"), "", bytes.NewReader([]byte("{}")))
	AssertThat(t, err, Is{nil})

	var sess map[string]string
	AssertThat(t, resp, AllOf{Code{http.StatusOK}, IsJson{&sess}})

	u := fmt.Sprintf("ws://%s/devtools/%s", srv.Listener.Addr().String(), sess["sessionId"])

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	conn, err := rpcc.DialContext(ctx, u)
	AssertThat(t, err, Is{nil})
	defer conn.Close()

	c := cdp.NewClient(conn)
	err = c.Page.Enable(ctx)
	AssertThat(t, err, Is{nil})

	sessions.Remove(sess["sessionId"])
	queue.Release()
}

func TestParseGgrHost(t *testing.T) {
	h := parseGgrHost("some-host.example.com:4444")
	AssertThat(t, h.Name, EqualTo{"some-host.example.com"})
	AssertThat(t, h.Port, EqualTo{4444})
}

func TestWelcomeScreen(t *testing.T) {
	rsp, err := http.Get(With(srv.URL).Path("/"))
	AssertThat(t, err, Is{nil})
	AssertThat(t, rsp, Code{http.StatusOK})

	rsp, err = http.Get(With(srv.URL).Path("/wd/hub"))
	AssertThat(t, err, Is{nil})
	AssertThat(t, rsp, Code{http.StatusOK})
}
