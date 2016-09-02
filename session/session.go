package session

import (
	"net/url"
	"sync"
)

type Session struct {
	Url     *url.URL
	Cancel  func()
	Timeout chan struct{}
}

type Map struct {
	m map[string]*Session
	l sync.RWMutex
}

func NewMap() *Map {
	return &Map{m: make(map[string]*Session)}
}

func (m *Map) Get(k string) (*Session, bool) {
	m.l.RLock()
	defer m.l.RUnlock()
	s, ok := m.m[k]
	return s, ok
}

func (m *Map) Put(k string, v *Session) {
	m.l.Lock()
	defer m.l.Unlock()
	m.m[k] = v
}

func (m *Map) Remove(k string) {
	m.l.Lock()
	defer m.l.Unlock()
	delete(m.m, k)
}

func (m *Map) Each(fn func(k string, v *Session)) {
	m.l.RLock()
	defer m.l.RUnlock()
	for k, v := range m.m {
		fn(k, v)
	}
}
