package protect

import (
	"log"
	"net/http"
	"time"

	"math"
)

// Queue - struct to hold a number of sessions
type Queue struct {
	disabled bool
	limit    chan struct{}
	queued   chan struct{}
	pending  chan struct{}
	used     chan struct{}
}

// Check - if queue disabled
func (q *Queue) Check(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		select {
		case q.limit <- struct{}{}:
			<-q.limit
		default:
			if q.disabled {
				log.Println("[QUEUE_IS_FULL]")
				http.Error(w, "Queue full, see other", http.StatusSeeOther)
				return
			}
		}
		next.ServeHTTP(w, r)
	}
}

// Protect - handler to control limit of sessions
func (q *Queue) Protect(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
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
	}
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

// New - create and initialize queue
func New(size int, disabled bool) *Queue {
	return &Queue{
		disabled,
		make(chan struct{}, size),
		make(chan struct{}, math.MaxInt32),
		make(chan struct{}, math.MaxInt32),
		make(chan struct{}, math.MaxInt32),
	}
}
