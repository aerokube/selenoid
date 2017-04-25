package service

import (
	"fmt"
	"log"
	"net/http"
	"net/url"
	"time"

	"github.com/aerokube/selenoid/config"
	"github.com/docker/docker/client"
)

// Starter - interface to create session with cancellation ability
type Starter interface {
	StartWithCancel() (*url.URL, func(), error)
}

// Manager - interface to choose appropriate starter
type Manager interface {
	Find(s string, v *string, sr string, requestId uint64) (Starter, bool)
}

// DefaultManager - struct for default implementation
type DefaultManager struct {
	IP     string
	Client *client.Client
	Config *config.Config
}

// Find - default implementation Manager interface
func (m *DefaultManager) Find(s string, v *string, sr string, requestId uint64) (Starter, bool) {
	log.Printf("[%d] [LOCATING_SERVICE] [%s-%s]\n", requestId, s, *v)
	service, ok := m.Config.Find(s, v)
	if !ok {
		return nil, false
	}
	switch service.Image.(type) {
	case string:
		if m.Client == nil {
			return nil, false
		}
		log.Printf("[%d] [USING_DOCKER] [%s-%s]\n", requestId, s, *v)
		return &Docker{m.IP, m.Client, service, m.Config.ContainerLogs, sr, requestId}, true
	case []interface{}:
		log.Printf("[%d] [USING_DRIVER] [%s-%s]\n", requestId, s, *v)
		return &Driver{service, requestId}, true
	}
	return nil, false
}

func wait(u string, t time.Duration) error {
	done := make(chan struct{})
	go func() {
	loop:
		for {
			select {
			case <-time.After(5 * time.Millisecond):
				r, err := http.Head(u)
				if err == nil {
					r.Body.Close()
					done <- struct{}{}
				}
			case <-done:
				break loop
			}
		}
	}()
	select {
	case <-time.After(t):
		return fmt.Errorf("%s does not respond in %v", u, t)
	case <-done:
		close(done)
	}
	return nil
}
