package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// A batch-only call has to survive the host's schema check and reach dispatch.
//
// The schema test above proves the published `required` list; this one proves
// the message actually gets through, which is the thing the user hit:
// "must have required property 'op'" came from validation, not from a handler.
func TestBatchOnlyCallReachesTheHandler(t *testing.T) {
	srv := server.NewMCPServer("test", "0")
	Register(srv, newTestService(t))

	resp := callTool(t, srv, "tag", map[string]any{
		"ops": []any{map[string]any{"op": "list"}},
	})

	// No Aseprite is running, so the op itself fails. What matters is that the
	// reply is a tool result carrying that failure, not an invalid-params error
	// from the schema before dispatch ran.
	result, isToolResult := toolResult(resp)
	if !isToolResult {
		t.Fatalf("batch-only call was rejected before dispatch: %s", describe(resp))
	}
	if !strings.Contains(textOf(result), "failed") {
		t.Fatalf("expected dispatch to report the op failure, got: %s", textOf(result))
	}
}

// The negative control: with no op and no ops, a tool must say so in words
// instead of failing schema validation with a JSON-path message.
func TestEmptyCallNamesTheMissingOp(t *testing.T) {
	srv := server.NewMCPServer("test", "0")
	Register(srv, newTestService(t))

	resp := callTool(t, srv, "tag", map[string]any{})

	result, isToolResult := toolResult(resp)
	if !isToolResult {
		t.Fatalf("empty call did not reach the handler: %s", describe(resp))
	}
	if !result.IsError || !strings.Contains(textOf(result), "needs an op") {
		t.Fatalf("expected `tag needs an op`, got: %s", textOf(result))
	}
}

// toolResult unwraps the JSON-RPC envelope HandleMessage returns, so a test
// asserts on the tool's own result rather than on the transport around it.
//
// Result is `any`, and mcp-go fills it with a pointer, a value or an error
// depending on the path, so the round trip normalises all three.
func toolResult(resp mcp.JSONRPCMessage) (mcp.CallToolResult, bool) {
	envelope, isResponse := resp.(mcp.JSONRPCResponse)
	if !isResponse {
		return mcp.CallToolResult{}, false
	}
	body, err := json.Marshal(envelope.Result)
	if err != nil {
		return mcp.CallToolResult{}, false
	}
	var result mcp.CallToolResult
	if err := json.Unmarshal(body, &result); err != nil {
		return mcp.CallToolResult{}, false
	}
	return result, true
}

func callTool(t *testing.T, srv *server.MCPServer, name string, args map[string]any) mcp.JSONRPCMessage {
	t.Helper()

	body, err := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "tools/call",
		"params":  map[string]any{"name": name, "arguments": args},
	})
	if err != nil {
		t.Fatalf("encode request: %v", err)
	}
	return srv.HandleMessage(context.Background(), body)
}

func textOf(r mcp.CallToolResult) string {
	var parts []string
	for _, c := range r.Content {
		if text, ok := c.(mcp.TextContent); ok {
			parts = append(parts, text.Text)
		}
	}
	return strings.Join(parts, "\n")
}

func describe(r mcp.JSONRPCMessage) string {
	body, err := json.Marshal(r)
	if err != nil {
		return "<unencodable>"
	}
	return string(body)
}
