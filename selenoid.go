package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"os"
	"strings"
	"sync"
	"time"
)

const (
	errPath = "/error"
)

type session struct {
	host   string
	cancel chan struct{}
}

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
	nodes   stringSlice
	hosts   chan string
	route   map[string]*session = make(map[string]*session)
	lock    sync.RWMutex
)

func sanitize(l string) string {
	host, port, _ := net.SplitHostPort(l)
	if host == "" {
		return net.JoinHostPort("localhost", port)
	}
	return l
}

func errFunc(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "Session not found", http.StatusNotFound)
}

func status(w http.ResponseWriter, r *http.Request) {
	status := make(map[string]string)
	lock.RLock()
	for k, v := range route {
		status[k] = v.host
	}
	lock.RUnlock()
	json.NewEncoder(w).Encode(&status)
}

func create(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	log.Println("[NEW_REQUEST] - Waiting for a free node")
	host := <-hosts
	r.URL.Scheme = "http"
	r.URL.Host = host
	r.URL.Path = strings.Replace(r.URL.Path, "/wd/hub", "", 1)
	log.Printf("[SESSION_ATTEMPTED] [%s]\n", host)
	resp, err := http.Post(r.URL.String(), "", r.Body)
	if err != nil {
		log.Printf("[SESSION_FAILED] [%s] - [%s]\n", host, err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		hosts <- host
		return
	}
	w.WriteHeader(resp.StatusCode)
	var s struct {
		Id string `json:"sessionId"`
	}
	tee := io.TeeReader(resp.Body, w)
	json.NewDecoder(tee).Decode(&s)
	if s.Id == "" {
		hosts <- host
		return
	}
	lock.Lock()
	route[s.Id] = &session{host, onTimeout(timeout, func() { deleteSession(s.Id) })}
	lock.Unlock()
	log.Printf("[SESSION_CREATED] [%s] [%s]\n", s.Id, host)
}

func proxy(r *http.Request) {
	dump, _ := httputil.DumpRequest(r, true)
	log.Println(string(dump))
	r.URL.Scheme = "http"
	sid := strings.Split(r.URL.Path, "/")[4]
	lock.RLock()
	s, ok := route[sid]
	lock.RUnlock()
	if ok {
		close(s.cancel)
		r.URL.Host = s.host
		r.URL.Path = strings.Replace(r.URL.Path, "/wd/hub", "", 1)
		if r.Method != http.MethodDelete {
			lock.Lock()
			s.cancel = onTimeout(timeout, func() { deleteSession(sid) })
			lock.Unlock()
			return
		}
		lock.Lock()
		delete(route, sid)
		hosts <- s.host
		lock.Unlock()
		log.Printf("[SESSION_DELETED] [%s]\n", sid)
		return
	}
	r.URL.Host = sanitize(listen)
	r.URL.Path = errPath
}

func deleteSession(id string) {
	log.Printf("[SESSION_TIMED_OUT] [%s] - Deleting session\n", id)
	req, err := http.NewRequest(http.MethodDelete, fmt.Sprintf("http://%s/wd/hub/session/%s", sanitize(listen), id), nil)
	if err != nil {
		log.Printf("[DELETE_FAILED] [%s] - %s\n", id, err.Error())
		return
	}
	http.DefaultClient.Do(req)
}

func onTimeout(t time.Duration, f func()) chan struct{} {
	cancel := make(chan struct{})
	go func() {
		select {
		case <-time.After(t):
			f()
		case <-cancel:
		}
	}()
	return cancel
}

func handlers() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc(errPath, errFunc)
	mux.HandleFunc("/status", status)
	mux.HandleFunc("/wd/hub/session", create)
	mux.Handle("/wd/hub/session/", &httputil.ReverseProxy{Director: proxy})
	return mux
}

func init() {
	flag.StringVar(&listen, "listen", ":4444", "network address to accept connections")
	flag.DurationVar(&timeout, "timeout", 60*time.Second, "session idle timeout in seconds")
	flag.Var(&nodes, "nodes", "underlying driver nodes (required)")
	flag.Parse()
}

func queue(nodes stringSlice) {
	log.Println("Initializing nodes with", nodes)
	hosts = make(chan string, len(nodes))
	for _, h := range nodes {
		hosts <- h
	}
}

func main() {
	if len(nodes) == 0 {
		flag.Usage()
		os.Exit(1)
	}
	queue(nodes)
	log.Printf("Listening on %s\n", listen)
	log.Fatal(http.ListenAndServe(listen, handlers()))
}
