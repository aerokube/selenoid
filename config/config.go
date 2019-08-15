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
	"time"
)

// Session - session id and vnc flag
type Session struct {
	ID            string             `json:"id"`
	Container     string             `json:"container,omitempty"`
	ContainerInfo *session.Container `json:"containerInfo,omitempty"`
	VNC           bool               `json:"vnc"`
	Screen        string             `json:"screen"`
	Caps          session.Caps       `json:"caps"`
}

// Sessions - used count and individual sessions for quota user
type Sessions struct {
	Count    int       `json:"count"`
	Sessions []Session `json:"sessions"`
}

// Quota - list of sessions for quota user
type Quota map[string]*Sessions

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
	Image           interface{}       `json:"image"`
	Port            string            `json:"port"`
	Path            string            `json:"path"`
	Tmpfs           map[string]string `json:"tmpfs,omitempty"`
	Volumes         []string          `json:"volumes,omitempty"`
	Env             []string          `json:"env,omitempty"`
	Hosts           []string          `json:"hosts,omitempty"`
	ShmSize         int64             `json:"shmSize,omitempty"`
	Labels          map[string]string `json:"labels,omitempty"`
	Sysctl          map[string]string `json:"sysctl,omitempty"`
	Mem             string            `json:"mem,omitempty"`
	Cpu             string            `json:"cpu,omitempty"`
	PublishAllPorts bool              `json:"publishAllPorts,omitempty"`
}

// Versions configuration
type Versions struct {
	Default  string              `json:"default"`
	Versions map[string]*Browser `json:"versions"`
}

// Config current configuration
type Config struct {
	lock           sync.RWMutex
	LastReloadTime time.Time
	Browsers       map[string]Versions
	ContainerLogs  *container.LogConfig
}

// NewConfig creates new config
func NewConfig() *Config {
	return &Config{Browsers: make(map[string]Versions), ContainerLogs: new(container.LogConfig), LastReloadTime: time.Now()}
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
	log.Println("[-] [INIT] [Loading configuration files...]")
	br := make(map[string]Versions)
	err := loadJSON(browsers, &br)
	if err != nil {
		return fmt.Errorf("browsers config: %v", err)
	}
	log.Printf("[-] [INIT] [Loaded configuration from %s]", browsers)
	cl := &container.LogConfig{}
	if containerLogs != "" {
		err = loadJSON(containerLogs, cl)
		if err != nil {
			return fmt.Errorf("log config: %v", err)
		}
		log.Printf("[-] [INIT] [Loaded log configuration from %s]", containerLogs)
	}
	config.lock.Lock()
	defer config.lock.Unlock()
	config.Browsers, config.ContainerLogs = br, cl
	config.LastReloadTime = time.Now()
	return nil
}

// Find - find concrete browser
func (config *Config) Find(name string, version string) (*Browser, string, bool) {
	config.lock.RLock()
	defer config.lock.RUnlock()
	browser, ok := config.Browsers[name]
	if !ok {
		return nil, "", false
	}
	if version == "" {
		log.Printf("[-] [DEFAULT_VERSION] [Using default version: %s]", browser.Default)
		version = browser.Default
		if version == "" {
			return nil, "", false
		}
	}
	for v, b := range browser.Versions {
		if strings.HasPrefix(v, version) {
			return b, v, true
		}
	}
	return nil, version, false
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
		browserName := session.Caps.BrowserName()
		version := session.Caps.Version
		_, ok := state.Browsers[browserName]
		if !ok {
			state.Browsers[browserName] = make(Version)
		}
		_, ok = state.Browsers[browserName][version]
		if !ok {
			state.Browsers[browserName][version] = make(Quota)
		}
		v, ok := state.Browsers[browserName][version][session.Quota]
		if !ok {
			v = &Sessions{0, []Session{}}
			state.Browsers[browserName][version][session.Quota] = v
		}
		v.Count++
		vnc := false
		if session.HostPort.VNC != "" {
			vnc = true
		}
		ctr := session.Container
		sess := Session{
			ID:            id,
			ContainerInfo: ctr,
			VNC:           vnc,
			Screen:        session.Caps.ScreenResolution,
			Caps:          session.Caps,
		}
		if ctr != nil {
			sess.Container = ctr.ID
		}
		v.Sessions = append(v.Sessions, sess)
	})
	return state
}
