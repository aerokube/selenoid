package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	ggr "github.com/aerokube/ggr/config"
	"github.com/aerokube/selenoid/config"
	"github.com/mafredri/cdp"
	"github.com/mafredri/cdp/rpcc"
	assert "github.com/stretchr/testify/require"
)

var _ = func() bool {
	testing.Init()
	return true
}()

var (
	srv *httptest.Server
)

func init() {
	enableFileUpload = true
	videoOutputDir, _ = os.MkdirTemp("", "selenoid-test")
	logOutputDir, _ = os.MkdirTemp("", "selenoid-test")
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
	assert.NoError(t, err)
	assert.Equal(t, rsp.StatusCode, http.StatusMethodNotAllowed)
	assert.Equal(t, queue.Used(), 0)
}

func TestBadJsonFormat(t *testing.T) {
	rsp, err := http.Post(With(srv.URL).Path("/wd/hub/session"), "", nil)
	assert.NoError(t, err)
	assert.Equal(t, rsp.StatusCode, http.StatusBadRequest)
	assert.Equal(t, queue.Used(), 0)
}

func TestServiceStartupFailure(t *testing.T) {
	manager = &StartupError{}

	rsp, err := http.Post(With(srv.URL).Path("/wd/hub/session"), "", bytes.NewReader([]byte("{}")))
	assert.NoError(t, err)
	assert.Equal(t, rsp.StatusCode, http.StatusInternalServerError)
	assert.Equal(t, queue.Used(), 0)
}

func TestBrowserNotFound(t *testing.T) {
	manager = &BrowserNotFound{}

	rsp, err := http.Post(With(srv.URL).Path("/wd/hub/session"), "", bytes.NewReader([]byte("{}")))
	assert.NoError(t, err)
	assert.Equal(t, rsp.StatusCode, http.StatusBadRequest)
	assert.Equal(t, queue.Used(), 0)
}

func TestGetDefaultScreenResolution(t *testing.T) {
	res, err := getScreenResolution("")
	assert.NoError(t, err)
	assert.Equal(t, res, "1920x1080x24")
}

func TestGetFullScreenResolution(t *testing.T) {
	res, err := getScreenResolution("1024x768x24")
	assert.NoError(t, err)
	assert.Equal(t, res, "1024x768x24")
}

func TestGetShortScreenResolution(t *testing.T) {
	res, err := getScreenResolution("1024x768")
	assert.NoError(t, err)
	assert.Equal(t, res, "1024x768x24")
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
	assert.NoError(t, err)
	assert.Equal(t, rsp.StatusCode, http.StatusBadRequest)
	assert.Equal(t, queue.Used(), 0)
}

func TestMalformedScreenResolutionCapability(t *testing.T) {
	manager = &BrowserNotFound{}

	rsp, err := http.Post(With(srv.URL).Path("/wd/hub/session"), "", bytes.NewReader([]byte(`{"desiredCapabilities":{"screenResolution":"bad-resolution"}}`)))
	assert.NoError(t, err)
	assert.Equal(t, rsp.StatusCode, http.StatusBadRequest)
	assert.Equal(t, queue.Used(), 0)
}

func TestGetVideoScreenSizeFromCapability(t *testing.T) {
	res, err := getVideoScreenSize("1024x768", "anything")
	assert.NoError(t, err)
	assert.Equal(t, res, "1024x768")
}

func TestDetermineVideoScreenSizeFromScreenResolution(t *testing.T) {
	res, err := getVideoScreenSize("", "1024x768x24")
	assert.NoError(t, err)
	assert.Equal(t, res, "1024x768")
}

func TestMalformedVideoScreenSizeCapability(t *testing.T) {
	manager = &BrowserNotFound{}

	rsp, err := http.Post(With(srv.URL).Path("/wd/hub/session"), "", bytes.NewReader([]byte(`{"desiredCapabilities":{"videoScreenSize":"bad-size"}}`)))
	assert.NoError(t, err)
	assert.Equal(t, rsp.StatusCode, http.StatusBadRequest)
	assert.Equal(t, queue.Used(), 0)
}

func TestNewSessionNotFound(t *testing.T) {
	manager = &HTTPTest{Handler: Selenium()}

	rsp, err := http.Get(With(srv.URL).Path("/wd/hub/session/123"))
	assert.NoError(t, err)
	assert.Equal(t, rsp.StatusCode, http.StatusNotFound)
	assert.Equal(t, queue.Used(), 0)
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
	assert.NoError(t, err)
	assert.Equal(t, rsp.StatusCode, http.StatusInternalServerError)

	canceled = <-ch
	assert.True(t, canceled)

	assert.Equal(t, queue.Used(), 0)
}

