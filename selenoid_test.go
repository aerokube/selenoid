package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	. "github.com/aandryashin/matchers"
	. "github.com/aandryashin/matchers/httpresp"
)

var (
	srv *httptest.Server
)

func hostport(u string) string {
	uri, _ := url.Parse(u)
	return uri.Host
}

func root(p string) string {
	return fmt.Sprintf("%s%s", srv.URL, p)
}

func peek() (host string) {
	select {
	case host = <-hosts:
	default:
	}
	return host
}

func init() {
	srv = httptest.NewServer(handlers())
	listen = hostport(srv.URL)
}

func TestNewSessionWithGet(t *testing.T) {
	rsp, err := http.Get(root("/wd/hub/session"))
	AssertThat(t, err, Is{nil})
	AssertThat(t, rsp, Code{http.StatusMethodNotAllowed})
}

func TestNewSessionNotFound(t *testing.T) {
	queue(stringSlice{":4444"})
	rsp, err := http.Post(root("/wd/hub/session/123"), "", nil)
	AssertThat(t, err, Is{nil})
	AssertThat(t, rsp, Code{http.StatusNotFound})
}

func TestNewSessionHostDown(t *testing.T) {
	queue(stringSlice{":4444"})
	rsp, err := http.Post(root("/wd/hub/session"), "", nil)
	AssertThat(t, err, Is{nil})
	AssertThat(t, rsp, Code{http.StatusInternalServerError})

	host := peek()
	AssertThat(t, host, EqualTo{":4444"})
}

func TestNewSessionBadGateway(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/session", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "", http.StatusBadRequest)
	})
	driver := httptest.NewServer(mux)
	defer driver.Close()

	queue(stringSlice{hostport(driver.URL)})
	rsp, err := http.Post(root("/wd/hub/session"), "", nil)
	AssertThat(t, err, Is{nil})
	AssertThat(t, rsp, Code{http.StatusBadRequest})

	host := peek()
	AssertThat(t, host, EqualTo{hostport(driver.URL)})
}

func TestNewSessionCreated(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/session", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"sessionId":"123"}`))
	})
	driver := httptest.NewServer(mux)
	defer driver.Close()

	queue(stringSlice{hostport(driver.URL)})
	rsp, err := http.Post(root("/wd/hub/session"), "", nil)
	AssertThat(t, err, Is{nil})
	AssertThat(t, rsp, Code{http.StatusOK})

	host := peek()
	AssertThat(t, host, EqualTo{""})
}

func TestNewSessionTimeout(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/session", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"sessionId":"123"}`))
	})
	driver := httptest.NewServer(mux)
	defer driver.Close()

	timeout = 30 * time.Millisecond
	queue(stringSlice{hostport(driver.URL)})
	rsp, err := http.Post(root("/wd/hub/session"), "", nil)
	AssertThat(t, err, Is{nil})
	AssertThat(t, rsp, Code{http.StatusOK})

	host := peek()
	AssertThat(t, host, EqualTo{""})

	<-time.After(50 * time.Millisecond)
	host = peek()
	AssertThat(t, host, EqualTo{hostport(driver.URL)})
}

func TestProxySessionTimeout(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/session", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"sessionId":"123"}`))
	})
	mux.HandleFunc("/session/123", func(w http.ResponseWriter, r *http.Request) {
	})
	driver := httptest.NewServer(mux)
	defer driver.Close()

	timeout = 30 * time.Millisecond
	queue(stringSlice{hostport(driver.URL)})
	rsp, err := http.Post(root("/wd/hub/session"), "", nil)
	AssertThat(t, err, Is{nil})
	AssertThat(t, rsp, Code{http.StatusOK})

	host := peek()
	AssertThat(t, host, EqualTo{""})

	<-time.After(10 * time.Millisecond)
	rsp, err = http.Post(root("/wd/hub/session/123"), "", nil)
	AssertThat(t, err, Is{nil})
	AssertThat(t, rsp, Code{http.StatusOK})

	<-time.After(50 * time.Millisecond)
	host = peek()
	AssertThat(t, host, EqualTo{hostport(driver.URL)})
}

func TestUseSession(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/session", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"sessionId":"123"}`))
	})
	mux.HandleFunc("/session/123", func(w http.ResponseWriter, r *http.Request) {
	})
	driver := httptest.NewServer(mux)
	defer driver.Close()

	queue(stringSlice{hostport(driver.URL)})
	rsp, err := http.Post(root("/wd/hub/session"), "", nil)
	AssertThat(t, err, Is{nil})
	AssertThat(t, rsp, Code{http.StatusOK})

	rsp, err = http.Post(root("/wd/hub/session/123"), "", nil)
	AssertThat(t, err, Is{nil})
	AssertThat(t, rsp, Code{http.StatusOK})
}

func TestDeleteSession(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/session", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"sessionId":"123"}`))
	})
	mux.HandleFunc("/session/123", func(w http.ResponseWriter, r *http.Request) {
	})

	driver := httptest.NewServer(mux)
	defer driver.Close()

	queue(stringSlice{hostport(driver.URL)})
	rsp, err := http.Post(root("/wd/hub/session"), "", nil)
	AssertThat(t, err, Is{nil})
	AssertThat(t, rsp, Code{http.StatusOK})

	host := peek()
	AssertThat(t, host, EqualTo{""})

	req, err := http.NewRequest(http.MethodDelete, fmt.Sprintf("http://%s/wd/hub/session/123", listen), nil)
	http.DefaultClient.Do(req)
	AssertThat(t, err, Is{nil})
	AssertThat(t, rsp, Code{http.StatusOK})
	
	host = peek()
	AssertThat(t, host, EqualTo{hostport(driver.URL)})
}
