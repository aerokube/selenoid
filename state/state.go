package state

import (
	"encoding/json"
	"strings"
	"sync"

	"github.com/aandryashin/selenoid/docker"
)

type Quota map[string]int

type Version map[string]Quota

type Browsers map[string]Version

type Status struct {
	Limit    int      `json:"limit"`
	Browsers Browsers `json:"browsers"`
}

type qbv struct {
	quota   string
	browser string
	version string
}

type State struct {
	lock     sync.RWMutex
	status   *Status
	sessions map[string]*qbv
}

func NewState(config *docker.Config, limit int) *State {
	s := &State{status: &Status{limit, make(Browsers)}}
	for n, b := range *config {
		s.status.Browsers[n] = make(Version)
		for v := range b.Versions {
			s.status.Browsers[n][v] = make(Quota)
		}
	}
	s.sessions = make(map[string]*qbv)
	return s
}

func (s *State) NewSession(session, quota, browser, version string) {
	s.lock.Lock()
	defer s.lock.Unlock()
	for v := range s.status.Browsers[browser] {
		if strings.HasPrefix(v, version) {
			version = v
			break
		}
	}
	_, ok := s.status.Browsers[browser][version][quota]
	if !ok {
		s.status.Browsers[browser][version] = make(Quota)
	}
	s.status.Browsers[browser][version][quota]++
	s.sessions[session] = &qbv{quota, browser, version}
}

func (s *State) DeleteSession(session string) {
	s.lock.Lock()
	defer s.lock.Unlock()
	q, ok := s.sessions[session]
	if !ok {
		return
	}
	s.status.Browsers[q.browser][q.version][q.quota]--
	delete(s.sessions, session)
}

func (s *State) Status() []byte {
	b, _ := json.Marshal(s.status)
	return b
}