func TestNewSessionBadHostResponse(t *testing.T) {
	canceled := false
	ch := make(chan bool)
	manager = &HTTPTest{
		Handler: HTTPResponse("Bad Request", http.StatusBadRequest),
		Cancel:  ch,
	}

	rsp, err := http.Post(With(srv.URL).Path("/wd/hub/session"), "", bytes.NewReader([]byte("{}")))
	assert.NoError(t, err)
	assert.Equal(t, rsp.StatusCode, http.StatusBadRequest)

	canceled = <-ch
	assert.True(t, canceled)
	assert.Equal(t, queue.Used(), 0)
}

func TestSessionCreated(t *testing.T) {
	manager = &HTTPTest{Handler: Selenium()}
	timeout = 5 * time.Second

	resp, err := http.Post(With(srv.URL).Path("/wd/hub/session"), "", bytes.NewReader([]byte(`{"desiredCapabilities": {"enableVideo": true, "enableVNC": true, "sessionTimeout": "3s"}}`)))
	assert.NoError(t, err)
	assert.Equal(t, resp.StatusCode, http.StatusOK)
	var sess map[string]string
	assert.NoError(t, json.NewDecoder(resp.Body).Decode(&sess))

	resp, err = http.Get(With(srv.URL).Path("/status"))
	assert.NoError(t, err)
	assert.Equal(t, resp.StatusCode, http.StatusOK)
	var state config.State
	assert.NoError(t, json.NewDecoder(resp.Body).Decode(&state))
	assert.Equal(t, state.Used, 1)
	assert.Equal(t, queue.Used(), 1)
	sessions.Remove(sess["sessionId"])
	queue.Release()
}

func TestSessionCreatedW3C(t *testing.T) {
	manager = &HTTPTest{Handler: Selenium()}

	resp, err := http.Post(With(srv.URL).Path("/wd/hub/session"), "", bytes.NewReader([]byte(`{"capabilities":{"alwaysMatch":{"acceptInsecureCerts":true, "browserName":"firefox", "browserVersion":"latest", "selenoid:options":{"enableVNC": true}}}}`)))
	assert.NoError(t, err)
	assert.Equal(t, resp.StatusCode, http.StatusOK)
	var sess map[string]string
	assert.NoError(t, json.NewDecoder(resp.Body).Decode(&sess))

	resp, err = http.Get(With(srv.URL).Path("/status"))
	assert.NoError(t, err)
	assert.Equal(t, resp.StatusCode, http.StatusOK)
	var state config.State
	assert.NoError(t, json.NewDecoder(resp.Body).Decode(&state))
	assert.Equal(t, state.Used, 1)
	assert.Equal(t, queue.Used(), 1)

	versions, firefoxPresent := state.Browsers["firefox"]
	assert.True(t, firefoxPresent)
	users, versionPresent := versions["latest"]
	assert.True(t, versionPresent)
	userInfo, userPresent := users["unknown"]
	assert.True(t, userPresent)
	assert.NotNil(t, userInfo)
	assert.Len(t, userInfo.Sessions, 1)
	assert.True(t, userInfo.Sessions[0].VNC)

	sessions.Remove(sess["sessionId"])
	queue.Release()
}

func TestSessionCreatedFirstMatchOnly(t *testing.T) {
	manager = &HTTPTest{Handler: Selenium()}

	resp, err := http.Post(With(srv.URL).Path("/wd/hub/session"), "", bytes.NewReader([]byte(`{"capabilities":{"firstMatch":[{"browserName":"firefox", "browserVersion":"latest", "selenoid:options":{"enableVNC": true}}]}}`)))
	assert.NoError(t, err)
	assert.Equal(t, resp.StatusCode, http.StatusOK)
	var sess map[string]string
	assert.NoError(t, json.NewDecoder(resp.Body).Decode(&sess))

	resp, err = http.Get(With(srv.URL).Path("/status"))
	assert.NoError(t, err)
	assert.Equal(t, resp.StatusCode, http.StatusOK)
	var state config.State
	assert.NoError(t, json.NewDecoder(resp.Body).Decode(&state))
	assert.Equal(t, state.Used, 1)
	assert.Equal(t, queue.Used(), 1)

	versions, firefoxPresent := state.Browsers["firefox"]
	assert.True(t, firefoxPresent)
	users, versionPresent := versions["latest"]
	assert.True(t, versionPresent)
	userInfo, userPresent := users["unknown"]
	assert.True(t, userPresent)
	assert.NotNil(t, userInfo)
	assert.Len(t, userInfo.Sessions, 1)
	assert.True(t, userInfo.Sessions[0].VNC)

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
	assert.NoError(t, err)
	assert.Equal(t, resp.StatusCode, http.StatusOK)
	var sess map[string]string
	assert.NoError(t, json.NewDecoder(resp.Body).Decode(&sess))

	resp, err = http.Get(With(srv.URL).Path("/status"))
	assert.NoError(t, err)
	assert.Equal(t, resp.StatusCode, http.StatusOK)
	var state config.State
	assert.NoError(t, json.NewDecoder(resp.Body).Decode(&state))
	assert.Equal(t, state.Used, 1)
	assert.Equal(t, queue.Used(), 1)
	sessions.Remove(sess["sessionId"])
	queue.Release()
}

