package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/aandryashin/selenoid/handler"
	"github.com/aandryashin/selenoid/service"
	"github.com/aandryashin/selenoid/session"
	"github.com/docker/engine-api/client"
)

type stringSlice []string

func (sslice *stringSlice) String() string {
	return fmt.Sprintf("%v", *sslice)
}

func (sslice *stringSlice) Set(value string) error {
	for _, s := range strings.Split(value, ",") {
		*sslice = append(*sslice, strings.TrimSpace(s))
	}
	return nil
}

var (
	listen      string
	timeout     time.Duration
	logHTTP     bool
	limit       int
	dockerImage string
	driverPort  string
	driverPath  string
)

func init() {
	flag.StringVar(&listen, "listen", ":4444", "network address to accept connections")
	flag.StringVar(&dockerImage, "docker-image", "", "Docker container image (required)")
	flag.IntVar(&limit, "limit", 5, "Simultanious container runs")
	flag.StringVar(&driverPort, "driver-port", "4444", "Underlying webdriver port")
	flag.StringVar(&driverPath, "driver-path", "", "Underlying webdriver path e.g. /wd/hub")
	flag.DurationVar(&timeout, "timeout", 60*time.Second, "session idle timeout in time.Duration format")
	flag.BoolVar(&logHTTP, "log-http", false, "log HTTP traffic")
	flag.Parse()
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
	if dockerImage == "" {
		flag.Usage()
		fmt.Println("error: docker-image is not set")
		os.Exit(1)
	}
	u := fmt.Sprintf("http://localhost:%s%s", driverPort, driverPath)
	if _, err := url.Parse(u); err != nil {
		flag.Usage()
		fmt.Println("error: invalid port number or driver path")
		os.Exit(1)
	}
	defaultHeaders := map[string]string{"User-Agent": "engine-api-cli-1.0"}
	cli, err := client.NewClient("unix:///var/run/docker.sock", client.DefaultVersion, nil, defaultHeaders)
	if err != nil {
		fmt.Println("error: unable to create client connection to docker daemon")
		os.Exit(1)
	}
	h := Handler(&service.Docker{dockerImage, driverPort, driverPath, cli}, limit)
	cancelOnSignal()
	if logHTTP {
		h = handler.Dumper(h)
	}
	log.Printf("Listening on %s\n", listen)
	log.Fatal(http.ListenAndServe(listen, h))
}
