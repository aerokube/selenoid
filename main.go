package main

import (
	"encoding/json"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/aandryashin/selenoid/config"
	"github.com/aandryashin/selenoid/handler"
	"github.com/aandryashin/selenoid/service"
	"github.com/aandryashin/selenoid/session"
	"github.com/docker/engine-api/client"
)

var (
	disableDocker bool
	dockerApi     string
	dockerHeaders = map[string]string{"User-Agent": "engine-api-cli-1.0"}
	listen        string
	timeout       time.Duration
	logHTTP       bool
	limit         int
	conf          string
	queue         chan struct{}
	queued        chan struct{} = make(chan struct{}, 2^64-1)
	sessions      *session.Map  = session.NewMap()
	cfg           *config.Config
	manager       service.Manager
)

func init() {
	flag.BoolVar(&disableDocker, "disable-docker", false, "Disable docker support")
	flag.StringVar(&listen, "listen", ":4444", "Network address to accept connections")
	flag.StringVar(&conf, "conf", "config/browsers.json", "Browsers configuration file")
	flag.StringVar(&dockerApi, "docker-api", "unix:///var/run/docker.sock", "Docker api url")
	flag.IntVar(&limit, "limit", 5, "Simultanious container runs")
	flag.DurationVar(&timeout, "timeout", 60*time.Second, "Session idle timeout in time.Duration format")
	flag.BoolVar(&logHTTP, "log-http", false, "Log HTTP traffic")
	flag.Parse()
	queue = make(chan struct{}, limit)
	var err error
	cfg, err = config.NewConfig(conf, limit)
	if err != nil {
		log.Fatalf("error loading configuration: %s\n", err)
	}
	var cli *client.Client
	if !disableDocker {
		cli, err = client.NewClient(dockerApi, client.DefaultVersion, nil, dockerHeaders)
		if err != nil {
			log.Fatal("warning: unable to create client connection to docker daemon.")
		}
	}
	cancelOnSignal()
	manager = &service.DefaultManager{cli, cfg}
}

func cancelOnSignal() {
	sig := make(chan os.Signal)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sig
		sessions.Each(func(k string, s *session.Session) {
			s.Cancel()
		})
		os.Exit(0)
	}()
}

func Handler() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/session", handler.OnlyPost(create))
	mux.Handle("/session/", handler.AnyMethod(proxy))
	root := http.NewServeMux()
	root.Handle("/wd/hub/", http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			r.URL.Scheme = "http"
			r.URL.Host = (&request{r}).localaddr()
			r.URL.Path = strings.TrimPrefix(r.URL.Path, "/wd/hub")
			mux.ServeHTTP(w, r)
		}))
	root.Handle("/error", http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, `{"value":{"message":"Session not found"},"status":13}`, http.StatusNotFound)
		}))
	root.Handle("/status", http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			json.NewEncoder(w).Encode(cfg.State(sessions, len(queued)))
		}))
	return root
}

func main() {
	h := Handler()
	if logHTTP {
		h = handler.Dumper(h)
	}
	log.Printf("Listening on %s\n", listen)
	log.Fatal(http.ListenAndServe(listen, h))
}
