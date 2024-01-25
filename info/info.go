package info

import (
	"net"
	"net/http"
	"time"
)

func RequestInfo(r *http.Request) (string, string) {
	const unknownUser = "unknown"
	user := ""
	if u, _, ok := r.BasicAuth(); ok {
		user = u
	} else {
		user = unknownUser
	}
	remote := r.Header.Get("X-Forwarded-For")
	if remote != "" {
		return user, remote
	}
	remote, _, _ = net.SplitHostPort(r.RemoteAddr)
	return user, remote
}

func SecondsSince(start time.Time) float64 {
	return time.Now().Sub(start).Seconds()
}
