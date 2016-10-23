package protect

import (
	"log"
	"net/http"
	"time"

	"github.com/aandryashin/selenoid/ensure"
)

type Queue struct {
	queued chan struct{}
	used   chan struct{}
	size   int
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
		case q.used <- struct{}{}:
		}
		<-q.queued
		log.Println("[NEW_REQUEST_ACCEPTED]")
		next.ServeHTTP(w, r)
	})
}

func (q *Queue) Used() int {
	return len(q.used)
}

func (q *Queue) Queued() int {
	return len(q.queued)
}

func (q *Queue) Size() int {
	return q.size
}

func (q *Queue) Release() {
	<-q.used
}

func New(size int) *Queue {
	return &Queue{make(chan struct{}, 2^64-1), make(chan struct{}, size), size}
}
