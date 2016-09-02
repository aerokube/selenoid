package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/aandryashin/selenoid/handler"
	"github.com/aandryashin/selenoid/service"
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
	listen  string
	timeout time.Duration
	logHTTP bool
	nodes   stringSlice
)

func init() {
	flag.StringVar(&listen, "listen", ":4444", "network address to accept connections")
	flag.DurationVar(&timeout, "timeout", 5*time.Second, "session idle timeout in seconds")
	flag.BoolVar(&logHTTP, "log-http", false, "log HTTP traffic")
	flag.Var(&nodes, "nodes", "comma separated underlying driver's or node's urls (required)")
	flag.Parse()
}

func main() {
	d, err := service.NewDriver(nodes)
	if err != nil {
		flag.Usage()
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	log.Println("Initializing nodes with", nodes)
	h := Handler(d, len(nodes))
	if logHTTP {
		h = handler.Dumper(h)
	}
	log.Printf("Listening on %s\n", listen)
	log.Fatal(http.ListenAndServe(listen, h))
}
