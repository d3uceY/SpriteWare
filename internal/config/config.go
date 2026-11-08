// Package config stores the few settings a user might want to change, in one
// JSON file under the OS's per-user config directory.
//
// Everything here has a working default, and the file is written on first run
// only.
package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
)

// Transport is the MCP HTTP transport. Both are implemented by mark3labs/mcp-go.
type Transport string

const (
	// TransportStreamableHTTP is the modern single-endpoint transport.
	TransportStreamableHTTP Transport = "streamable-http"
	// TransportSSE is the older two-endpoint transport, kept for older clients.
	TransportSSE Transport = "sse"
)

// Mode selects which Aseprite the tools talk to.
type Mode string

const (
	// ModeHeadless runs one background Aseprite. Works with no window open.
	ModeHeadless Mode = "headless"
	// ModeLive drives the Aseprite window the user already has open.
	ModeLive Mode = "live"
)

// DefaultPort is high on purpose, clear of the ports other Aseprite tooling
// listens on.
const DefaultPort = 9933

// DefaultBridgePort is where the live bridge listens for the extension.
const DefaultBridgePort = 9934

// Config is the whole on-disk configuration. Keep it short: each field is a
// decision the user can get wrong.
type Config struct {
	// AsepritePath is the executable to run. Empty means "find it".
	AsepritePath string `json:"asepritePath,omitempty"`
	// AsepritePathConfirmed records that the user approved the detected path.
	// Until then it is only a suggestion.
	AsepritePathConfirmed bool `json:"asepritePathConfirmed,omitempty"`
	// Mode is the default connection: "headless" or "live".
	Mode Mode `json:"mode,omitempty"`
	// Port is the MCP HTTP port, bound to 127.0.0.1 only.
	Port int `json:"port,omitempty"`
	// Transport is "streamable-http" (default) or "sse".
	Transport Transport `json:"transport,omitempty"`
	// BridgePort is where the Aseprite extension dials in, live mode only.
	BridgePort int `json:"bridgePort,omitempty"`
	// ProjectPath is the folder "This project" means. Empty means the launch
	// folder, which from a shortcut is the install directory or System32.
	ProjectPath string `json:"projectPath,omitempty"`
	// StartMCPOnLaunch opens the MCP endpoint as soon as the app starts.
	StartMCPOnLaunch bool `json:"startMcpOnLaunch,omitempty"`
	// AllowLua unlocks the run-any-code escape hatch. Off on purpose.
	AllowLua bool `json:"allowLua,omitempty"`
}

// Default returns the configuration a new install uses.
func Default() Config {
	return Config{
		Mode:             ModeHeadless,
		Port:             DefaultPort,
		Transport:        TransportStreamableHTTP,
		BridgePort:       DefaultBridgePort,
		StartMCPOnLaunch: true,
	}
}

// Normalize replaces anything missing or invalid with a default, so a
// hand-edited file cannot break the runtime.
func (c Config) Normalize() Config {
	d := Default()
	c.AsepritePath = strings.TrimSpace(c.AsepritePath)

	switch c.Mode {
	case ModeLive, ModeHeadless:
	default:
		c.Mode = d.Mode
	}

	switch c.Transport {
	case TransportSSE, TransportStreamableHTTP:
	default:
		c.Transport = d.Transport
	}

	if c.Port < 1 || c.Port > 65535 {
		c.Port = d.Port
	}
	if c.BridgePort < 1 || c.BridgePort > 65535 {
		c.BridgePort = d.BridgePort
	}
	// Two listeners on one port would fail with a message naming the wrong
	// subsystem.
	if c.BridgePort == c.Port {
		c.BridgePort = d.BridgePort
		// The default bridge port is the one value that cannot repair a
		// collision with itself: an MCP port of 9934 used to leave both
		// listeners on 9934.
		if c.BridgePort == c.Port {
			c.BridgePort = nextPort(c.Port)
		}
	}
	return c
}

// nextPort returns a port one step from p that is still usable.
func nextPort(p int) int {
	if p < 65535 {
		return p + 1
	}
	return p - 1
}

// Validate reports what Normalize would quietly repair.
//
// The settings screen calls this before saving: a typo should come back as a
// sentence, not as a value the app silently replaced with its default. A zero
// port means "unset", which Normalize fills in, so it is not an error here.
func (c Config) Validate() error {
	switch c.Mode {
	case "", ModeHeadless, ModeLive:
	default:
		return fmt.Errorf("mode %q is not one of %q or %q", c.Mode, ModeHeadless, ModeLive)
	}

	switch c.Transport {
	case "", TransportStreamableHTTP, TransportSSE:
	default:
		return fmt.Errorf("transport %q is not one of %q or %q", c.Transport, TransportStreamableHTTP, TransportSSE)
	}

	for _, field := range []struct {
		name string
		port int
	}{
		{"MCP port", c.Port},
		{"bridge port", c.BridgePort},
	} {
		if field.port != 0 && (field.port < 1 || field.port > 65535) {
			return fmt.Errorf("the %s has to be 1..65535, not %d", field.name, field.port)
		}
	}

	// Compare what would actually be bound, not what was typed.
	port, bridge := c.Port, c.BridgePort
	if port == 0 {
		port = Default().Port
	}
	if bridge == 0 {
		bridge = Default().BridgePort
	}
	if port == bridge {
		return fmt.Errorf("the MCP port and the bridge port cannot both be %d", port)
	}
	return nil
}