func TestSessionWithContentTypeCreatedWdHub(t *testing.T) {
	root := http.NewServeMux()
	root.Handle("/wd/hub/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.URL.Path = strings.TrimPrefix(r.URL.Path, "/wd/hub")
		assert.Equal(t, r.Header.Get("Content-Type"), "application/json; charset=utf-8")
		Selenium().ServeHTTP(w, r)
	}))
	manager = &HTTPTest{Handler: root}

	resp, err := http.Post(With(srv.URL).Path("/wd/hub/session"), "application/json; charset=utf-8", bytes.NewReader([]byte("{}")))
	assert.NoError(t, err)
	assert.Equal(t, resp.StatusCode, http.StatusOK)
	var sess map[string]string
	assert.NoError(t, json.NewDecoder(resp.Body).Decode(&sess))

	resp, err = http.Get(With(srv.URL).Path("/status"))
	assert.NoError(t, err)
	assert.Equal(t, resp.StatusCode, http.StatusOK)
	var state config.State
	assert.NoError(t, json.NewDecoder(resp.Body).Decode(&state))
	assert.Equal(t, state.Used, 1)
	assert.Equal(t, queue.Used(), 1)
	sessions.Remove(sess["sessionId"])
	queue.Release()
}

func TestSessionFailedAfterTimeout(t *testing.T) {
	newSessionAttemptTimeout = 10 * time.Millisecond
	manager = &HTTPTest{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-time.After(100 * time.Millisecond)
	})}

	resp, err := http.Post(With(srv.URL).Path("/wd/hub/session"), "", bytes.NewReader([]byte("{}")))
	assert.NoError(t, err)
	assert.Equal(t, resp.StatusCode, http.StatusInternalServerError)

	resp, err = http.Get(With(srv.URL).Path("/status"))
	assert.NoError(t, err)
	assert.Equal(t, resp.StatusCode, http.StatusOK)
	var state config.State
	assert.NoError(t, json.NewDecoder(resp.Body).Decode(&state))
	assert.Equal(t, state.Used, 0)
	assert.Equal(t, queue.Used(), 0)
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
	assert.NoError(t, err)
	assert.Equal(t, resp.StatusCode, http.StatusOK)
	var state config.State
	assert.NoError(t, json.NewDecoder(resp.Body).Decode(&state))
	assert.Equal(t, state.Used, 0)
	assert.Equal(t, queue.Used(), 0)
}

func TestSessionFailedAfterTwoTimeout(t *testing.T) {
	retryCount = 2
	newSessionAttemptTimeout = 10 * time.Millisecond
	manager = &HTTPTest{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-time.After(100 * time.Millisecond)
	})}

	resp, err := http.Post(With(srv.URL).Path("/wd/hub/session"), "", bytes.NewReader([]byte("{}")))
	assert.NoError(t, err)
	assert.Equal(t, resp.StatusCode, http.StatusInternalServerError)

	resp, err = http.Get(With(srv.URL).Path("/status"))
	assert.NoError(t, err)
	assert.Equal(t, resp.StatusCode, http.StatusOK)
	var state config.State
	assert.NoError(t, json.NewDecoder(resp.Body).Decode(&state))
	assert.Equal(t, state.Used, 0)
	assert.Equal(t, queue.Used(), 0)
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
	assert.NoError(t, err)
	assert.Equal(t, resp.StatusCode, http.StatusFound)
	location := resp.Header.Get("Location")
	fragments := strings.Split(location, "/")
	sid := fragments[len(fragments)-1]

	resp, err = http.Get(With(srv.URL).Path("/status"))
	assert.NoError(t, err)
	assert.Equal(t, resp.StatusCode, http.StatusOK)
	var state config.State
	assert.NoError(t, json.NewDecoder(resp.Body).Decode(&state))
	assert.Equal(t, state.Used, 1)
	assert.Equal(t, queue.Used(), 1)
	sessions.Remove(sid)
	queue.Release()
}

