package protect

import (
	"log"
	"net/http"
	"time"

	"github.com/aandryashin/selenoid/ensure"
	"math"
)

// Queue - struct to hold a number of sessions
type Queue struct {
	limit   chan struct{}
	queued  chan struct{}
	pending chan struct{}
	used    chan struct{}
	size    int
}

// Protect - handler to control limit of sessions
func (q *Queue) Protect(next http.HandlerFunc) http.HandlerFunc {
	return ensure.CloseNotifier(func(w http.ResponseWriter, r *http.Request) {
		log.Println("[NEW_REQUEST]")
		cn := w.(http.CloseNotifier)
		s := time.Now()
		go func() {
			q.queued <- struct{}{}
		}()
		select {
		case <-cn.CloseNotify():
			<-q.queued
			log.Printf("connection from %s closed by client after %s waiting in queue\n", r.RemoteAddr, time.Since(s))
			return
		case q.limit <- struct{}{}:
			q.pending <- struct{}{}
		}
		<-q.queued
		log.Println("[NEW_REQUEST_ACCEPTED]")
		next.ServeHTTP(w, r)
	})
}

// Used - get created sessions
func (q *Queue) Used() int {
	return len(q.used)
}

// Pending - get pending sessions
func (q *Queue) Pending() int {
	return len(q.pending)
}

// Queued - get queued sessions
func (q *Queue) Queued() int {
	return len(q.queued)
}

// Drop - session is not created
func (q *Queue) Drop() {
	<-q.limit
	<-q.pending
}

// Create - session is created
func (q *Queue) Create() {
	q.used <- <-q.pending
}

// Release - session is closed
func (q *Queue) Release() {
	<-q.limit
	<-q.used
}

// Size - limit of sessions
func (q *Queue) Size() int {
	return q.size
}

// New - create and initialize queue
func New(size int) *Queue {
	return &Queue{
		make(chan struct{}, size),
		make(chan struct{}, math.MaxUint32),
		make(chan struct{}, math.MaxUint32),
		make(chan struct{}, math.MaxUint32),
		size,
	}
}
