package main

import (
	"encoding/json"
	"flag"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/aandryashin/selenoid/config"
	"github.com/aandryashin/selenoid/ensure"
	"github.com/aandryashin/selenoid/protect"
	"github.com/aandryashin/selenoid/service"
	"github.com/aandryashin/selenoid/session"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/facebookgo/grace/gracehttp"
)

var (
	disableDocker bool
	dockerAPI     string
	listen        string
	timeout       time.Duration
	limit         int
	conf          string
	sessions      = session.NewMap()
	cfg           *config.Config
	queue         *protect.Queue
	manager       service.Manager
)

func init() {
	lcFilename := ""
	flag.BoolVar(&disableDocker, "disable-docker", false, "Disable docker support")
	flag.StringVar(&listen, "listen", ":4444", "Network address to accept connections")
	flag.StringVar(&conf, "conf", "config/browsers.json", "Browsers configuration file")
	flag.StringVar(&lcFilename, "log-conf", "", "Container logging configuration file")
	flag.StringVar(&dockerAPI, "docker-api", "unix:///var/run/docker.sock", "Docker api url")
	flag.IntVar(&limit, "limit", 5, "Simultaneous container runs")
	flag.DurationVar(&timeout, "timeout", 60*time.Second, "Session idle timeout in time.Duration format")
	flag.Parse()
	var lc container.LogConfig
	if lcFilename != "" {
		err := config.Load(lcFilename, &lc)
		if err != nil {
			log.Fatal(err)
		}
	}
	queue = protect.New(limit)
	var err error
	cfg, err = config.New(conf, limit)
	if err != nil {
		log.Fatalf("error loading configuration: %v\n", err)
	}
	var cli *client.Client
	if !disableDocker {
		cli, err = client.NewClient(dockerAPI, client.DefaultVersion, nil, nil)
		if err != nil {
			log.Fatal("unable to create client connection to docker daemon.")
		}
	}
	u, err := url.Parse(dockerAPI)
	if err != nil {
		log.Fatalf("malformed docker api url %s: %v\n,", dockerAPI, err)
	}
	ip, _, _ := net.SplitHostPort(u.Host)
	cancelOnSignal()
	manager = &service.DefaultManager{ip, cli, cfg, lc}
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

func mux() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/session", ensure.Post(queue.Protect(create)))
	mux.HandleFunc("/session/", proxy)
	return mux
}

func handler() http.Handler {
	root := http.NewServeMux()
	root.Handle("/wd/hub/", http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			r.URL.Scheme = "http"
			r.URL.Host = (&request{r}).localaddr()
			r.URL.Path = strings.TrimPrefix(r.URL.Path, "/wd/hub")
			mux().ServeHTTP(w, r)
		}))
	root.Handle("/error", http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, `{"value":{"message":"Session not found"},"status":13}`, http.StatusNotFound)
		}))
	root.Handle("/status", http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			json.NewEncoder(w).Encode(cfg.State(sessions, queue.Queued(), queue.Pending()))
		}))
	return root
}

func main() {
	log.Printf("Listening on %s\n", listen)
	log.Fatal(
		gracehttp.Serve([]*http.Server{
			{Addr: listen, Handler: handler()},
		}...),
	)
}