func TestSessionCreatedRemoveExtensionCapabilities(t *testing.T) {
	desiredCapabilitiesPresent := true
	alwaysMatchPresent := true
	firstMatchPresent := true
	chromeOptionsPresent := true

	var browser struct {
		Caps    map[string]interface{} `json:"desiredCapabilities"`
		W3CCaps struct {
			AlwaysMatch map[string]interface{}   `json:"alwaysMatch"`
			FirstMatch  []map[string]interface{} `json:"firstMatch"`
		} `json:"capabilities"`
	}

	root := http.NewServeMux()
	root.Handle("/wd/hub/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		err := json.NewDecoder(r.Body).Decode(&browser)
		assert.NoError(t, err)
		_, desiredCapabilitiesPresent = browser.Caps["selenoid:options"]
		_, alwaysMatchPresent = browser.W3CCaps.AlwaysMatch["selenoid:options"]
		_, chromeOptionsPresent = browser.W3CCaps.AlwaysMatch["goog:chromeOptions"]
		assert.Len(t, browser.W3CCaps.FirstMatch, 1)
		_, firstMatchPresent = browser.W3CCaps.FirstMatch[0]["selenoid:options"]
	}))
	manager = &HTTPTest{Handler: root}

	resp, err := httpClient.Post(With(srv.URL).Path("/wd/hub/session"), "", bytes.NewReader([]byte(`{"desiredCapabilities": {"browserName": "chrome", "selenoid:options": {"enableVNC": true}}, "capabilities":{"alwaysMatch":{"browserName": "chrome", "goog:chromeOptions": {"args": ["headless"]}, "selenoid:options":{"enableVNC": true}}, "firstMatch": [{"platform": "linux", "selenoid:options": {"enableVideo": true}}]}}`)))
	assert.NoError(t, err)
	assert.Equal(t, resp.StatusCode, http.StatusOK)
	assert.False(t, desiredCapabilitiesPresent)
	assert.False(t, alwaysMatchPresent)
	assert.True(t, chromeOptionsPresent)
	assert.False(t, firstMatchPresent)
}

func TestProxySession(t *testing.T) {
	manager = &HTTPTest{Handler: Selenium()}

	resp, err := http.Post(With(srv.URL).Path("/wd/hub/session"), "", bytes.NewReader([]byte("{}")))
	assert.NoError(t, err)
	assert.Equal(t, resp.StatusCode, http.StatusOK)
	var sess map[string]string
	assert.NoError(t, json.NewDecoder(resp.Body).Decode(&sess))

	resp, err = http.Get(With(srv.URL).Path(fmt.Sprintf("/wd/hub/session/%s/url", sess["sessionId"])))
	assert.NoError(t, err)
	assert.Equal(t, resp.StatusCode, http.StatusOK)

	assert.Equal(t, queue.Used(), 1)
	sessions.Remove(sess["sessionId"])
	queue.Release()
}

func TestProxySessionPanicOnAbortHandler(t *testing.T) {

	manager = &HTTPTest{Handler: Selenium()}

	resp, err := http.Post(With(srv.URL).Path("/wd/hub/session"), "", bytes.NewReader([]byte("{}")))
	assert.NoError(t, err)
	assert.Equal(t, resp.StatusCode, http.StatusOK)
	var sess map[string]string
	assert.NoError(t, json.NewDecoder(resp.Body).Decode(&sess))

	req, _ := http.NewRequest(http.MethodGet, With(srv.URL).Path(fmt.Sprintf("/wd/hub/session/%s/url?abort-handler=true", sess["sessionId"])), nil)
	resp, err = http.DefaultClient.Do(req)
	assert.Error(t, err)

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
	assert.NoError(t, err)
	assert.Equal(t, resp.StatusCode, http.StatusOK)
	var sess map[string]string
	assert.NoError(t, json.NewDecoder(resp.Body).Decode(&sess))

	req, _ := http.NewRequest(http.MethodDelete,
		With(srv.URL).Path(fmt.Sprintf("/wd/hub/session/%s", sess["sessionId"])), nil)
	_, err = http.DefaultClient.Do(req)
	assert.NoError(t, err)

	resp, err = http.Get(With(srv.URL).Path("/status"))
	assert.NoError(t, err)
	assert.Equal(t, resp.StatusCode, http.StatusOK)
	var state config.State
	assert.NoError(t, json.NewDecoder(resp.Body).Decode(&state))
	assert.Equal(t, state.Used, 0)

	canceled = <-ch
	assert.True(t, canceled)

	assert.Equal(t, queue.Used(), 0)
}

