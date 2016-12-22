package protect

import (
	"log"
	"net/http"
	"time"

	"github.com/aandryashin/selenoid/ensure"
	"math"
)

type Queue struct {
	limit   chan struct{}
	queued  chan struct{}
	pending chan struct{}
	used    chan struct{}
	size    int
}

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

func (q *Queue) Used() int {
	return len(q.used)
}

func (q *Queue) Pending() int {
	return len(q.pending)
}

func (q *Queue) Queued() int {
	return len(q.queued)
}

func (q *Queue) Drop() {
	<-q.limit
	<-q.pending
}

func (q *Queue) Create() {
	q.used <- <-q.pending
}

func (q *Queue) Release() {
	<-q.limit
	<-q.used
}

func (q *Queue) Size() int {
	return q.size
}

func New(size int) *Queue {
	return &Queue{
		make(chan struct{}, size),
		make(chan struct{}, math.MaxUint32),
		make(chan struct{}, math.MaxUint32),
		make(chan struct{}, math.MaxUint32),
		size,
	}
}
