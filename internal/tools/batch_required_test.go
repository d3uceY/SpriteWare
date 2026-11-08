package tools

import (
	"encoding/json"
	"testing"

	"github.com/mark3labs/mcp-go/server"
)

// A tool that advertises `ops` must not also demand `op`: a host refuses the
// batch before any handler runs with "must have required property 'op'".
// extractOps accepts either shape, so the schema was the only obstacle.
func TestBatchToolsDoNotRequireOp(t *testing.T) {
	srv := server.NewMCPServer("test", "0")
	Register(srv, newTestService(t))

	// The four op-table tools. draw and validate publish `ops` through a raw
	// schema and have nothing to require, so they are not listed here.
	want := []string{"layer", "cel", "frame", "tag"}
	seen := map[string]bool{}

	for name, tool := range srv.ListTools() {
		raw := schemaJSON(t, tool.Tool.RawInputSchema, tool.Tool.InputSchema)

		var schema struct {
			Properties map[string]any `json:"properties"`
			Required   []string       `json:"required"`
		}
		if err := json.Unmarshal(raw, &schema); err != nil {
			t.Fatalf("%s: schema does not decode: %v", name, err)
		}
		if _, batched := schema.Properties["ops"]; !batched {
			continue
		}
		seen[name] = true

		for _, field := range schema.Required {
			if field == "op" {
				t.Errorf("%s: publishes `ops` but requires `op`, so a batch-only call is rejected by the schema", name)
			}
		}
	}

	// Without this the test could pass by finding nothing.
	for _, name := range want {
		if !seen[name] {
			t.Errorf("%s does not publish an `ops` array any more, so this test covers nothing for it", name)
		}
	}
}
