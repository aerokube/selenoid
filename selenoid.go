package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"strings"
	"time"

	"github.com/aandryashin/selenoid/handler"
	"github.com/aandryashin/selenoid/session"
)

const (
	errorPath = "/error"
)

var (
	await    chan struct{} = make(chan struct{}, 2^64-1)
	sessions *session.Map  = session.NewMap()
)

func Handler() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/session", handler.OnlyPost(create))
	mux.Handle("/session/", handler.AnyMethod(proxy))
	root := http.NewServeMux()
	root.Handle("/wd/hub/", http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			r.URL.Scheme = "http"
			r.URL.Host = localaddr(r)
			r.URL.Path = strings.TrimPrefix(r.URL.Path, "/wd/hub")
			mux.ServeHTTP(w, r)
		}))
	root.HandleFunc(errorPath, errorHandler)
	root.HandleFunc("/status", status)
	return root
}

func localaddr(r *http.Request) string {
	return r.Context().Value(http.LocalAddrContextKey).(net.Addr).String()
}

func errorHandler(w http.ResponseWriter, r *http.Request) {
	http.Error(w, `{"value" : {"message" : "Session not found"}, "status" : 13}`, http.StatusNotFound)
}

func status(w http.ResponseWriter, r *http.Request) {
	state := conf.State(sessions, len(await))
	json.NewEncoder(w).Encode(&state)
}

func create(w http.ResponseWriter, r *http.Request) {
	log.Println("[NEW_REQUEST] - Waiting for a free node")
	go func() {
		await <- struct{}{}
	}()
	queue <- struct{}{}
	<-await
	log.Println("[NEW_REQUEST_ACCEPTED]")
	quota, _, ok := r.BasicAuth()
	if !ok {
		quota = "unknown"
	}
	body, err := ioutil.ReadAll(r.Body)
	r.Body.Close()
	if err != nil {
		log.Printf("[ERROR_READING_REQUEST] [%s]\n", err.Error())
		http.Error(w, err.Error(), http.StatusBadRequest)
		<-queue
		return
	}
	var browser struct {
		Caps struct {
			Name    string `json:"browserName"`
			Version string `json:"version"`
		} `json:"desiredCapabilities"`
	}
	err = json.Unmarshal(body, &browser)
	if err != nil {
		log.Printf("[BAD_JSON_FORMAT] [%s]\n", err.Error())
		http.Error(w, err.Error(), http.StatusBadRequest)
		<-queue
		return
	}
	starter, ok := manager.Find(browser.Caps.Name, &browser.Caps.Version)
	if !ok {
		log.Printf("[ENVIRONMENT_NOT_AVAILABLE] [%s-%s]\n", browser.Caps.Name, browser.Caps.Version)
		http.Error(w, "Requested environment is not available", http.StatusBadRequest)
		<-queue
		return
	}
	u, cancel, err := starter.StartWithCancel()
	if err != nil {
		log.Printf("[SERVICE_STARTUP_FAILED] [%s]\n", err.Error())
		http.Error(w, err.Error(), http.StatusInternalServerError)
		<-queue
		return
	}
	r.URL.Host, r.URL.Path = u.Host, u.Path+r.URL.Path
	req, _ := http.NewRequest(http.MethodPost, r.URL.String(), r.Body)
	if r.ContentLength > 0 {
		req.ContentLength = r.ContentLength
	}
	req.Body = ioutil.NopCloser(bytes.NewReader(body))
	log.Printf("[SESSION_ATTEMPTED] [%s]\n", u.String())
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Printf("[SESSION_FAILED] [%s] - [%s]\n", u.String(), err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		<-queue
		cancel()
		return
	}
	defer resp.Body.Close()
	w.WriteHeader(resp.StatusCode)
	var s struct {
		Id string `json:"sessionId"`
	}
	tee := io.TeeReader(resp.Body, w)
	json.NewDecoder(tee).Decode(&s)
	if s.Id == "" {
		log.Printf("[SESSION_FAILED] Bad response from [%s] - [%v]\n", u.String(), resp.Status)
		<-queue
		cancel()
		return
	}
	sessions.Put(s.Id, &session.Session{
		quota,
		browser.Caps.Name,
		browser.Caps.Version,
		u,
		cancel,
		onTimeout(timeout, func() {
			deleteSession(localaddr(r), s.Id, cancel)
		})})
	log.Printf("[SESSION_CREATED] [%s] [%s]\n", s.Id, u)
}

func proxy(w http.ResponseWriter, r *http.Request) {
	done := make(chan struct{ fn func() })
	go func() {
		cancel := struct{ fn func() }{func() {}}
		defer func() {
			done <- cancel
		}()
		(&httputil.ReverseProxy{
			Director: func(r *http.Request) {
				id := strings.Split(r.URL.Path, "/")[2]
				sess, ok := sessions.Get(id)
				if ok {
					r.URL.Host, r.URL.Path = sess.Url.Host, sess.Url.Path+r.URL.Path
					close(sess.Timeout)
					if r.Method != http.MethodDelete {
						sess.Timeout = onTimeout(timeout, func() {
							deleteSession(localaddr(r), id, sess.Cancel)
						})
						return
					}
					cancel.fn = sess.Cancel
					sessions.Remove(id)
					<-queue
					log.Printf("[SESSION_DELETED] [%s]\n", id)
					return
				}
				r.URL.Path = errorPath
			},
		}).ServeHTTP(w, r)
	}()
	(<-done).fn()
}

func deleteSession(host, id string, cancel func()) {
	log.Printf("[SESSION_TIMED_OUT] [%s] - Deleting session\n", id)
	req, err := http.NewRequest(http.MethodDelete, fmt.Sprintf("http://%s/wd/hub/session/%s", host, id), nil)
	if err == nil {
		req.Close = true
		resp, err := http.DefaultClient.Do(req)
		if err == nil && resp.StatusCode == http.StatusOK {
			return
		}
	}
	log.Printf("[DELETE_FAILED]")
	<-queue
	cancel()
	sessions.Remove(id)
	log.Printf("[FORCED_SESSION_REMOVAL] [%s]\n", id)
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
