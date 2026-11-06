package backend

import (
	"context"
	"net"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
)

// Talks to a *running* SpriteWare, not a server built in the test process:
// everything else proves a package works, this proves the binary does.
//
//	$env:SPRITEWARE_E2E_URL = "http://127.0.0.1:9977/mcp"
//	$env:SPRITEWARE_E2E_BRIDGE = "9978"
//	go test ./backend -run TestE2E -v
func e2eURL(t *testing.T) string {
	t.Helper()
	url := os.Getenv("SPRITEWARE_E2E_URL")
	if url == "" {
		t.Skip("set SPRITEWARE_E2E_URL to a running SpriteWare endpoint")
	}
	return url
}

func e2eClient(t *testing.T) *client.Client {
	t.Helper()
	c, err := client.NewStreamableHttpClient(e2eURL(t))
	if err != nil {
		t.Fatalf("client: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	initReq := mcp.InitializeRequest{}
	initReq.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
	initReq.Params.ClientInfo = mcp.Implementation{Name: "spriteware-e2e", Version: "0"}
	if _, err := c.Initialize(ctx, initReq); err != nil {
		t.Fatalf("initialize: %v", err)
	}
	return c
}

// The tool surface a real client sees, including everything added this session.
func TestE2EToolSurface(t *testing.T) {
	c := e2eClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	list, err := c.ListTools(ctx, mcp.ListToolsRequest{})
	if err != nil {
		t.Fatalf("tools/list: %v", err)
	}

	have := map[string]bool{}
	for _, tool := range list.Tools {
		have[tool.Name] = true
	}
	for _, want := range []string{"doctor", "sprite", "draw", "transform", "look", "cel", "export", "validate", "palette", "layer", "frame", "tag"} {
		if !have[want] {
			t.Errorf("the running server does not publish %q", want)
		}
	}
}

// A client that never installs the skill folder still has to see the resources.
func TestE2EResourcesAreServed(t *testing.T) {
	c := e2eClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	list, err := c.ListResources(ctx, mcp.ListResourcesRequest{})
	if err != nil {
		t.Fatalf("resources/list: %v", err)
	}

	uris := map[string]bool{}
	for _, r := range list.Resources {
		uris[r.URI] = true
	}
	for _, want := range []string{"skill://spriteware", "skill://draw", "rules://craft"} {
		if !uris[want] {
			t.Errorf("the running server does not serve %s", want)
		}
	}

	read, err := c.ReadResource(ctx, mcp.ReadResourceRequest{
		Params: mcp.ReadResourceParams{URI: "skill://spriteware"},
	})
	if err != nil {
		t.Fatalf("resources/read: %v", err)
	}
	if len(read.Contents) == 0 {
		t.Fatal("the resource came back empty")
	}
}

// doctor must answer even with no Aseprite, and must say what to do next.
func TestE2EDoctorAlwaysAnswers(t *testing.T) {
	c := e2eClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	result, err := c.CallTool(ctx, mcp.CallToolRequest{
		Params: mcp.CallToolParams{Name: "doctor"},
	})
	if err != nil {
		t.Fatalf("doctor: %v", err)
	}
	if result.IsError {
		t.Fatal("doctor returned a tool error instead of a report")
	}

	text := toolText(result)
	if !strings.Contains(text, "directive") && !strings.Contains(text, "Ready") &&
		!strings.Contains(text, "Set asepritePath") && !strings.Contains(text, "not available") {
		t.Errorf("doctor's answer says nothing actionable: %.300s", text)
	}
}

// No unit test can prove the bridge: dial the running app as the extension
// would, then ask doctor whether it noticed.
func TestE2EBridgeAcceptsAnExtension(t *testing.T) {
	port := os.Getenv("SPRITEWARE_E2E_BRIDGE")
	if port == "" {
		t.Skip("set SPRITEWARE_E2E_BRIDGE to the running app's bridge port")
	}
	c := e2eClient(t)

	conn, err := net.DialTimeout("tcp", "127.0.0.1:"+port, 5*time.Second)
	if err != nil {
		t.Fatalf("dial the bridge: %v", err)
	}
	defer conn.Close()

	if _, err := conn.Write([]byte("{\"role\":\"extension\"}\n")); err != nil {
		t.Fatalf("role: %v", err)
	}
	// Say hello, exactly as the Lua does, so the bridge records a version.
	hello := `{"type":"hello","protocol":1,"extensionVersion":"0.1.0","asepriteVersion":"test","features":["dump"]}`
	if _, err := conn.Write([]byte("\x1eSW1 " + hello + "\n")); err != nil {
		t.Fatalf("hello: %v", err)
	}

	// The bridge broadcasts state to clients; doctor reads it.
	deadline := time.Now().Add(10 * time.Second)
	for {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		result, err := c.CallTool(ctx, mcp.CallToolRequest{
			Params: mcp.CallToolParams{Name: "doctor"},
		})
		cancel()
		if err != nil {
			t.Fatalf("doctor: %v", err)
		}
		if strings.Contains(toolText(result), "attached to the bridge") {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("the bridge never reported the extension as connected: %.400s", toolText(result))
		}
		time.Sleep(200 * time.Millisecond)
	}
}

func toolText(result *mcp.CallToolResult) string {
	var sb strings.Builder
	for _, content := range result.Content {
		if text, ok := content.(mcp.TextContent); ok {
			sb.WriteString(text.Text)
		}
	}
	return sb.String()
}
