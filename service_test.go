package main

import (
	"bytes"
	"fmt"
	. "github.com/aandryashin/matchers"
	"github.com/aerokube/selenoid/config"
	"github.com/aerokube/selenoid/service"
	"github.com/aerokube/selenoid/session"
	"github.com/aerokube/util"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"golang.org/x/net/websocket"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"sync"
	"testing"
	"time"
)

var (
	mockServer *httptest.Server
	lock       sync.Mutex
)

func init() {
	updateMux(testMux())
	timeout = 2 * time.Second
	serviceStartupTimeout = 1 * time.Second
	newSessionAttemptTimeout = 1 * time.Second
	sessionDeleteTimeout = 1 * time.Second
}

func updateMux(mux http.Handler) {
	lock.Lock()
	defer lock.Unlock()
	mockServer = httptest.NewServer(mux)
	os.Setenv("DOCKER_HOST", "tcp://"+hostPort(mockServer.URL))
	os.Setenv("DOCKER_API_VERSION", "1.29")
	cli, _ = client.NewClientWithOpts(client.FromEnv)
}

func testMux() http.Handler {
	mux := http.NewServeMux()

	//Selenium Hub mock
	mux.HandleFunc("/wd/hub", http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		},
	))

	//Docker API mock
	mux.HandleFunc("/v1.29/containers/create", http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusCreated)
			output := `{"id": "e90e34656806", "warnings": []}`
			w.Write([]byte(output))
		},
	))
	mux.HandleFunc("/v1.29/containers/e90e34656806/start", http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		},
	))
	mux.HandleFunc("/v1.29/containers/e90e34656806/kill", http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		},
	))
	mux.HandleFunc("/v1.29/containers/e90e34656806/logs", http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			w.Header().Add("Content-Type", "text/plain; charset=utf-8")
			w.Header().Add("Transfer-Encoding", "chunked")
			w.WriteHeader(http.StatusOK)
			const streamTypeStderr = 2
			header := []byte{streamTypeStderr, 0, 0, 0, 0, 0, 0, 9}
			w.Write(header)
			data := []byte("test-data")
			w.Write(data)
		},
	))
	mux.HandleFunc("/v%s/containers/e90e34656806", http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		},
	))
	mux.HandleFunc("/v1.29/containers/e90e34656806/json", http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			p := port(mockServer.URL)
			output := fmt.Sprintf(`			
			{
			  "Id": "e90e34656806",
			  "Created": "2015-01-06T15:47:31.485331387Z",
			  "Driver": "aufs",
			  "HostConfig": {},
			  "NetworkSettings": {
			    "Ports": {
					"4444/tcp": [
						{
						"HostIp": "0.0.0.0",
						"HostPort": "%s"
						}
					],
					"7070/tcp": [
						{
						"HostIp": "0.0.0.0",
						"HostPort": "%s"
						}
					],
					"8080/tcp": [
						{
						"HostIp": "0.0.0.0",
						"HostPort": "%s"
						}
					],
					"9090/tcp": [
						{
						"HostIp": "0.0.0.0",
						"HostPort": "%s"
						}
					],
					"5900/tcp": [
						{
						"HostIp": "0.0.0.0",
						"HostPort": "5900"
						}
					],
					"%s/tcp": [
						{
						"HostIp": "0.0.0.0",
						"HostPort": "%s"
						}
					]
			    },
				"Networks": {
					"bridge": {
						"IPAMConfig": null,
						"Links": null,
						"Aliases": null,
						"NetworkID": "0152391a00ed79360bcf69401f7e2659acfab9553615726dbbcfc08b4f367b25",
						"EndpointID": "6a36b6f58b37490666329fd0fd74b21aa4eba939dd1ce466bdb6e0f826d56f98",
						"Gateway": "127.0.0.1",
						"IPAddress": "127.0.0.1",
						"IPPrefixLen": 16,
						"IPv6Gateway": "",
						"GlobalIPv6Address": "",
						"GlobalIPv6PrefixLen": 0,
						"MacAddress": "02:42:ac:11:00:02"
					}
				}			
			  },
			  "State": {},
			  "Mounts": []
			}
			`, p, p, p, p, p, p)
			w.Write([]byte(output))
		},
	))
	mux.HandleFunc("/v1.29/networks/net-1/connect", http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		},
	))
	return mux
}