// Path returns the config file location for this OS.
func Path() (string, error) {
	// An explicit path lets a portable install or a second copy use its own
	// settings.
	if custom := os.Getenv("SPRITEWARE_CONFIG"); custom != "" {
		return custom, nil
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("locate user config directory: %w", err)
	}
	return filepath.Join(dir, "SpriteWare", "config.json"), nil
}

// Store is a concurrency-safe holder for the configuration.
type Store struct {
	mu   sync.RWMutex
	path string
	cfg  Config
}

// Load reads the config file, creating it with defaults when it is absent.
//
// A corrupt file is moved aside to config.bad.json and replaced: an unreadable
// config must never stop the app starting.
func Load() (*Store, error) {
	path, err := Path()
	if err != nil {
		return nil, err
	}
	return LoadFrom(path)
}

// LoadFrom is Load with an explicit path, which is what tests use.
func LoadFrom(path string) (*Store, error) {
	s := &Store{path: path, cfg: Default()}

	raw, err := os.ReadFile(path)
	switch {
	case err == nil:
		// Notepad and PowerShell write a UTF-8 BOM, which encoding/json refuses;
		// without this the config looks corrupt and settings vanish.
		raw = bytes.TrimPrefix(raw, []byte{0xEF, 0xBB, 0xBF})
		var cfg Config
		if jsonErr := json.Unmarshal(raw, &cfg); jsonErr != nil {
			_ = os.Rename(path, strings.TrimSuffix(path, ".json")+".bad.json")
			// Moved aside *and replaced*: without the write, Load returns defaults
			// but leaves no file, and callers report a missing config.
			if err := writeFile(path, s.cfg); err != nil {
				return s, err
			}
			break
		}
		s.cfg = cfg.Normalize()
	case errors.Is(err, os.ErrNotExist):
		if err := writeFile(path, s.cfg); err != nil {
			return s, err
		}
	default:
		return s, fmt.Errorf("read config: %w", err)
	}

	return s, nil
}

// Get returns a copy of the current configuration.
func (s *Store) Get() Config {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cfg
}

// FilePath reports where this store persists.
func (s *Store) FilePath() string { return s.path }

// Save validates, stores and persists a new configuration.
func (s *Store) Save(cfg Config) (Config, error) {
	cfg = cfg.Normalize()

	s.mu.Lock()
	s.cfg = cfg
	s.mu.Unlock()

	if err := writeFile(s.path, cfg); err != nil {
		return cfg, err
	}
	return cfg, nil
}

// Update applies fn to a copy of the config and saves the result.
func (s *Store) Update(fn func(*Config)) (Config, error) {
	cfg := s.Get()
	fn(&cfg)
	return s.Save(cfg)
}

// Dir returns the directory holding the config file.
func (s *Store) Dir() string { return filepath.Dir(s.path) }

func writeFile(path string, cfg Config) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}
	raw, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("encode config: %w", err)
	}
	raw = append(raw, '\n')
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	return nil
}

// DefaultAsepriteCandidates lists the usual install locations, most specific
// first.
func DefaultAsepriteCandidates() []string {
	switch runtime.GOOS {
	case "windows":
		return []string{
			`C:\Program Files\Aseprite\aseprite.exe`,
			`C:\Program Files (x86)\Steam\steamapps\common\Aseprite\aseprite.exe`,
			`C:\Program Files\Steam\steamapps\common\Aseprite\aseprite.exe`,
		}
	case "darwin":
		return []string{
			"/Applications/Aseprite.app/Contents/MacOS/aseprite",
			filepath.Join(os.Getenv("HOME"), "Applications/Aseprite.app/Contents/MacOS/aseprite"),
		}
	default:
		return []string{
			"/usr/bin/aseprite",
			"/usr/local/bin/aseprite",
			"/var/lib/flatpak/exports/bin/com.aseprite.Aseprite",
		}
	}
}

// lookPath is exec.LookPath, held in a variable so tests can stub the PATH.
var lookPath = exec.LookPath

// SearchAseprite finds the executable without user input and reports every
// place it looked.
func SearchAseprite() (string, []string) {
	searched := make([]string, 0, len(DefaultAsepriteCandidates())+1)
	searched = append(searched, "PATH")

	if p, err := lookPath("aseprite"); err == nil {
		return p, searched
	}
	for _, candidate := range DefaultAsepriteCandidates() {
		searched = append(searched, candidate)
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate, searched
		}
	}
	return "", searched
}

// FindAseprite returns the first usable executable: the setting wins, then PATH,
// then the usual install spots.
func FindAseprite(configured string) (string, error) {
	if configured != "" {
		if info, err := os.Stat(configured); err != nil || info.IsDir() {
			return "", fmt.Errorf("aseprite not found at %q", configured)
		}
		return configured, nil
	}
	if found, _ := SearchAseprite(); found != "" {
		return found, nil
	}
	return "", errors.New("aseprite not found: set asepritePath in the config to its full path")
}
