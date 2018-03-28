package session

import (
	"net/url"
	"reflect"
	"sync"
)

// Caps - user capabilities
type Caps struct {
	Name                  string                 `json:"browserName"`
	Version               string                 `json:"version"`
	ScreenResolution      string                 `json:"screenResolution"`
	VNC                   bool                   `json:"enableVNC"`
	Video                 bool                   `json:"enableVideo"`
	VideoName             string                 `json:"videoName"`
	VideoScreenSize       string                 `json:"videoScreenSize"`
	VideoFrameRate        uint16                 `json:"videoFrameRate"`
	TestName              string                 `json:"name"`
	TimeZone              string                 `json:"timeZone"`
	ContainerHostname     string                 `json:"containerHostname"`
	ApplicationContainers string                 `json:"applicationContainers"`
	HostsEntries          string                 `json:"hostsEntries"`
	Labels                string                 `json:"labels"`
	ExtensionCapabilities map[string]interface{} `json:"selenoid:options"`
}

func (c *Caps) ProcessExtensionCapabilities() {
	if len(c.ExtensionCapabilities) > 0 {
		s := reflect.ValueOf(c).Elem()

		tagToFieldMap := make(map[string]reflect.StructField)

		for i := 0; i < s.NumField(); i++ {
			field := s.Type().Field(i)
			tag := field.Tag.Get("json")
			tagToFieldMap[tag] = field
		}

		for k, v := range c.ExtensionCapabilities {
			value := reflect.ValueOf(v)
			if field, ok := tagToFieldMap[k]; ok && value.Type().ConvertibleTo(field.Type) {
				s.FieldByName(field.Name).Set(value.Convert(field.Type))
			}
		}
	}
}

// Container - container information
type Container struct {
	ID        string `json:"id"`
	IPAddress string `json:"ip"`
}

// Session - holds session info
type Session struct {
	Quota     string
	Caps      Caps
	URL       *url.URL
	Container *Container
	VNC       string
	Cancel    func()
	Timeout   chan struct{}
	Lock      sync.Mutex
}

// Map - session uuid to sessions mapping
type Map struct {
	m map[string]*Session
	l sync.RWMutex
}

// NewMap - create session map
func NewMap() *Map {
	return &Map{m: make(map[string]*Session)}
}

// Get - synchronous get session
func (m *Map) Get(k string) (*Session, bool) {
	m.l.RLock()
	defer m.l.RUnlock()
	s, ok := m.m[k]
	return s, ok
}

// Put - synchronous put session
func (m *Map) Put(k string, v *Session) {
	m.l.Lock()
	defer m.l.Unlock()
	m.m[k] = v
}

// Remove - synchronous remove session
func (m *Map) Remove(k string) {
	m.l.Lock()
	defer m.l.Unlock()
	delete(m.m, k)
}

// Each - synchronous iterate through sessions
func (m *Map) Each(fn func(k string, v *Session)) {
	m.l.RLock()
	defer m.l.RUnlock()
	for k, v := range m.m {
		fn(k, v)
	}
}
