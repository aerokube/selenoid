package handler

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"strings"
	"sync"

	"github.com/pborman/uuid"
)

func Dumper(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rdmp, _ := httputil.DumpRequest(r, true)
		log.Println(string(rdmp))
		wr := httptest.NewRecorder()
		h.ServeHTTP(wr, r)
		wdmp, _ := httputil.DumpResponse(wr.Result(), true)
		log.Println(string(wdmp))
		w.WriteHeader(wr.Result().StatusCode)
		io.Copy(w, wr.Result().Body)
	})
}

func OnlyPost(fn http.HandlerFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		fn(w, r)
	})
}

func AnyMethod(fn http.HandlerFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fn(w, r)
	})
}

func HttpResponse(msg string, status int) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, msg, status)
	})
}

func Selenium() http.Handler {
	var lock sync.RWMutex
	sessions := make(map[string]struct{})
	mux := http.NewServeMux()
	mux.HandleFunc("/session", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		u := uuid.New()
		lock.Lock()
		sessions[u] = struct{}{}
		lock.Unlock()
		json.NewEncoder(w).Encode(struct {
			S string `json:"sessionId"`
		}{u})
	})
	mux.HandleFunc("/session/", func(w http.ResponseWriter, r *http.Request) {
		u := strings.Split(r.URL.Path, "/")[2]
		lock.RLock()
		_, ok := sessions[u]
		lock.RUnlock()
		if !ok {
			http.Error(w, "Session not found", http.StatusNotFound)
			return
		}
		if r.Method != http.MethodDelete {
			return
		}
		lock.Lock()
		delete(sessions, u)
		lock.Unlock()
	})
	return mux
}

func Ok(w http.ResponseWriter, r *http.Request) {
	log.Println("registered new node...")
}

func Success(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintln(w, `{"success":true}`)
}
