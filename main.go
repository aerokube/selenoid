package main

import (
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/aandryashin/selenoid/config"
	"github.com/aandryashin/selenoid/docker"
	"github.com/aandryashin/selenoid/handler"
	"github.com/aandryashin/selenoid/service"
	"github.com/aandryashin/selenoid/session"
	"github.com/docker/engine-api/client"
)

var (
	listen  string
	timeout time.Duration
	logHTTP bool
	limit   int
	cfgfile string
	queue   chan struct{}
	conf    *config.Config
	manager service.Finder
)

func init() {
	flag.StringVar(&listen, "listen", ":4444", "Network address to accept connections")
	flag.StringVar(&cfgfile, "conf", "config/browsers.json", "Browsers configuration file")
	flag.IntVar(&limit, "limit", 5, "Simultanious container runs")
	flag.DurationVar(&timeout, "timeout", 60*time.Second, "Session idle timeout in time.Duration format")
	flag.BoolVar(&logHTTP, "log-http", false, "Log HTTP traffic")
	flag.Parse()

	queue = make(chan struct{}, limit)

	defaultHeaders := map[string]string{"User-Agent": "engine-api-cli-1.0"}
	cli, err := client.NewClient("unix:///var/run/docker.sock", client.DefaultVersion, nil, defaultHeaders)
	if err != nil {
		log.Fatalf("error: unable to create client connection to docker daemon")
	}

	conf, err = config.NewConfig(cfgfile, limit)
	if err != nil {
		log.Fatalf("error loading configuration: %s\n", err)
	}

	manager = &docker.Manager{cli, conf}

	cancelOnSignal()
}

func cancelOnSignal() {
	sig := make(chan os.Signal)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sig
		sessions.Each(func(k string, s *session.Session) {
			s.Cancel()
		})
		os.Exit(1)
	}()
}

func main() {
	h := Handler()
	if logHTTP {
		h = handler.Dumper(h)
	}
	log.Printf("Listening on %s\n", listen)
	log.Fatal(http.ListenAndServe(listen, h))
}
