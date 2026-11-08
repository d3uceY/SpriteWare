package backend

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"sync"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/d3uceY/spriteware/internal/aseprite"
	"github.com/d3uceY/spriteware/internal/config"
	"github.com/d3uceY/spriteware/internal/link"
	"github.com/d3uceY/spriteware/internal/mcpserver"
	"github.com/d3uceY/spriteware/internal/tools"
)

// Version is reported to every client that handshakes, stamps the tool list,
// and is what the window shows. Release builds override it with
// -ldflags "-X github.com/d3uceY/spriteware/backend.Version=<tag>"; it has to
// stay a var, because a const cannot be overridden.
var Version = "0.1.0"

// MCPStatus is pushed on the mcp:status event, so the UI never polls.
type MCPStatus struct {
	Running         bool   `json:"running"`
	Port            int    `json:"port"`
	URL             string `json:"url"`
	Transport       string `json:"transport"`
	Mode            string `json:"mode"`
	BridgePort      int    `json:"bridgePort"`
	BridgeConnected bool   `json:"bridgeConnected"`
	ToolCount       int    `json:"toolCount"`
	LastError       string `json:"lastError,omitempty"`
}

// MCPService exposes the in-process MCP server and settings to the frontend,
// bound to 127.0.0.1. Method names carry a Server or Settings suffix so they
// never collide with the Wails lifecycle hooks.
type MCPService struct {
	cfg   *config.Store
	links *link.Selector
	tools *tools.Service

	mu sync.Mutex
	// host is replaced, not reconfigured, when the port moves: a listener
	// cannot change address in place, and the old one has to keep serving until
	// the new one has bound.
	host   *mcpserver.Host
	bridge *link.Bridge

	// candidate is a detected Aseprite the user has not agreed to yet.
	candidate string
	searched  []string
	searching bool

	app *application.App
}

// NewMCPService creates the service bound to the shared config and links.
func NewMCPService(cfg *config.Store, links *link.Selector) *MCPService {
	s := &MCPService{
		cfg:   cfg,
		links: links,
		host:  &mcpserver.Host{},
		tools: tools.New(cfg, links),
	}
	// The bridge lives here, so doctor asks this service.
	s.tools.SetBridgeReporter(func() (link.BridgeState, bool) {
		s.mu.Lock()
		b := s.bridge
		s.mu.Unlock()
		if b == nil {
			return link.BridgeState{}, false
		}
		return b.State(), true
	})
	return s
}

// ServiceStartup runs once the application exists, so the service can emit
// events without a global set at construction time.
func (s *MCPService) ServiceStartup(_ context.Context, _ application.ServiceOptions) error {
	s.mu.Lock()
	s.app = application.Get()
	s.mu.Unlock()

	// Look for Aseprite now so the first screen has an answer; it stays a
	// suggestion until the user agrees.
	s.startupDetection()
	return nil
}

// ServiceShutdown releases the socket and the bridge when the window closes.
func (s *MCPService) ServiceShutdown() error {
	_ = s.StopServer()
	return nil
}

// StartServer begins an MCP server on 127.0.0.1:port and returns the URL.
//
// Starting a running server is a no-op returning the current URL, unless the
// transport differs.
func (s *MCPService) StartServer(port int, transport string) (string, error) {
	if port < 1 || port > 65535 {
		return "", fmt.Errorf("invalid port %d: use a value in 1..65535", port)
	}
	t := config.Transport(transport)
	if t != config.TransportSSE {
		t = config.TransportStreamableHTTP
	}

	if state := s.hostRef().State(); state.Running {
		if state.Transport != t {
			return "", fmt.Errorf("MCP server is already running on the %s transport; stop it first", state.Transport)
		}
		if state.Port != port {
			return "", fmt.Errorf("MCP server is already running on port %d; stop it before moving to %d", state.Port, port)
		}
		return state.URL, nil
	}

	// Live mode needs the bridge before the first call.
	if err := s.StartBridge(); err != nil {
		slog.Warn("bridge did not start", "err", err)
	}

	url, err := s.hostRef().Start(mcpserver.Options{
		Port:      port,
		Transport: t,
		Tools:     s.tools,
		Version:   Version,
	})
	if err != nil {
		s.pushStatus()
		return "", err
	}

	// Persist what worked, so the next launch does not have to guess.
	if _, saveErr := s.cfg.Update(func(c *config.Config) {
		c.Port = port
		c.Transport = t
	}); saveErr != nil {
		slog.Warn("could not save the port and transport", "err", saveErr)
	}

	s.pushStatus()
	return url, nil
}

