// Package config loads and persists the user's known lights. Lights are keyed
// by a friendly name and identified by their stable serial number; the last
// known host/port is cached so repeat commands are fast and survive reboots,
// while discovery refreshes the host if DHCP hands out a new address.
package config

import (
	"os"
	"path/filepath"
	"time"

	"github.com/arventures/elgato/internal/elgato"
	"gopkg.in/yaml.v3"
)

// Light is a persisted light entry.
type Light struct {
	Serial string `yaml:"serial,omitempty"`
	Host   string `yaml:"host,omitempty"` // last known IP; refreshed by discovery
	Port   int    `yaml:"port,omitempty"`
}

// Config is the on-disk configuration.
type Config struct {
	// Lights maps a friendly name (e.g. "key-left") to a light.
	Lights map[string]Light `yaml:"lights,omitempty"`
	// DiscoveryTimeout is how long to browse for lights (e.g. "2s").
	DiscoveryTimeout string `yaml:"discovery_timeout,omitempty"`

	path string `yaml:"-"`
}

// Path returns the config file location, honoring XDG_CONFIG_HOME.
func Path() string {
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "elgato-config.yaml"
		}
		base = filepath.Join(home, ".config")
	}
	return filepath.Join(base, "elgato", "config.yaml")
}

// Load reads the config file. A missing file is not an error: an empty config
// is returned so first-run works with pure discovery.
func Load() (*Config, error) {
	path := Path()
	cfg := &Config{path: path, Lights: map[string]Light{}}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return nil, err
	}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, err
	}
	if cfg.Lights == nil {
		cfg.Lights = map[string]Light{}
	}
	cfg.path = path
	return cfg, nil
}

// Save writes the config file, creating the directory if needed.
func (c *Config) Save() error {
	if c.path == "" {
		c.path = Path()
	}
	if err := os.MkdirAll(filepath.Dir(c.path), 0o755); err != nil {
		return err
	}
	data, err := yaml.Marshal(c)
	if err != nil {
		return err
	}
	return os.WriteFile(c.path, data, 0o644)
}

// Timeout returns the configured discovery timeout, or fallback if unset or
// unparseable.
func (c *Config) Timeout(fallback time.Duration) time.Duration {
	if c.DiscoveryTimeout == "" {
		return fallback
	}
	d, err := time.ParseDuration(c.DiscoveryTimeout)
	if err != nil {
		return fallback
	}
	return d
}

// RememberDevices updates the host cache from freshly discovered devices,
// matching on serial number. Devices not seen before are added with an
// auto-generated friendly name. It reports whether anything changed.
func (c *Config) RememberDevices(devices []*elgato.Device) bool {
	changed := false
	for _, d := range devices {
		serial := d.Serial()
		if serial == "" {
			continue
		}
		if name := c.nameForSerial(serial); name != "" {
			l := c.Lights[name]
			if l.Host != d.Host || l.Port != d.Port {
				l.Serial, l.Host, l.Port = serial, d.Host, d.Port
				c.Lights[name] = l
				changed = true
			}
			continue
		}
		name := c.uniqueName(d.Label())
		c.Lights[name] = Light{Serial: serial, Host: d.Host, Port: d.Port}
		changed = true
	}
	return changed
}

func (c *Config) nameForSerial(serial string) string {
	for name, l := range c.Lights {
		if l.Serial == serial {
			return name
		}
	}
	return ""
}

func (c *Config) uniqueName(base string) string {
	name := slugify(base)
	if name == "" {
		name = "light"
	}
	if _, exists := c.Lights[name]; !exists {
		return name
	}
	for i := 2; ; i++ {
		candidate := name + "-" + itoa(i)
		if _, exists := c.Lights[candidate]; !exists {
			return candidate
		}
	}
}

func slugify(s string) string {
	out := make([]rune, 0, len(s))
	prevDash := false
	for _, r := range s {
		switch {
		case r >= 'A' && r <= 'Z':
			out = append(out, r+('a'-'A'))
			prevDash = false
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'):
			out = append(out, r)
			prevDash = false
		default:
			if !prevDash && len(out) > 0 {
				out = append(out, '-')
				prevDash = true
			}
		}
	}
	for len(out) > 0 && out[len(out)-1] == '-' {
		out = out[:len(out)-1]
	}
	return string(out)
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var buf [20]byte
	pos := len(buf)
	for i > 0 {
		pos--
		buf[pos] = byte('0' + i%10)
		i /= 10
	}
	return string(buf[pos:])
}