func TestSessionOnClose(t *testing.T) {
	manager = &HTTPTest{Handler: Selenium()}

	resp, err := http.Post(With(srv.URL).Path("/wd/hub/session"), "", bytes.NewReader([]byte("{}")))
	assert.NoError(t, err)
	assert.Equal(t, resp.StatusCode, http.StatusOK)
	var sess map[string]string
	assert.NoError(t, json.NewDecoder(resp.Body).Decode(&sess))

	req, _ := http.NewRequest(http.MethodDelete,
		With(srv.URL).Path(fmt.Sprintf("/wd/hub/session/%s/window", sess["sessionId"])), nil)
	_, _ = http.DefaultClient.Do(req)

	assert.Equal(t, queue.Used(), 1)
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
	assert.NoError(t, err)
	assert.Equal(t, resp.StatusCode, http.StatusOK)
	var sess map[string]string
	assert.NoError(t, json.NewDecoder(resp.Body).Decode(&sess))

	_, ok := sessions.Get(sess["sessionId"])
	assert.True(t, ok)

	req, _ := http.NewRequest(http.MethodGet, With(srv.URL).Path(fmt.Sprintf("/wd/hub/session/%s/url?timeout=1s", sess["sessionId"])), nil)
	ctx, cancel := context.WithCancel(context.Background())
	req = req.WithContext(ctx)
	go func() {
		_, _ = http.DefaultClient.Do(req)
	}()
	<-time.After(50 * time.Millisecond)
	cancel()
	<-time.After(100 * time.Millisecond)
	_, ok = sessions.Get(sess["sessionId"])
	assert.False(t, ok)

	canceled = <-ch
	assert.True(t, canceled)

	assert.Equal(t, queue.Used(), 0)
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
	assert.NoError(t, err)
	assert.Equal(t, resp.StatusCode, http.StatusOK)
	var sess map[string]string
	assert.NoError(t, json.NewDecoder(resp.Body).Decode(&sess))

	_, ok := sessions.Get(sess["sessionId"])
	assert.True(t, ok)

	<-time.After(50 * time.Millisecond)
	_, ok = sessions.Get(sess["sessionId"])
	assert.False(t, ok)

	canceled = <-ch
	assert.True(t, canceled)

	assert.Equal(t, queue.Used(), 0)
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
	assert.NoError(t, err)
	assert.Equal(t, resp.StatusCode, http.StatusOK)
	var sess map[string]string
	assert.NoError(t, json.NewDecoder(resp.Body).Decode(&sess))

	_, ok := sessions.Get(sess["sessionId"])
	assert.True(t, ok)

	<-time.After(20 * time.Millisecond)
	_, ok = sessions.Get(sess["sessionId"])
	assert.True(t, ok)
	_, _ = http.Get(With(srv.URL).Path(fmt.Sprintf("/wd/hub/session/%s/url", sess["sessionId"])))

	<-time.After(20 * time.Millisecond)
	_, ok = sessions.Get(sess["sessionId"])
	assert.True(t, ok)

	<-time.After(50 * time.Millisecond)
	_, ok = sessions.Get(sess["sessionId"])
	assert.False(t, ok)

	canceled = <-ch
	assert.True(t, canceled)

	assert.Equal(t, queue.Used(), 0)
}

func TestFileUpload(t *testing.T) {
	manager = &HTTPTest{Handler: Selenium()}

	resp, err := http.Post(With(srv.URL).Path("/wd/hub/session"), "", bytes.NewReader([]byte("{}")))
	assert.NoError(t, err)
	assert.Equal(t, resp.StatusCode, http.StatusOK)
	var sess map[string]string
	assert.NoError(t, json.NewDecoder(resp.Body).Decode(&sess))

	fileContents := []byte(`{"file":"UEsDBBQACAgIAJiC4koAAAAAAAAAAAAAAAAJAAAAaGVsbG8udHh080jNyclXCM8vyklRBABQSwcIoxwpHA4AAAAMAAAAUEsBAhQAFAAICAgAmILiSqMcKRwOAAAADAAAAAkAAAAAAAAAAAAAAAAAAAAAAGhlbGxvLnR4dFBLBQYAAAAAAQABADcAAABFAAAAAAA="}`)

	//Doing two times to test sequential upload
	resp, err = http.Post(With(srv.URL).Path(fmt.Sprintf("/wd/hub/session/%s/file", sess["sessionId"])), "", bytes.NewReader(fileContents))
	assert.NoError(t, err)
	assert.Equal(t, resp.StatusCode, http.StatusOK)

	resp, err = http.Post(With(srv.URL).Path(fmt.Sprintf("/wd/hub/session/%s/file", sess["sessionId"])), "", bytes.NewReader(fileContents))
	assert.NoError(t, err)
	assert.Equal(t, resp.StatusCode, http.StatusOK)

	var jsonResponse map[string]string
	assert.NoError(t, json.NewDecoder(resp.Body).Decode(&jsonResponse))

	f, err := os.Open(jsonResponse["value"])
	assert.NoError(t, err)

	content, err := io.ReadAll(f)
	assert.NoError(t, err)

	assert.Equal(t, string(content), "Hello World!")

	sessions.Remove(sess["sessionId"])
	queue.Release()
}

