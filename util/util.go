package util

import (
	"net"
	"net/http"
	"time"
)

func SecondsSince(start time.Time) float64 {
	return float64(time.Now().Sub(start).Seconds())
}

func RequestInfo(r *http.Request) (string, string) {
	user := ""
	if u, _, ok := r.BasicAuth(); ok {
		user = u
	} else {
		user = "unknown"
	}
	remote := r.Header.Get("X-Forwarded-For")
	if remote != "" {
		return user, remote
	}
	remote, _, _ = net.SplitHostPort(r.RemoteAddr)
	return user, remote
}