// StopServer shuts down the running transport and the bridge.
func (s *MCPService) StopServer() error {
	if err := s.hostRef().Stop(); err != nil {
		slog.Error("MCP shutdown error", "err", err)
	}
	if err := s.StopBridge(); err != nil {
		slog.Warn("bridge shutdown error", "err", err)
	}
	s.pushStatus()
	return nil
}

// StartBridge listens for the Aseprite extension and MCP clients. A failure is
// reported, not fatal: a taken port means another copy's bridge is up.
func (s *MCPService) StartBridge() error {
	s.mu.Lock()
	if s.bridge != nil {
		s.mu.Unlock()
		return nil
	}
	port := s.cfg.Get().BridgePort
	b := link.NewBridge()
	// Aseprite connecting or disconnecting cannot come from a button press, so
	// the bridge reports it.
	b.OnStateChange(func(link.BridgeState) { s.pushStatus() })
	s.bridge = b
	s.mu.Unlock()

	if err := b.Start(port); err != nil {
		s.mu.Lock()
		s.bridge = nil
		s.mu.Unlock()
		return err
	}

	// The port is baked into the installed script, so both are written together:
	// a moved port without the script goes quiet with no error.
	if result, err := aseprite.Install(port); err != nil {
		slog.Warn("could not install the Aseprite extension", "err", err)
	} else {
		slog.Info("Aseprite extension ready", "dir", result.Dir, "wrote", result.Wrote, "reason", result.Reason)
	}
	return nil
}

// StopBridge stops the bridge if this process owns one.
func (s *MCPService) StopBridge() error {
	s.mu.Lock()
	b := s.bridge
	s.bridge = nil
	s.mu.Unlock()
	if b == nil {
		return nil
	}
	return b.Close()
}

// GetMCPStatus returns the current server state.
func (s *MCPService) GetMCPStatus() MCPStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.statusLocked()
}

// hostRef is the current listener. It is read once and used without the lock, so
// a swap cannot leave a caller holding a field that moved under it.
func (s *MCPService) hostRef() *mcpserver.Host {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.host
}

func (s *MCPService) statusLocked() MCPStatus {
	cfg := s.cfg.Get()
	state := s.host.State()
	st := MCPStatus{
		Running:    state.Running,
		Port:       state.Port,
		URL:        state.URL,
		Transport:  string(state.Transport),
		Mode:       string(s.links.Mode()),
		BridgePort: cfg.BridgePort,
		ToolCount:  mcpserver.ToolCount(),
		LastError:  state.LastError,
	}
	// Stopped there is no live port; the configured one is the number the user
	// would type. The transport falls back the same way: a start that failed has
	// no listener to report, and the card must not claim streamable-http while
	// the file says sse.
	if st.Port <= 0 {
		st.Port = cfg.Port
	}
	if st.Transport == "" {
		st.Transport = string(cfg.Transport)
	}
	if s.bridge != nil {
		st.BridgeConnected = s.bridge.State().Connected
	}
	return st
}

