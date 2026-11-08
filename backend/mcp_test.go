package backend

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"

	"github.com/d3uceY/spriteware/internal/config"
	"github.com/d3uceY/spriteware/internal/link"
)

// newTestService has no Aseprite and no UI: a fresh machine. Every tool must
// answer.
func newTestService(t *testing.T) *MCPService {
	t.Helper()
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