func TestFileUploadBadJson(t *testing.T) {
	manager = &HTTPTest{Handler: Selenium()}

	resp, err := http.Post(With(srv.URL).Path("/wd/hub/session"), "", bytes.NewReader([]byte("{}")))
	assert.NoError(t, err)
	assert.Equal(t, resp.StatusCode, http.StatusOK)
	var sess map[string]string
	assert.NoError(t, json.NewDecoder(resp.Body).Decode(&sess))

	resp, err = http.Post(With(srv.URL).Path(fmt.Sprintf("/wd/hub/session/%s/file", sess["sessionId"])), "", bytes.NewReader([]byte(`malformed json`)))
	assert.NoError(t, err)
	assert.Equal(t, resp.StatusCode, http.StatusBadRequest)

	sessions.Remove(sess["sessionId"])
	queue.Release()
}

func TestFileUploadNoFile(t *testing.T) {
	manager = &HTTPTest{Handler: Selenium()}

	resp, err := http.Post(With(srv.URL).Path("/wd/hub/session"), "", bytes.NewReader([]byte("{}")))
	assert.NoError(t, err)
	assert.Equal(t, resp.StatusCode, http.StatusOK)
	var sess map[string]string
	assert.NoError(t, json.NewDecoder(resp.Body).Decode(&sess))

	resp, err = http.Post(With(srv.URL).Path(fmt.Sprintf("/wd/hub/session/%s/file", sess["sessionId"])), "", bytes.NewReader([]byte(`{}`)))
	assert.NoError(t, err)
	assert.Equal(t, resp.StatusCode, http.StatusBadRequest)

	sessions.Remove(sess["sessionId"])
	queue.Release()
}

func TestFileUploadTwoFiles(t *testing.T) {
	manager = &HTTPTest{Handler: Selenium()}

	resp, err := http.Post(With(srv.URL).Path("/wd/hub/session"), "", bytes.NewReader([]byte("{}")))
	assert.NoError(t, err)
	assert.Equal(t, resp.StatusCode, http.StatusOK)
	var sess map[string]string
	assert.NoError(t, json.NewDecoder(resp.Body).Decode(&sess))

	resp, err = http.Post(With(srv.URL).Path(fmt.Sprintf("/wd/hub/session/%s/file", sess["sessionId"])), "", bytes.NewReader([]byte(`{"file":"UEsDBAoAAAAAAKGJ4koAAAAAAAAAAAAAAAAHABwAb25lLnR4dFVUCQADbv9YWZT/WFl1eAsAAQT1AQAABBQAAABQSwMECgAAAAAApIniSgAAAAAAAAAAAAAAAAcAHAB0d28udHh0VVQJAANz/1hZc/9YWXV4CwABBPUBAAAEFAAAAFBLAQIeAwoAAAAAAKGJ4koAAAAAAAAAAAAAAAAHABgAAAAAAAAAAACkgQAAAABvbmUudHh0VVQFAANu/1hZdXgLAAEE9QEAAAQUAAAAUEsBAh4DCgAAAAAApIniSgAAAAAAAAAAAAAAAAcAGAAAAAAAAAAAAKSBQQAAAHR3by50eHRVVAUAA3P/WFl1eAsAAQT1AQAABBQAAABQSwUGAAAAAAIAAgCaAAAAggAAAAAA"}`)))
	assert.NoError(t, err)
	assert.Equal(t, resp.StatusCode, http.StatusBadRequest)

	sessions.Remove(sess["sessionId"])
	queue.Release()
}

func TestPing(t *testing.T) {
	rsp, err := http.Get(With(srv.URL).Path("/ping"))

	assert.NoError(t, err)
	assert.Equal(t, rsp.StatusCode, http.StatusOK)
	assert.NotNil(t, rsp.Body)

	var data map[string]interface{}
	bt, readErr := io.ReadAll(rsp.Body)
	assert.NoError(t, readErr)
	jsonErr := json.Unmarshal(bt, &data)
	assert.NoError(t, jsonErr)
	_, hasUptime := data["uptime"]
	assert.True(t, hasUptime)
	_, hasLastReloadTime := data["lastReloadTime"]
	assert.True(t, hasLastReloadTime)
	_, hasNumRequests := data["numRequests"]
	assert.True(t, hasNumRequests)
	version, hasVersion := data["version"]
	assert.True(t, hasVersion)
	assert.Equal(t, version, "test-revision")
}