func parseUrl(input string) *url.URL {
	u, err := url.Parse(input)
	if err != nil {
		panic(err)
	}
	return u
}

func hostPort(input string) string {
	return parseUrl(input).Host
}

func port(input string) string {
	return parseUrl(input).Port()
}

func testConfig(env *service.Environment) *config.Config {
	conf := config.NewConfig()
	p := "4444"
	if env.InDocker {
		p = port(mockServer.URL)
	}
	conf.Browsers["firefox"] = config.Versions{
		Default: "33.0",
		Versions: map[string]*config.Browser{
			"33.0": {
				Image:   "selenoid/firefox:33.0",
				Tmpfs:   map[string]string{"/tmp": "size=128m"},
				Port:    p,
				Volumes: []string{"/test:/test"},
				Labels:  map[string]string{"key": "value"},
				Sysctl:  map[string]string{"sysctl net.ipv4.tcp_timestamps": "2"},
				Mem:     "512m",
				Cpu:     "1.0",
			},
		},
	}
	conf.Browsers["internet explorer"] = config.Versions{
		Default: "11",
		Versions: map[string]*config.Browser{
			"11": {
				Image: []interface{}{
					"/usr/bin/test-command", "-arg",
				},
			},
		},
	}
	return conf
}

func testEnvironment() *service.Environment {
	logOutputDir, _ = ioutil.TempDir("", "selenoid-test")
	return &service.Environment{
		CPU:                 int64(0),
		Memory:              int64(0),
		Network:             containerNetwork,
		StartupTimeout:      serviceStartupTimeout,
		CaptureDriverLogs:   captureDriverLogs,
		VideoContainerImage: "aerokube/video-recorder",
		VideoOutputDir:      "/some/dir",
		LogOutputDir:        logOutputDir,
		Privileged:          false,
	}
}

func TestFindOutsideOfDocker(t *testing.T) {
	env := testEnvironment()
	env.InDocker = false
	testDocker(t, env, testConfig(env))
}

func TestFindInsideOfDocker(t *testing.T) {
	env := testEnvironment()
	env.InDocker = true
	cfg := testConfig(env)
	logConfig := make(map[string]string)
	cfg.ContainerLogs = &container.LogConfig{
		Type:   "rsyslog",
		Config: logConfig,
	}
	testDocker(t, env, cfg)
}

func TestFindDockerIPSpecified(t *testing.T) {
	env := testEnvironment()
	env.IP = "127.0.0.1"
	testDocker(t, env, testConfig(env))
}

func testDocker(t *testing.T, env *service.Environment, cfg *config.Config) {
	starter := createDockerStarter(t, env, cfg)
	startedService, err := starter.StartWithCancel()
	AssertThat(t, err, Is{nil})
	AssertThat(t, startedService.Url, Not{nil})
	AssertThat(t, startedService.Container, Not{nil})
	AssertThat(t, startedService.Container.ID, EqualTo{"e90e34656806"})
	AssertThat(t, startedService.HostPort.VNC, EqualTo{"127.0.0.1:5900"})
	AssertThat(t, startedService.Cancel, Not{nil})
	startedService.Cancel()
}

