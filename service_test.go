package main

import (
	"fmt"
	. "github.com/aandryashin/matchers"
	"github.com/aerokube/selenoid/config"
	"github.com/aerokube/selenoid/service"
	"github.com/aerokube/selenoid/session"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
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
	mux.HandleFunc("/v1.29/containers/e90e34656806", http.HandlerFunc(
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
			`, p, p, p)
			w.Write([]byte(output))
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
	return &service.Environment{
		CPU:                 int64(0),
		Memory:              int64(0),
		Network:             containerNetwork,
		StartupTimeout:      serviceStartupTimeout,
		CaptureDriverLogs:   captureDriverLogs,
		VideoContainerImage: "aerokube/video-recorder",
		VideoOutputDir:      "/some/dir",
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
	AssertThat(t, startedService.VNCHostPort, EqualTo{"127.0.0.1:5900"})
	AssertThat(t, startedService.Cancel, Not{nil})
	startedService.Cancel()
}

func createDockerStarter(t *testing.T, env *service.Environment, cfg *config.Config) service.Starter {
	cli, err := client.NewEnvClient()
	AssertThat(t, err, Is{nil})
	manager := service.DefaultManager{Environment: env, Client: cli, Config: cfg}
	caps := session.Caps{
		Name:                  "firefox",
		Version:               "33.0",
		ScreenResolution:      "1024x768",
		VNC:                   true,
		Video:                 true,
		VideoScreenSize:       "1024x768",
		VideoFrameRate:        25,
		HostsEntries:          "example.com:192.168.0.1,test.com:192.168.0.2",
		Labels:                "label1:some-value,label2",
		ApplicationContainers: "one,two",
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
