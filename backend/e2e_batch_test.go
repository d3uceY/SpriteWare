package backend

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
)

// The running binary must accept a batch with no top-level `op`.
//
// The four op-table tools advertise `ops` and used to mark `op` required, so a
// host refused the batch before any handler ran. A package-level test proves the
// schema; this one proves the *shipped* server, which is what a client meets.
func TestE2EBatchWithNoTopLevelOp(t *testing.T) {
	c := e2eClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Own document, so the check does not depend on what the user has open.
	callTool(t, ctx, c, "sprite", map[string]any{
		"op": "new", "name": "e2e-batch", "width": 8, "height": 8, "frames": 2,
	})

	// `op` is deliberately absent: `ops` alone has to be enough.
	result := callTool(t, ctx, c, "layer", map[string]any{
		"ops": []any{map[string]any{"op": "create", "name": "A"}},
	})
	if result.IsError {
		t.Fatalf("a batch-only call was refused: %s", textOf(result))
	}

	body := textOf(result)
	if !strings.Contains(body, "create") {
		t.Fatalf("expected the batch to run the create op, got: %s", body)
	}
}

// With neither `op` nor `ops` the tool has to say so in words.
func TestE2EEmptyCallNamesTheMissingOp(t *testing.T) {
	c := e2eClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	result := callTool(t, ctx, c, "tag", map[string]any{})
	if !result.IsError || !strings.Contains(textOf(result), "needs an op") {
		t.Fatalf("expected `tag needs an op`, got: %s", textOf(result))
	}
}

// The published schema is the artifact a host validates against.
func TestE2ETheRunningServerPublishesOpAsOptional(t *testing.T) {
	c := e2eClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	list, err := c.ListTools(ctx, mcp.ListToolsRequest{})
	if err != nil {
		t.Fatalf("tools/list: %v", err)
	}

	checked := 0
	for _, tool := range list.Tools {
		raw := tool.RawInputSchema
		if len(raw) == 0 {
			encoded, err := json.Marshal(tool.InputSchema)
			if err != nil {
				t.Fatalf("%s: schema does not encode: %v", tool.Name, err)
			}
			raw = encoded
		}

		var schema struct {
			Properties map[string]any `json:"properties"`
			Required   []string       `json:"required"`
		}
		if err := json.Unmarshal(raw, &schema); err != nil {
			t.Fatalf("%s: schema does not decode: %v", tool.Name, err)
		}
		if _, batched := schema.Properties["ops"]; !batched {
			continue
		}
		checked++
		for _, field := range schema.Required {
			if field == "op" {
				t.Errorf("%s: the running server still requires `op` next to `ops`", tool.Name)
			}
		}
	}
	if checked == 0 {
		t.Fatal("no tool on the running server publishes `ops`")
	}
}

// callTool drives one tool and unwraps the reply.
func callTool(t *testing.T, ctx context.Context, c *client.Client, name string, args map[string]any) mcp.CallToolResult {
	t.Helper()
	result, err := c.CallTool(ctx, mcp.CallToolRequest{
		Params: mcp.CallToolParams{Name: name, Arguments: args},
	})
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	return *result
}

// textOf is the text a caller reads when it ignores structured content.
func textOf(r mcp.CallToolResult) string {
	var parts []string
	for _, c := range r.Content {
		if text, ok := c.(mcp.TextContent); ok {
			parts = append(parts, text.Text)
		}
	}
	return strings.Join(parts, "\n")
}
