package session

import (
	"github.com/imdario/mergo"
	"net/url"
	"sync"
	"time"
)

// Caps - user capabilities
type Caps struct {
	Name                  string            `json:"browserName,omitempty"`
	DeviceName            string            `json:"deviceName,omitempty"`
	Version               string            `json:"version,omitempty"`
	W3CVersion            string            `json:"browserVersion,omitempty"`
	Platform              string            `json:"platform,omitempty"`
	W3CPlatform           string            `json:"platformName,omitempty"`
	ScreenResolution      string            `json:"screenResolution,omitempty"`
	Skin                  string            `json:"skin,omitempty"`
	VNC                   bool              `json:"enableVNC,omitempty"`
	Video                 bool              `json:"enableVideo,omitempty"`
	Log                   bool              `json:"enableLog,omitempty"`
	VideoName             string            `json:"videoName,omitempty"`
	VideoScreenSize       string            `json:"videoScreenSize,omitempty"`
	VideoFrameRate        uint16            `json:"videoFrameRate,omitempty"`
	VideoCodec            string            `json:"videoCodec,omitempty"`
	LogName               string            `json:"logName,omitempty"`
	TestName              string            `json:"name,omitempty"`
	TimeZone              string            `json:"timeZone,omitempty"`
	ContainerHostname     string            `json:"containerHostname,omitempty"`
	Env                   []string          `json:"env,omitempty"`
	ApplicationContainers []string          `json:"applicationContainers,omitempty"`
	AdditionalNetworks    []string          `json:"additionalNetworks,omitempty"`
	HostsEntries          []string          `json:"hostsEntries,omitempty"`
	DNSServers            []string          `json:"dnsServers,omitempty"`
	Labels                map[string]string `json:"labels,omitempty"`
	SessionTimeout        string            `json:"sessionTimeout,omitempty"`
	S3KeyPattern          string            `json:"s3KeyPattern,omitempty"`
	ExtensionCapabilities *Caps             `json:"selenoid:options,omitempty"`
}

func (c *Caps) ProcessExtensionCapabilities() {
	if c.W3CVersion != "" {
		c.Version = c.W3CVersion
	}
	if c.W3CPlatform != "" {
		c.Platform = c.W3CPlatform
	}

	if c.ExtensionCapabilities != nil {
		mergo.Merge(c, *c.ExtensionCapabilities, mergo.WithOverride) //We probably need to handle returned error

		//According to Selenium standard vendor-specific capabilities for
		//intermediary node should not be proxied to endpoint node
		c.ExtensionCapabilities = nil
	}
}

func (c *Caps) BrowserName() string {
	browserName := c.Name
	if browserName != "" {
		return browserName
	}
	return c.DeviceName
}

// Container - container information
type Container struct {
	ID        string            `json:"id"`
	IPAddress string            `json:"ip"`
	Ports     map[string]string `json:"exposedPorts,omitempty"`
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
	Started   time.Time
	Lock      sync.Mutex
}

// HostPort - hold host-port values for all forwarded ports
type HostPort struct {
	Selenium   string
	Fileserver string
	Clipboard  string
	VNC        string
	Devtools   string
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

// Metadata - session metadata saved to file
type Metadata struct {
	ID           string    `json:"id"`
	Capabilities Caps      `json:"capabilities"`
	Started      time.Time `json:"started"`
	Finished     time.Time `json:"finished"`
}
