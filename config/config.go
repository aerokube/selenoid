package config

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"sync"

	"github.com/aandryashin/selenoid/session"
	"strings"
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
	Tmpfs map[string]string `json:"tmpfs"`
}

// Versions configuration
type Versions struct {
	Default  string              `json:"default"`
	Versions map[string]*Browser `json:"versions"`
}

// Config current configuration
type Config struct {
	Filename string
	Limit    int
	lock     sync.RWMutex
	browsers map[string]*Versions
}

// Create and init new config
func New(filename string, limit int) (*Config, error) {
	conf := &Config{Filename: filename, Limit: limit}
	err := conf.LoadNew()
	if err != nil {
		return nil, err
	}
	return conf, nil
}

// Load file with any json structure
func Load(filename string, v interface{}) error {
	buf, err := ioutil.ReadFile(filename)
	if err != nil {
		return fmt.Errorf("config: read error: %v", err)
	}
	if err := json.Unmarshal(buf, &v); err != nil {
		return fmt.Errorf("config: parse error: %v", err)
	}
	return nil
}

// Load file to new object map
func (config *Config) LoadNew() error {
	browsers := make(map[string]*Versions)
	err := Load(config.Filename, &browsers)
	if err != nil {
		return err
	}
	config.lock.Lock()
	config.browsers = browsers
	config.lock.Unlock()
	return nil
}

// Find - find concrete browser
func (config *Config) Find(name string, version *string) (*Browser, bool) {
	config.lock.RLock()
	defer config.lock.RUnlock()
	browser, ok := config.browsers[name]
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
func (config *Config) State(sessions *session.Map, queued, pending int) *State {
	config.lock.RLock()
	defer config.lock.RUnlock()
	state := &State{config.Limit, 0, queued, pending, make(Browsers)}
	for n, b := range config.browsers {
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
