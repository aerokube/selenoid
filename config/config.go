package config

import (
	"log"

	"encoding/json"
	"fmt"
	"io/ioutil"
	"strings"
	"sync"

	"github.com/aerokube/selenoid/session"
	"github.com/docker/docker/api/types/container"
)

// Quota - number of sessions for quota user
type Quota map[string]int

// Version - browser version for quota
type Version map[string]Quota

// Browsers - browser names for versions
type Browsers map[string]Version

// State - current state
type State struct {
	Total    int      `json:"total"`
	Used     int      `json:"used"`
	Queued   int      `json:"queued"`
	Pending  int      `json:"pending"`
	Browsers Browsers `json:"browsers"`
}

// Browser configuration
type Browser struct {
	Image interface{}       `json:"image"`
	Port  string            `json:"port"`
	Path  string            `json:"path"`
	Tmpfs map[string]string `json:"tmpfs,omitempty"`
}

// Versions configuration
type Versions struct {
	Default  string              `json:"default"`
	Versions map[string]*Browser `json:"versions"`
}

// Config current configuration
type Config struct {
	lock          sync.RWMutex
	Browsers      map[string]Versions
	ContainerLogs *container.LogConfig
}

// NewConfig creates new config
func NewConfig() *Config {
	return &Config{Browsers: make(map[string]Versions), ContainerLogs: new(container.LogConfig)}
}

func loadJSON(filename string, v interface{}) error {
	buf, err := ioutil.ReadFile(filename)
	if err != nil {
		return fmt.Errorf("read error: %v", err)
	}
	if err := json.Unmarshal(buf, v); err != nil {
		return fmt.Errorf("parse error: %v", err)
	}
	return nil
}

// Load loads config from file
func (config *Config) Load(browsers, containerLogs string) error {
	log.Println("Loading configuration files...")
	br := make(map[string]Versions)
	err := loadJSON(browsers, &br)
	if err != nil {
		return fmt.Errorf("browsers config: %v", err)
	}
	log.Printf("Loaded configuration from [%s]\n", browsers)
	var cl *container.LogConfig
	err = loadJSON(containerLogs, &cl)
	if err != nil {
		log.Printf("Using default containers log configuration because of: %v\n", err)
		cl = &container.LogConfig{}
	} else {
		log.Printf("Loaded configuration from [%s]\n", containerLogs)
	}
	config.lock.Lock()
	defer config.lock.Unlock()
	config.Browsers, config.ContainerLogs = br, cl
	return nil
}

// Find - find concrete browser
func (config *Config) Find(name string, version *string) (*Browser, bool) {
	config.lock.RLock()
	defer config.lock.RUnlock()
	browser, ok := config.Browsers[name]
	if !ok {
		return nil, false
	}
	if *version == "" {
		log.Println("Using default version:", browser.Default)
		*version = browser.Default
		if *version == "" {
			return nil, false
		}
	}
	for v, b := range browser.Versions {
		if strings.HasPrefix(v, *version) {
			*version = v
			return b, true
		}
	}
	return nil, false
}

// State - get current state
func (config *Config) State(sessions *session.Map, limit, queued, pending int) *State {
	config.lock.RLock()
	defer config.lock.RUnlock()
	state := &State{limit, 0, queued, pending, make(Browsers)}
	for n, b := range config.Browsers {
		state.Browsers[n] = make(Version)
		for v := range b.Versions {
			state.Browsers[n][v] = make(Quota)
		}
	}
	sessions.Each(func(id string, session *session.Session) {
		state.Used++
		_, ok := state.Browsers[session.Browser]
		if !ok {
			state.Browsers[session.Browser] = make(Version)
		}
		_, ok = state.Browsers[session.Browser][session.Version]
		if !ok {
			state.Browsers[session.Browser][session.Version] = make(Quota)
		}
		_, ok = state.Browsers[session.Browser][session.Version][session.Quota]
		if !ok {
			state.Browsers[session.Browser][session.Version][session.Quota] = 0
		}
		state.Browsers[session.Browser][session.Version][session.Quota]++
	})
	return state
}
