package session

import (
	"net/url"
	"reflect"
	"sync"
	"time"
)

// Caps - user capabilities
type Caps struct {
	Name                  string                 `json:"browserName"`
	DeviceName            string                 `json:"deviceName"`
	Version               string                 `json:"version"`
	W3CVersion            string                 `json:"browserVersion"`
	Platform              string                 `json:"platform"`
	W3CPlatform           string                 `json:"platformName"`
	ScreenResolution      string                 `json:"screenResolution"`
	Skin                  string                 `json:"skin"`
	VNC                   bool                   `json:"enableVNC"`
	Video                 bool                   `json:"enableVideo"`
	VideoName             string                 `json:"videoName"`
	VideoScreenSize       string                 `json:"videoScreenSize"`
	VideoFrameRate        uint16                 `json:"videoFrameRate"`
	VideoCodec            string                 `json:"videoCodec"`
	LogName               string                 `json:"logName"`
	TestName              string                 `json:"name"`
	TimeZone              string                 `json:"timeZone"`
	ContainerHostname     string                 `json:"containerHostname"`
	Env                   []string               `json:"env"`
	ApplicationContainers []string               `json:"applicationContainers"`
	HostsEntries          []string               `json:"hostsEntries"`
	DNSServers            []string               `json:"dnsServers"`
	Labels                map[string]string      `json:"labels"`
	SessionTimeout        uint32                 `json:"sessionTimeout"`
	ExtensionCapabilities map[string]interface{} `json:"selenoid:options"`
}

func (c *Caps) ProcessExtensionCapabilities() {
	if c.W3CVersion != "" {
		c.Version = c.W3CVersion
	}
	if c.W3CPlatform != "" {
		c.Platform = c.W3CPlatform
	}
	if len(c.ExtensionCapabilities) > 0 {
		s := reflect.ValueOf(c).Elem()

		tagToFieldMap := make(map[string]reflect.StructField)

		for i := 0; i < s.NumField(); i++ {
			field := s.Type().Field(i)
			tag := field.Tag.Get("json")
			tagToFieldMap[tag] = field
		}

		//NOTE: entries from the first maps have less priority than then next ones
		nestedMaps := []map[string]interface{}{c.ExtensionCapabilities}
		for _, nm := range nestedMaps {
			for k, v := range nm {
				value := reflect.ValueOf(v)
				if field, ok := tagToFieldMap[k]; ok && value.Type().ConvertibleTo(field.Type) {
					s.FieldByName(field.Name).Set(value.Convert(field.Type))
				}
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
	HostPort  HostPort
	Cancel    func()
	Timeout   time.Duration
	TimeoutCh chan struct{}
	Lock      sync.Mutex
}

// HostPort - hold host-port values for all forwarded ports
type HostPort struct {
	Selenium   string
	Fileserver string
	Clipboard  string
	VNC        string
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

// Len - get total number of sessions
func (m *Map) Len() int {
	m.l.RLock()
	defer m.l.RUnlock()
	return len(m.m)
}
