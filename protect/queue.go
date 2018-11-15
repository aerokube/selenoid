package protect

import (
	"log"
	"math"
	"net/http"
	"time"

	"github.com/aerokube/util"
)

// Queue - struct to hold a number of sessions
type Queue struct {
	disabled bool
	limit    chan struct{}
	queued   chan struct{}
	pending  chan struct{}
	used     chan struct{}
}

// Try - when X-Selenoid-No-Wait header is set
// reply to client immediately if queue is full
func (q *Queue) Try(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		_, noWait := r.Header["X-Selenoid-No-Wait"]
		select {
		case q.limit <- struct{}{}:
			<-q.limit
		default:
			if noWait {
				util.JsonError(w, http.StatusText(http.StatusTooManyRequests), http.StatusTooManyRequests)
				return
			}
		}
		next.ServeHTTP(w, r)
	}
}

// Check - if queue disabled
func (q *Queue) Check(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		select {
		case q.limit <- struct{}{}:
			<-q.limit
		default:
			if q.disabled {
				user, remote := util.RequestInfo(r)
				log.Printf("[-] [QUEUE_IS_FULL] [%s] [%s]", user, remote)
				util.JsonError(w, "Queue Is Full", http.StatusTooManyRequests)
				return
			}
		}
		next.ServeHTTP(w, r)
	}
}

// Protect - handler to control limit of sessions
func (q *Queue) Protect(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, remote := util.RequestInfo(r)
		log.Printf("[-] [NEW_REQUEST] [%s] [%s]", user, remote)
		s := time.Now()
		go func() {
			q.queued <- struct{}{}
		}()
		select {
		case <-r.Context().Done():
			<-q.queued
			log.Printf("[-] [CLIENT_DISCONNECTED] [%s] [%s] [%s]", user, remote, time.Since(s))
			return
		case q.limit <- struct{}{}:
			q.pending <- struct{}{}
		}
		<-q.queued
		log.Printf("[-] [NEW_REQUEST_ACCEPTED] [%s] [%s]", user, remote)
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
