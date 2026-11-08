package mcpserver

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"sync"
	"time"

	"github.com/mark3labs/mcp-go/server"

	"github.com/d3uceY/spriteware/internal/config"
	"github.com/d3uceY/spriteware/internal/tools"
)

// errServerClosed is what an mcp-go transport returns when it is shut down on
// purpose. It is not a failure and must not be reported as one.
const errServerClosed = "http: Server closed"

// transportServer is the shared lifecycle of mcp-go's two HTTP transports, so a
// caller can hold whichever one the settings asked for.
type transportServer interface {
	Start(addr string) error
	Shutdown(ctx context.Context) error
}

// EndpointPath is where clients reach each transport.
func EndpointPath(t config.Transport) string {
	if t == config.TransportSSE {
		return "/sse"
	}
	return "/mcp"
}

// Options says what a Host should serve.
type Options struct {
	Port      int
	Transport config.Transport
	Tools     *tools.Service
	Version   string
}

// State is the listener's answer, safe to read at any time. Port and Transport
// outlive a Stop: they are the last ones that ran, which is what a settings
// panel wants to show.
type State struct {
	Running   bool
	Port      int
	Transport config.Transport
	URL       string
	LastError string
}

// Host owns one HTTP listener: the loopback port it took, the transport behind
// it, and a stop that is safe to call twice. The window serves through it, so
// there is one lifecycle to get right.
type Host struct {
	mu        sync.Mutex
	srv       transportServer
	port      int
	transport config.Transport
	lastErr   string
}

// Start binds 127.0.0.1:port and serves in the background, returning the URL a
// client should use.
func (h *Host) Start(o Options) (string, error) {
	if o.Port < 1 || o.Port > 65535 {
		return "", fmt.Errorf("invalid port %d: use a value in 1..65535", o.Port)
	}
	t := o.Transport
	if t != config.TransportSSE {
		t = config.TransportStreamableHTTP
	}

	h.mu.Lock()
	running := h.srv != nil
	h.mu.Unlock()
	if running {
		return "", errors.New("an MCP server is already listening")
	}

	// Fail fast and name the problem, rather than dying in the background.
	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", o.Port))
	if err != nil {
		err = fmt.Errorf("port %d is in use: %w", o.Port, err)
		// Kept so the window can say why nothing is listening: a start at
		// launch has no caller left to report to.
		h.mu.Lock()
		h.lastErr = err.Error()
		h.mu.Unlock()
		return "", err
	}
	_ = ln.Close()

	// The streamable server serves /mcp by default, matching EndpointPath.
	//
	// No heartbeat: mcp-go raises pings as JSON-RPC requests, and a client that
	// routes only its own request ids logs every ping as unexpected.
	var srv transportServer
	if t == config.TransportSSE {
		srv = server.NewSSEServer(New(o.Tools, o.Version), server.WithKeepAliveInterval(15*time.Second))
	} else {
		srv = server.NewStreamableHTTPServer(New(o.Tools, o.Version))
	}

	h.mu.Lock()
	h.srv, h.port, h.transport, h.lastErr = srv, o.Port, t, ""
	url := h.urlLocked()
	h.mu.Unlock()

	addr := fmt.Sprintf("127.0.0.1:%d", o.Port)
	go func() {
		if err := srv.Start(addr); err != nil && err.Error() != errServerClosed {
			slog.Error("MCP server stopped", "transport", t, "err", err)
			h.mu.Lock()
			// A Stop followed by a new Start must not be clobbered by the old
			// goroutine reporting its shutdown.
			if h.srv == srv {
				h.srv, h.port, h.lastErr = nil, 0, err.Error()
			}
			h.mu.Unlock()
		}
	}()

	// Start hands the address to a goroutine, so returning here would promise a
	// URL before the socket accepts; a client connecting instantly would get a
	// refusal.
	if err := waitForPort(addr, 2*time.Second); err != nil {
		_ = h.Stop()
		return "", err
	}
	return url, nil
}

// waitForPort polls until something accepts on addr, so a successful Start means
// a client can connect now.
func waitForPort(addr string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		conn, err := net.DialTimeout("tcp", addr, 100*time.Millisecond)
		if err == nil {
			_ = conn.Close()
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("nothing is listening on %s: %w", addr, err)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// Stop shuts the transport down. Calling it when nothing runs is a no-op.
func (h *Host) Stop() error {
	h.mu.Lock()
	srv := h.srv
	h.srv, h.port = nil, 0
	h.mu.Unlock()

	if srv == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return srv.Shutdown(ctx)
}

// State reports what the listener is doing.
func (h *Host) State() State {
	h.mu.Lock()
	defer h.mu.Unlock()
	return State{
		Running:   h.srv != nil,
		Port:      h.port,
		Transport: h.transport,
		URL:       h.urlLocked(),
		LastError: h.lastErr,
	}
}

// URL is the client address, or empty when nothing listens.
func (h *Host) URL() string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.urlLocked()
}

func (h *Host) urlLocked() string {
	if h.srv == nil || h.port <= 0 {
		return ""
	}
	return fmt.Sprintf("http://127.0.0.1:%d%s", h.port, EndpointPath(h.transport))
}