// SwitchMode changes the connection and remembers it, so a restart opens in the
// chosen mode. Both links stay alive.
func (s *MCPService) SwitchMode(mode string) MCPStatus {
	next := config.Config{Mode: config.Mode(mode)}.Normalize().Mode
	s.links.Use(next)

	if _, err := s.cfg.Update(func(c *config.Config) { c.Mode = next }); err != nil {
		slog.Warn("could not save the mode", "err", err)
	}

	// Live without a listener fails every call, so selecting it brings the
	// bridge up.
	if next == config.ModeLive {
		if err := s.StartBridge(); err != nil {
			slog.Warn("bridge did not start", "err", err)
		}
	}

	s.pushStatus()
	return s.GetMCPStatus()
}

// GetSettings returns the current configuration.
func (s *MCPService) GetSettings() config.Config { return s.cfg.Get() }

// SaveSettings validates, persists, then applies what can change while running.
func (s *MCPService) SaveSettings(next config.Config) (config.Config, error) {
	previous := s.cfg.Get()

	// Validate what was typed, then work with what would be written: an empty
	// transport or an unset port is legal and takes its default.
	if err := next.Validate(); err != nil {
		return previous, err
	}
	effective := next.Normalize()

	state := s.hostRef().State()
	moves := state.Running && (effective.Port != state.Port || effective.Transport != state.Transport)
	samePort := state.Running && effective.Port == state.Port

	// Bind the replacement before anything is written. A port that turns out to
	// be taken then costs nothing: the running listener is untouched.
	var replacement *mcpserver.Host
	if moves {
		h := &mcpserver.Host{}
		start := func(transport config.Transport) error {
			_, err := h.Start(mcpserver.Options{
				Port:      effective.Port,
				Transport: transport,
				Tools:     s.tools,
				Version:   Version,
			})
			return err
		}

		err := start(effective.Transport)
		if err != nil && samePort {
			// A transport change cannot bind beside the listener it replaces:
			// close that one, then take the port again.
			_ = s.hostRef().Stop()
			err = start(effective.Transport)
			if err != nil {
				// Nothing is listening now, so put the old transport back rather
				// than leave the app with neither.
				_ = start(state.Transport)
			}
		}
		if err != nil {
			s.pushStatus()
			return previous, err
		}
		replacement = h
	}

	// A stopped server cannot discover a taken port, so ask the OS now rather
	// than let the next Start fail with the same sentence.
	if !state.Running && effective.Port != previous.Port {
		if err := probePort(effective.Port); err != nil {
			return previous, err
		}
	}

	saved, err := s.cfg.Save(effective)
	if err != nil {
		if replacement != nil {
			_ = replacement.Stop()
		}
		return saved, err
	}
	s.links.Use(saved.Mode)

	if replacement != nil {
		s.mu.Lock()
		old := s.host
		s.host = replacement
		s.mu.Unlock()
		if err := old.Stop(); err != nil {
			slog.Warn("the previous MCP listener did not close cleanly", "err", err)
		}
	}

	// A listener cannot move ports and the extension has the old one baked in,
	// so both change together.
	if saved.BridgePort != previous.BridgePort {
		if err := s.StopBridge(); err != nil {
			slog.Warn("could not stop the bridge to move its port", "err", err)
		}
		s.links.SetBridgePort(saved.BridgePort)
	}
	// Live without a listener fails every call, so bring the bridge up.
	if saved.Mode == config.ModeLive || saved.BridgePort != previous.BridgePort {
		if err := s.StartBridge(); err != nil {
			slog.Warn("bridge did not start", "err", err)
		}
	}

	s.pushStatus()
	return saved, nil
}

// probePort asks the OS whether a loopback port is free. The message matches
// Host.Start, so a save and a failed start explain a taken port the same way.
func probePort(port int) error {
	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return fmt.Errorf("port %d is in use: %w", port, err)
	}
	return ln.Close()
}

// pushStatus emits the current state so the panel stays live without polling.
func (s *MCPService) pushStatus() {
	s.mu.Lock()
	app := s.app
	st := s.statusLocked()
	s.mu.Unlock()

	if app == nil {
		return
	}
	// A missing UI is normal; never let event delivery take the service down.
	defer func() { _ = recover() }()
	app.Event.Emit("mcp:status", st)
}
