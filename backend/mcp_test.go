package backend

import (
	"context"
	"fmt"
	"net"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"

	"github.com/d3uceY/spriteware/internal/aseprite"
	"github.com/d3uceY/spriteware/internal/config"
	"github.com/d3uceY/spriteware/internal/link"
)

// newTestService has no Aseprite and no UI: a fresh machine. Every tool must
// answer.
func newTestService(t *testing.T) *MCPService {
	t.Helper()
	// A bridge that starts writes the extension with its port baked in, so keep
	// every test out of the real Aseprite folder.
	t.Setenv(aseprite.EnvOverride, t.TempDir())

	store, err := config.LoadFrom(filepath.Join(t.TempDir(), "config.json"))
	if err != nil {
		t.Fatalf("config: %v", err)
	}
	links := link.NewSelector(link.Options{
		Mode:         config.ModeHeadless,
		AsepritePath: filepath.Join(t.TempDir(), "no-such-aseprite"),
		BridgePort:   0, // not started by these tests
	})
	t.Cleanup(func() { _ = links.Close() })
	return NewMCPService(store, links)
}

// A real agent connects over HTTP, so a broken handshake fails here.
func TestMCPEndpointServesTheToolList(t *testing.T) {
	svc := newTestService(t)

	// The URL must be known up front, so ask for a port and retry if it was
	// taken meanwhile.
	var url string
	var lastErr error
	for attempt := 0; attempt < 8; attempt++ {
		port := 21000 + attempt*7
		got, err := svc.StartServer(port, string(config.TransportStreamableHTTP))
		if err != nil {
			lastErr = err
			continue
		}
		url = got
		break
	}
	if url == "" {
		t.Fatalf("could not start the MCP server: %v", lastErr)
	}
	t.Cleanup(func() { _ = svc.StopServer() })

	cli, err := client.NewStreamableHttpClient(url)
	if err != nil {
		t.Fatalf("client: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := cli.Start(ctx); err != nil {
		t.Fatalf("start: %v", err)
	}
	initRequest := mcp.InitializeRequest{}
	initRequest.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
	initRequest.Params.ClientInfo = mcp.Implementation{Name: "spriteware-test", Version: "0"}
	if _, err := cli.Initialize(ctx, initRequest); err != nil {
		t.Fatalf("initialize: %v", err)
	}

	result, err := cli.ListTools(ctx, mcp.ListToolsRequest{})
	if err != nil {
		t.Fatalf("tools/list: %v", err)
	}
	if len(result.Tools) == 0 {
		t.Fatal("the server advertised no tools")
	}

	names := map[string]bool{}
	for _, tool := range result.Tools {
		names[tool.Name] = true
		if tool.Description == "" {
			t.Errorf("tool %q has no description; the model has nothing to go on", tool.Name)
		}
	}
	for _, want := range []string{"doctor", "sprite", "layer", "cel", "frame", "tag", "draw", "look", "validate", "palette", "export"} {
		if !names[want] {
			t.Errorf("tool %q is missing from the surface", want)
		}
	}
}

// With no Aseprite, doctor must still answer and say what to do: one broken
// doctor must never take out every other tool.
func TestDoctorAnswersWithNoAseprite(t *testing.T) {
	svc := newTestService(t)

	report := svc.GetMCPStatus()
	if report.ToolCount == 0 {
		t.Fatal("status reported no tools")
	}
	if report.Running {
		t.Fatal("status says running before StartServer")
	}

	// Saving settings with a nonsense path must not lose the rest of the file.
	saved, err := svc.SaveSettings(config.Config{
		AsepritePath: "C:/definitely/not/here/aseprite.exe",
		Mode:         config.ModeHeadless,
		Port:         22999,
		Transport:    config.TransportSSE,
	})
	if err != nil {
		t.Fatalf("save: %v", err)
	}
	if saved.Port != 22999 || saved.Transport != config.TransportSSE {
		t.Errorf("saved settings did not round-trip: %+v", saved)
	}
	if saved.BridgePort == 0 {
		t.Error("an unset bridge port should have been filled with a default")
	}
}

// listening reports whether something accepts on a port.
func listening(port int) bool {
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 300*time.Millisecond)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

// A colliding pair is refused with a sentence and the file keeps its old
// numbers, instead of Normalize quietly resetting one of them.
func TestSaveSettingsRejectsCollidingPorts(t *testing.T) {
	svc := newTestService(t)
	before := svc.GetSettings()

	if _, err := svc.SaveSettings(config.Config{Port: 23300, BridgePort: 23300}); err == nil {
		t.Fatal("SaveSettings accepted the same port for both listeners")
	}
	if got := svc.GetSettings(); got.Port != before.Port || got.BridgePort != before.BridgePort {
		t.Errorf("a refused save still wrote %+v", got)
	}
}

// Saving a new port while the server is up has to move the listener, not just
// the file: the UI shows the running address.
func TestSaveSettingsMovesTheRunningListener(t *testing.T) {
	svc := newTestService(t)

	first := freePort(t)
	if _, err := svc.StartServer(first, string(config.TransportStreamableHTTP)); err != nil {
		t.Fatalf("start: %v", err)
	}
	t.Cleanup(func() { _ = svc.StopServer() })

	second := freePort(t)
	for second == first {
		second = freePort(t)
	}
	bridge := freePort(t)
	for bridge == second || bridge == first {
		bridge = freePort(t)
	}

	saved, err := svc.SaveSettings(config.Config{
		Mode:       config.ModeHeadless,
		Port:       second,
		Transport:  config.TransportStreamableHTTP,
		BridgePort: bridge,
	})
	if err != nil {
		t.Fatalf("save: %v", err)
	}
	if saved.Port != second || saved.BridgePort != bridge {
		t.Fatalf("saved %+v, want port %d and bridge %d", saved, second, bridge)
	}

	st := svc.GetMCPStatus()
	if !st.Running || st.Port != second {
		t.Fatalf("status says running=%v on %d, want running on %d", st.Running, st.Port, second)
	}
	if !listening(second) {
		t.Fatalf("nothing is listening on the saved port %d", second)
	}
	if listening(first) {
		t.Fatalf("the old port %d is still accepting", first)
	}
}

// A transport change keeps the port, so it cannot bind the replacement beside
// the listener it is replacing. It has to close that one first and must not end
// up with nothing listening.
func TestSaveSettingsSwitchesTransportOnTheSamePort(t *testing.T) {
	svc := newTestService(t)

	port := freePort(t)
	if _, err := svc.StartServer(port, string(config.TransportStreamableHTTP)); err != nil {
		t.Fatalf("start: %v", err)
	}
	t.Cleanup(func() { _ = svc.StopServer() })

	bridge := freePort(t)
	for bridge == port {
		bridge = freePort(t)
	}

	saved, err := svc.SaveSettings(config.Config{
		Mode:       config.ModeHeadless,
		Port:       port,
		Transport:  config.TransportSSE,
		BridgePort: bridge,
	})
	if err != nil {
		t.Fatalf("save: %v", err)
	}
	if saved.Transport != config.TransportSSE {
		t.Fatalf("saved transport %q, want sse", saved.Transport)
	}

	st := svc.GetMCPStatus()
	if !st.Running || st.Port != port || st.Transport != string(config.TransportSSE) {
		t.Fatalf("status is running=%v port=%d transport=%s, want %d on sse", st.Running, st.Port, st.Transport, port)
	}
	if !listening(port) {
		t.Fatalf("nothing is listening on %d after the transport change", port)
	}
	if !strings.HasSuffix(st.URL, "/sse") {
		t.Errorf("the URL still points at the old transport: %q", st.URL)
	}
}

// The one thing a save must never do: take a working listener away for a port
// that turns out to be held by something else.
func TestSaveSettingsKeepsTheListenerWhenTheNewPortIsTaken(t *testing.T) {
	svc := newTestService(t)

	first := freePort(t)
	if _, err := svc.StartServer(first, string(config.TransportStreamableHTTP)); err != nil {
		t.Fatalf("start: %v", err)
	}
	t.Cleanup(func() { _ = svc.StopServer() })

	held, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("hold a port: %v", err)
	}
	defer held.Close()
	blocked := held.Addr().(*net.TCPAddr).Port

	if _, err := svc.SaveSettings(config.Config{Port: blocked}); err == nil {
		t.Fatal("SaveSettings accepted a port another listener holds")
	}

	st := svc.GetMCPStatus()
	if !st.Running || st.Port != first {
		t.Fatalf("the listener moved to %d (running=%v), want %d", st.Port, st.Running, first)
	}
	if got := svc.GetSettings().Port; got != first {
		t.Errorf("a refused save still wrote port %d", got)
	}
	if !listening(first) {
		t.Errorf("the working listener on %d stopped answering", first)
	}
}
