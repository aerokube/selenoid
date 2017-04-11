package main

import (
	"encoding/json"
	"flag"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"fmt"

	"github.com/aandryashin/selenoid/config"
	"github.com/aandryashin/selenoid/protect"
	"github.com/aandryashin/selenoid/service"
	"github.com/aandryashin/selenoid/session"
	"github.com/docker/docker/client"
)

var (
	disableDocker bool
	listen        string
	timeout       time.Duration
	limit         int
	sessions      = session.NewMap()
	confPath      string
	logConfPath   string
	conf          *config.Config
	queue         *protect.Queue
	manager       service.Manager
	cli           *client.Client
)

func init() {
	flag.BoolVar(&disableDocker, "disable-docker", false, "Disable docker support")
	flag.StringVar(&listen, "listen", ":4444", "Network address to accept connections")
	flag.StringVar(&confPath, "conf", "config/browsers.json", "Browsers configuration file")
	flag.StringVar(&logConfPath, "log-conf", "config/container-logs.json", "Container logging configuration file")
	flag.IntVar(&limit, "limit", 5, "Simultaneous container runs")
	flag.DurationVar(&timeout, "timeout", 60*time.Second, "Session idle timeout in time.Duration format")
	flag.Parse()

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
	if disableDocker {
		manager = &service.DefaultManager{Config: conf}
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
	cli, err = client.NewClient(dockerHost, client.DefaultVersion, nil, nil)
	if err != nil {
		log.Fatalf("docker error: %v\n", err)
	}
	manager = &service.DefaultManager{IP: ip, Client: cli, Config: conf}
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
	return root
}

func main() {
	log.Printf("Timezone: %s\n", time.Local)
	log.Printf("Listening on %s\n", listen)
	log.Fatal(http.ListenAndServe(listen, handler()))
}
