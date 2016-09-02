package service

import (
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
)

type HttpTest struct {
	Handler http.Handler
	Action  func(s *httptest.Server)
	Cancel  chan bool
}

func (m *HttpTest) StartWithCancel() (*url.URL, func(), error) {
	log.Println("Starting HttpTest Service...")
	s := httptest.NewServer(m.Handler)
	u, err := url.Parse(s.URL)
	if err != nil {
		log.Println("Failed to start HttpTest Service...")
		return nil, nil, err
	}
	log.Println("HttpTest Service started...")
	if m.Action != nil {
		m.Action(s)
	}
	return u, func() {
		log.Println("Stopping HttpTest Service...")
		s.Close()
		log.Println("HttpTest Service stopped...")
		if m.Cancel != nil {
			go func() {
				m.Cancel <- true
			}()
		}
	}, nil
}