func TestStatus(t *testing.T) {
	rsp, err := http.Get(With(srv.URL).Path("/wd/hub/status"))

	assert.NoError(t, err)
	assert.Equal(t, rsp.StatusCode, http.StatusOK)
	assert.NotNil(t, rsp.Body)

	var data map[string]interface{}
	bt, readErr := io.ReadAll(rsp.Body)
	assert.NoError(t, readErr)
	jsonErr := json.Unmarshal(bt, &data)
	assert.NoError(t, jsonErr)
	value, hasValue := data["value"]
	assert.True(t, hasValue)
	valueMap := value.(map[string]interface{})
	ready, hasReady := valueMap["ready"]
	assert.True(t, hasReady)
	assert.Equal(t, ready, true)
	_, hasMessage := valueMap["message"]
	assert.True(t, hasMessage)
}

func TestServeAndDeleteVideoFile(t *testing.T) {
	fileName := "testfile"
	filePath := filepath.Join(videoOutputDir, fileName)
	_ = os.WriteFile(filePath, []byte("test-data"), 0644)

	rsp, err := http.Get(With(srv.URL).Path("/video/testfile"))
	assert.NoError(t, err)
	assert.Equal(t, rsp.StatusCode, http.StatusOK)

	rsp, err = http.Get(With(srv.URL).Path("/video/?json"))
	assert.NoError(t, err)
	assert.Equal(t, rsp.StatusCode, http.StatusOK)
	var files []string
	assert.NoError(t, json.NewDecoder(rsp.Body).Decode(&files))
	assert.Equal(t, files, []string{"testfile"})

	deleteReq, _ := http.NewRequest(http.MethodDelete, With(srv.URL).Path("/video/testfile"), nil)
	rsp, err = http.DefaultClient.Do(deleteReq)
	assert.NoError(t, err)
	assert.Equal(t, rsp.StatusCode, http.StatusOK)

	//Deleting already deleted file
	rsp, err = http.DefaultClient.Do(deleteReq)
	assert.NoError(t, err)
	assert.Equal(t, rsp.StatusCode, http.StatusNotFound)
}

func TestServeAndDeleteLogFile(t *testing.T) {
	fileName := "logfile.log"
	filePath := filepath.Join(logOutputDir, fileName)
	_ = os.WriteFile(filePath, []byte("test-data"), 0644)

	rsp, err := http.Get(With(srv.URL).Path("/logs/logfile.log"))
	assert.NoError(t, err)
	assert.Equal(t, rsp.StatusCode, http.StatusOK)

	rsp, err = http.Get(With(srv.URL).Path("/logs/?json"))
	assert.NoError(t, err)
	assert.Equal(t, rsp.StatusCode, http.StatusOK)
	var files []string
	assert.NoError(t, json.NewDecoder(rsp.Body).Decode(&files))
	assert.True(t, len(files) > 0)

	deleteReq, _ := http.NewRequest(http.MethodDelete, With(srv.URL).Path("/logs/logfile.log"), nil)
	rsp, err = http.DefaultClient.Do(deleteReq)
	assert.NoError(t, err)
	assert.Equal(t, rsp.StatusCode, http.StatusOK)

	rsp, err = http.DefaultClient.Do(deleteReq)
	assert.NoError(t, err)
	assert.Equal(t, rsp.StatusCode, http.StatusNotFound)
}

func TestFileDownload(t *testing.T) {
	testFileDownload(t, func(sessionId string) string {
		return fmt.Sprintf("/download/%s/testfile", sessionId)
	})
}

func TestFileDownloadProtocolExtension(t *testing.T) {
	testFileDownload(t, func(sessionId string) string {
		return fmt.Sprintf("/wd/hub/session/%s/aerokube/download/testfile", sessionId)
	})
}

func testFileDownload(t *testing.T, path func(string) string) {
	manager = &HTTPTest{Handler: Selenium()}

	resp, err := http.Post(With(srv.URL).Path("/wd/hub/session"), "", bytes.NewReader([]byte("{}")))
	assert.NoError(t, err)
	assert.Equal(t, resp.StatusCode, http.StatusOK)
	var sess map[string]string
	assert.NoError(t, json.NewDecoder(resp.Body).Decode(&sess))

	rsp, err := http.Get(With(srv.URL).Path(path(sess["sessionId"])))
	assert.NoError(t, err)
	assert.Equal(t, resp.StatusCode, http.StatusOK)
	data, err := io.ReadAll(rsp.Body)
	assert.NoError(t, err)
	assert.Equal(t, string(data), "test-data")

	sessions.Remove(sess["sessionId"])
	queue.Release()
}

func TestFileDownloadMissingSession(t *testing.T) {
	rsp, err := http.Get(With(srv.URL).Path("/download/missing-session/testfile"))
	assert.NoError(t, err)
	assert.Equal(t, rsp.StatusCode, http.StatusNotFound)
}

func TestClipboard(t *testing.T) {
	testClipboard(t, func(sessionId string) string {
		return fmt.Sprintf("/clipboard/%s", sessionId)
	})
}

