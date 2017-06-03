package main

import (
	"encoding/json"
	"flag"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/docker/go-units"
	"golang.org/x/net/websocket"

	"fmt"

	"github.com/aerokube/selenoid/config"
	"github.com/aerokube/selenoid/protect"
	"github.com/aerokube/selenoid/service"
	"github.com/aerokube/selenoid/session"
	"github.com/docker/docker/client"
)

type memLimit int64

func (m *memLimit) String() string {
	return units.HumanSize(float64(*m))
}

func (limit *memLimit) Set(s string) error {
	v, err := units.RAMInBytes(s)
	if err != nil {
		return fmt.Errorf("set memory limit: %v", err)
	}
	*limit = memLimit(v)
	return nil
}

type cpuLimit int64

func (limit *cpuLimit) String() string {
	return strconv.FormatFloat(float64(*limit/1000000000), 'f', -1, 64)
}

func (limit *cpuLimit) Set(s string) error {
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return fmt.Errorf("set cpu limit: %v", err)
	}
	*limit = cpuLimit(v * 1000000000)
	return nil
}

var (
	disableDocker            bool
	listen                   string
	timeout                  time.Duration
	newSessionAttemptTimeout time.Duration
	sessionDeleteTimeout     time.Duration
	limit                    int
	sessions                 = session.NewMap()
	confPath                 string
	logConfPath              string
	conf                     *config.Config
	queue                    *protect.Queue
	manager                  service.Manager
	cli                      *client.Client

	version     bool
	gitRevision string = "HEAD"
	buildStamp  string = "unknown"
)

func init() {
	var mem memLimit
	var cpu cpuLimit
	flag.BoolVar(&disableDocker, "disable-docker", false, "Disable docker support")
	flag.StringVar(&listen, "listen", ":4444", "Network address to accept connections")
	flag.StringVar(&confPath, "conf", "config/browsers.json", "Browsers configuration file")
	flag.StringVar(&logConfPath, "log-conf", "config/container-logs.json", "Container logging configuration file")
	flag.IntVar(&limit, "limit", 5, "Simultaneous container runs")
	flag.DurationVar(&timeout, "timeout", 60*time.Second, "Session idle timeout in time.Duration format")
	flag.DurationVar(&newSessionAttemptTimeout, "session-attempt-timeout", 30*time.Second, "New session attempt timeout in time.Duration format")
	flag.DurationVar(&sessionDeleteTimeout, "session-delete-timeout", 30*time.Second, "Session delete timeout in time.Duration format")
	flag.BoolVar(&version, "version", false, "Show version and exit")
	flag.Var(&mem, "mem", "Containers memory limit e.g. 128m or 1g")
	flag.Var(&cpu, "cpu", "Containers cpu limit as float e.g. 0.2 or 1.0")
	flag.Parse()

	if version {
		showVersion()
		os.Exit(0)
	}

	queue = protect.New(limit)
	conf = config.NewConfig()
	err := conf.Load(confPath, logConfPath)
	if err != nil {
		log.Fatalf("%s: %v", os.Args[0], err)
	}
	onSIGHUP(func() {
		err := conf.Load(confPath, logConfPath)
		if err != nil {
			log.Printf("%s: %v", os.Args[0], err)
		}
	})
	cancelOnSignal()
	inDocker := false
	_, err = os.Stat("/.dockerenv")
	if err == nil {
		inDocker = true
	}
	if disableDocker {
		manager = &service.DefaultManager{InDocker: inDocker, Config: conf}
		return
	}
	dockerHost := os.Getenv("DOCKER_HOST")
	if dockerHost == "" {
		dockerHost = client.DefaultDockerHost
	}
	_, addr, _, err := client.ParseHost(dockerHost)
	if err != nil {
		log.Fatal(err)
	}
	ip, _, _ := net.SplitHostPort(addr)
	cli, err = client.NewEnvClient()
	if err != nil {
		log.Fatalf("new docker client: %v\n", err)
	}
	manager = &service.DefaultManager{IP: ip, InDocker: inDocker, CPU: int64(cpu), Memory: int64(mem), Client: cli, Config: conf}
}

func cancelOnSignal() {
	sig := make(chan os.Signal)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sig
		sessions.Each(func(k string, s *session.Session) {
			s.Cancel()
		})
		err := cli.Close()
		if err != nil {
			log.Fatalf("close docker client: %v", err)
		}
		os.Exit(0)
	}()
}

func onSIGHUP(fn func()) {
	sig := make(chan os.Signal)
	signal.Notify(sig, syscall.SIGHUP)
	go func() {
		for {
			<-sig
			fn()
		}
	}()
}

func mux() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/session", queue.Protect(post(create)))
	mux.HandleFunc("/session/", proxy)
	return mux
}

func post(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		next.ServeHTTP(w, r)
	}
}

func handler() http.Handler {
	root := http.NewServeMux()
	root.Handle("/wd/hub/", http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			w.Header().Add("Content-Type", "application/json")
			r.URL.Scheme = "http"
			r.URL.Host = (&request{r}).localaddr()
			r.URL.Path = strings.TrimPrefix(r.URL.Path, "/wd/hub")
			mux().ServeHTTP(w, r)
		}))
	root.Handle("/error", http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			w.Header().Add("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			fmt.Fprintln(w, `{"value":{"message":"Session not found"},"status":13}`)
		}))
	root.Handle("/status", http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			json.NewEncoder(w).Encode(conf.State(sessions, limit, queue.Queued(), queue.Pending()))
		}))
	root.Handle("/vnc/", websocket.Handler(vnc))
	root.Handle("/logs/", websocket.Handler(logs))
	return root
}

func showVersion() {
	fmt.Printf("Git Revision: %s\n", gitRevision)
	fmt.Printf("UTC Build Time: %s\n", buildStamp)
}

func main() {
	log.Printf("Timezone: %s\n", time.Local)
	log.Printf("Listening on %s\n", listen)
	log.Fatal(http.ListenAndServe(listen, handler()))
}
