package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"strings"
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
	Browsers Browsers `json:"browsers"`
}

type Browser struct {
	Image string `json:"image"`
	Port  string `json:"port"`
	Path  string `json:"path"`
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

func NewConfig(fn string, limit int) (*Config, error) {
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
		return errors.New(fmt.Sprintf("error reading configuration file %s: %v", f, err))
	}
	if err := json.Unmarshal(f, &config.Browsers); err != nil {
		return errors.New(fmt.Sprintf("error parsing configuration file %s: %v", f, err))
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

func (config *Config) State(sessions *session.Map, queued int) *State {
	config.lock.RLock()
	defer config.lock.RUnlock()
	state := &State{config.Limit, 0, queued, make(Browsers)}
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