func TestClipboardProtocolExtension(t *testing.T) {
	testClipboard(t, func(sessionId string) string {
		return fmt.Sprintf("/wd/hub/session/%s/aerokube/clipboard", sessionId)
	})
}

func testClipboard(t *testing.T, path func(string) string) {
	manager = &HTTPTest{Handler: Selenium()}

	resp, err := http.Post(With(srv.URL).Path("/wd/hub/session"), "", bytes.NewReader([]byte("{}")))
	assert.NoError(t, err)
	assert.Equal(t, resp.StatusCode, http.StatusOK)
	var sess map[string]string
	assert.NoError(t, json.NewDecoder(resp.Body).Decode(&sess))

	rsp, err := http.Get(With(srv.URL).Path(path(sess["sessionId"])))
	assert.NoError(t, err)
	assert.Equal(t, resp.StatusCode, http.StatusOK)
	data, err := io.ReadAll(rsp.Body)
	assert.NoError(t, err)
	assert.Equal(t, string(data), "test-clipboard-value")

	rsp, err = http.Post(With(srv.URL).Path(path(sess["sessionId"])), "text/plain", bytes.NewReader([]byte("any-data")))
	assert.NoError(t, err)
	assert.Equal(t, resp.StatusCode, http.StatusOK)

	sessions.Remove(sess["sessionId"])
	queue.Release()
}

func TestClipboardMissingSession(t *testing.T) {
	rsp, err := http.Get(With(srv.URL).Path("/clipboard/missing-session"))
	assert.NoError(t, err)
	assert.Equal(t, rsp.StatusCode, http.StatusNotFound)
}

func TestDevtools(t *testing.T) {
	manager = &HTTPTest{Handler: Selenium()}

	resp, err := http.Post(With(srv.URL).Path("/wd/hub/session"), "", bytes.NewReader([]byte("{}")))
	assert.NoError(t, err)
	assert.Equal(t, resp.StatusCode, http.StatusOK)
	var sess map[string]string
	assert.NoError(t, json.NewDecoder(resp.Body).Decode(&sess))

	u := fmt.Sprintf("ws://%s/devtools/%s", srv.Listener.Addr().String(), sess["sessionId"])

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	conn, err := rpcc.DialContext(ctx, u)
	assert.NoError(t, err)
	defer conn.Close()

	c := cdp.NewClient(conn)
	err = c.Page.Enable(ctx)
	assert.NoError(t, err)

	sessions.Remove(sess["sessionId"])
	queue.Release()
}

func TestAddedSeCdpCapability(t *testing.T) {
	fn := func(input map[string]interface{}) {
		input["value"] = map[string]interface{}{
			"sessionId":    input["sessionId"],
			"capabilities": make(map[string]interface{}),
		}
		delete(input, "sessionId")
	}
	manager = &HTTPTest{Handler: Selenium(fn)}

	resp, err := http.Post(With(srv.URL).Path("/wd/hub/session"), "", bytes.NewReader([]byte("{}")))
	assert.NoError(t, err)
	assert.Equal(t, resp.StatusCode, http.StatusOK)
	var sess map[string]interface{}
	assert.NoError(t, json.NewDecoder(resp.Body).Decode(&sess))

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	rv, ok := sess["value"]
	assert.True(t, ok)
	value, ok := rv.(map[string]interface{})
	assert.True(t, ok)
	rc, ok := value["capabilities"]
	assert.True(t, ok)
	rs, ok := value["sessionId"]
	assert.True(t, ok)
	sessionId, ok := rs.(string)
	assert.True(t, ok)
	capabilities, ok := rc.(map[string]interface{})
	assert.True(t, ok)
	rws, ok := capabilities["se:cdp"]
	assert.True(t, ok)
	ws, ok := rws.(string)
	assert.True(t, ok)
	assert.NotEmpty(t, ws)
	conn, err := rpcc.DialContext(ctx, ws)
	assert.NoError(t, err)
	defer conn.Close()

	c := cdp.NewClient(conn)
	err = c.Page.Enable(ctx)
	assert.NoError(t, err)

	sessions.Remove(sessionId)
	queue.Release()
}

func TestParseGgrHost(t *testing.T) {
	h := parseGgrHost("some-host.example.com:4444")
	assert.Equal(t, h.Name, "some-host.example.com")
	assert.Equal(t, h.Port, 4444)
}

func TestWelcomeScreen(t *testing.T) {
	rsp, err := http.Get(With(srv.URL).Path("/"))
	assert.NoError(t, err)
	assert.Equal(t, rsp.StatusCode, http.StatusOK)

	rsp, err = http.Get(With(srv.URL).Path("/wd/hub"))
	assert.NoError(t, err)
	assert.Equal(t, rsp.StatusCode, http.StatusOK)
}