func createDockerStarter(t *testing.T, env *service.Environment, cfg *config.Config) service.Starter {
	cli, err := client.NewClientWithOpts(client.FromEnv)
	AssertThat(t, err, Is{nil})
	manager := service.DefaultManager{Environment: env, Client: cli, Config: cfg}
	caps := session.Caps{
		DeviceName:            "firefox",
		Version:               "33.0",
		ScreenResolution:      "1024x768",
		Skin:                  "WXGA800",
		VNC:                   true,
		Video:                 true,
		VideoScreenSize:       "1024x768",
		VideoFrameRate:        25,
		VideoCodec:            "libx264",
		Log:                   true,
		LogName:               "testfile",
		Env:                   []string{"LANG=ru_RU.UTF-8", "LANGUAGE=ru:en"},
		HostsEntries:          []string{"example.com:192.168.0.1", "test.com:192.168.0.2"},
		DNSServers:            []string{"192.168.0.1", "192.168.0.2"},
		Labels:                map[string]string{"label1": "some-value", "label2": ""},
		ApplicationContainers: []string{"one", "two"},
		AdditionalNetworks:    []string{"net-1"},
		TimeZone:              "Europe/Moscow",
		ContainerHostname:     "some-hostname",
		TestName:              "my-cool-test",
	}
	starter, success := manager.Find(caps, 42)
	AssertThat(t, success, Is{true})
	AssertThat(t, starter, Not{nil})
	return starter
}

func failingMux(numDeleteRequests *int) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1.29/containers/create", http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusCreated)
			output := `{"id": "e90e34656806", "warnings": []}`
			w.Write([]byte(output))
		},
	))
	mux.HandleFunc("/v1.29/containers/e90e34656806/start", http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		},
	))
	mux.HandleFunc("/v1.29/containers/e90e34656806", http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			*numDeleteRequests++
			w.WriteHeader(http.StatusNoContent)
		},
	))
	return mux
}

func TestDeleteContainerOnStartupError(t *testing.T) {
	numDeleteRequests := 0
	updateMux(failingMux(&numDeleteRequests))
	defer updateMux(testMux())
	env := testEnvironment()
	starter := createDockerStarter(t, env, testConfig(env))
	_, err := starter.StartWithCancel()
	AssertThat(t, err, Not{nil})
	AssertThat(t, numDeleteRequests, EqualTo{1})
}

func TestFindDriver(t *testing.T) {
	env := testEnvironment()
	manager := service.DefaultManager{Environment: env, Config: testConfig(env)}
	caps := session.Caps{
		Name:             "internet explorer", //Using default version
		ScreenResolution: "1024x768",
		VNC:              true,
	}
	starter, success := manager.Find(caps, 42)
	AssertThat(t, success, Is{true})
	AssertThat(t, starter, Not{nil})
}

func TestGetVNC(t *testing.T) {

	srv := httptest.NewServer(handler())
	defer srv.Close()

	testTcpServer := testTCPServer("test-data")
	sessions.Put("test-session", &session.Session{
		HostPort: session.HostPort{
			VNC: testTcpServer.Addr().String(),
		},
	})
	defer sessions.Remove("test-session")

	u := fmt.Sprintf("ws://%s/vnc/test-session", util.HostPort(srv.URL))
	AssertThat(t, readDataFromWebSocket(t, u), EqualTo{"test-data"})
}

func testTCPServer(data string) net.Listener {
	l, _ := net.Listen("tcp", "127.0.0.1:0")
	go func() {
		for {
			conn, err := l.Accept()
			if err != nil {
				continue
			}
			io.WriteString(conn, data)
			conn.Close()
			return
		}
	}()
	return l
}

func readDataFromWebSocket(t *testing.T, wsURL string) string {
	ws, err := websocket.Dial(wsURL, "", "http://localhost")
	AssertThat(t, err, Is{nil})

	var msg = make([]byte, 512)
	_, err = ws.Read(msg)
	msg = bytes.Trim(msg, "\x00")
	//AssertThat(t, err, Is{nil})
	return string(msg)
}

func TestGetLogs(t *testing.T) {

	srv := httptest.NewServer(handler())
	defer srv.Close()

	sessions.Put("test-session", &session.Session{
		Container: &session.Container{
			ID:        "e90e34656806",
			IPAddress: "127.0.0.1",
		},
	})
	defer sessions.Remove("test-session")

	u := fmt.Sprintf("ws://%s/logs/test-session", util.HostPort(srv.URL))
	AssertThat(t, readDataFromWebSocket(t, u), EqualTo{"test-data"})
}
