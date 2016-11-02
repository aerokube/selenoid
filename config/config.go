package config

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"sync"

	"github.com/aandryashin/selenoid/session"
)

type Quota map[string]int

type Version map[string]Quota

type Browsers map[string]Version

type State struct {
	Total    int      `json:"total"`
	Used     int      `json:"used"`
	Queued   int      `json:"queued"`
	Pending  int      `json:"pending"`
	Browsers Browsers `json:"browsers"`
}

type Browser struct {
	Image interface{} `json:"image"`
	Port  string      `json:"port"`
	Path  string      `json:"path"`
}

type Versions struct {
	Default  string              `json:"default"`
	Versions map[string]*Browser `json:"versions"`
}

type Config struct {
	lock     sync.RWMutex
	File     string
	Limit    int
	Browsers map[string]*Versions
}

func New(fn string, limit int) (*Config, error) {
	config := &Config{File: fn, Limit: limit, Browsers: make(map[string]*Versions)}
	err := config.Load()
	if err != nil {
		return nil, err
	}
	return config, nil
}

func (config *Config) Load() error {
	config.lock.RLock()
	defer config.lock.RUnlock()
	f, err := ioutil.ReadFile(config.File)
	if err != nil {
		return fmt.Errorf("error reading configuration file %s: %v", f, err)
	}
	if err := json.Unmarshal(f, &config.Browsers); err != nil {
		return fmt.Errorf("error parsing configuration file %s: %v", f, err)
	}
	return nil
}

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
		if v == *version {
			return b, true
		}
	}
	return nil, false
}

func (config *Config) State(sessions *session.Map, queued, pending int) *State {
	config.lock.RLock()
	defer config.lock.RUnlock()
	state := &State{config.Limit, 0, queued, pending, make(Browsers)}
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
