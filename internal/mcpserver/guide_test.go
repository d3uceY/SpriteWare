package mcpserver

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// Every URI a guide returns has to be readable, or the agent follows it into
// nothing. The published resource list is the contract the guide promises.
func TestGuideReturnsOnlyPublishedResources(t *testing.T) {
	srv := New(testService(t), "test")
	payload := callGuide(t, srv, map[string]any{
		"task":   "animate a walk cycle for a knight",
		"inline": true,
	})

	published := make(map[string]bool)
	for _, r := range Resources() {
		published[r.URI] = true
	}
	assertPublished := func(uri string) {
		t.Helper()
		if uri == "" {
			t.Error("guide returned an empty URI")
			return
		}
		if !published[uri] {
			t.Errorf("%s is returned by guide but is not a published resource", uri)
		}
	}

	skillData, _ := payload["skill"].(map[string]any)
	skillURI, _ := skillData["uri"].(string)
	assertPublished(skillURI)

	for _, key := range []string{"rules", "references"} {
		list, _ := payload[key].([]any)
		for _, item := range list {
			entry, _ := item.(map[string]any)
			uri, _ := entry["uri"].(string)
			assertPublished(uri)
		}
	}
	for _, key := range []string{"included", "omitted"} {
		list, _ := payload[key].([]any)
		for _, item := range list {
			uri, _ := item.(string)
			assertPublished(uri)
		}
	}
}

// The connect text is the only thing an agent reads before its first call, so
// the guide and its limits have to be named there.
func TestTheSessionTextPointsAtTheGuide(t *testing.T) {
	text := Instructions + ResourceIndex()
	for _, want := range []string{"guide", "rules://index", "starting point"} {
		if !strings.Contains(text, want) {
			t.Errorf("the session text does not mention %q", want)
		}
	}
}

// callGuide invokes the tool through the real server and unwraps the structured
// payload, so a schema or wiring mistake fails here rather than in a client.
func callGuide(t *testing.T, srv *server.MCPServer, args map[string]any) map[string]any {
	t.Helper()

	body, err := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "tools/call",
		"params":  map[string]any{"name": "guide", "arguments": args},
	})
	if err != nil {
		t.Fatalf("encode request: %v", err)
	}

	resp, ok := srv.HandleMessage(context.Background(), body).(mcp.JSONRPCResponse)
	if !ok {
		t.Fatalf("the guide call did not answer: %+v", resp)
	}
	raw, err := json.Marshal(resp.Result)
	if err != nil {
		t.Fatalf("re-encode result: %v", err)
	}
	var result mcp.CallToolResult
	if err := json.Unmarshal(raw, &result); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if result.IsError {
		t.Fatalf("guide returned an error: %s", textOfResult(result))
	}
	payload, ok := result.StructuredContent.(map[string]any)
	if !ok {
		t.Fatalf("structured content is %T, want an object", result.StructuredContent)
	}
	return payload
}

func textOfResult(result mcp.CallToolResult) string {
	var parts []string
	for _, content := range result.Content {
		if text, ok := content.(mcp.TextContent); ok {
			parts = append(parts, text.Text)
		}
	}
	return strings.Join(parts, "\n")
}
