package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

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
	conf    string
	queue   chan struct{}
	manager service.Finder
)

func init() {
	flag.StringVar(&listen, "listen", ":4444", "Network address to accept connections")
	flag.StringVar(&conf, "conf", "browsers.json", "Browsers configuration file")
	flag.IntVar(&limit, "limit", 5, "Simultanious container runs")
	flag.DurationVar(&timeout, "timeout", 60*time.Second, "Session idle timeout in time.Duration format")
	flag.BoolVar(&logHTTP, "log-http", false, "Log HTTP traffic")
	flag.Parse()
	queue = make(chan struct{}, limit)
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
	config, err := docker.NewConfig(conf)
	if err != nil {
		fmt.Printf("error loading configuration: %s\n", err)
		os.Exit(1)
	}
	defaultHeaders := map[string]string{"User-Agent": "engine-api-cli-1.0"}
	cli, err := client.NewClient("unix:///var/run/docker.sock", client.DefaultVersion, nil, defaultHeaders)
	if err != nil {
		fmt.Println("error: unable to create client connection to docker daemon")
		os.Exit(1)
	}
	manager = &docker.Manager{cli, config}
	h := Handler()
	cancelOnSignal()
	if logHTTP {
		h = handler.Dumper(h)
	}
	log.Printf("Listening on %s\n", listen)
	log.Fatal(http.ListenAndServe(listen, h))
}
