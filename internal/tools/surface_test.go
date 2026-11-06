package tools

import (
	"encoding/json"
	"testing"

	"github.com/mark3labs/mcp-go/server"
)

// A feature missing from the published enum is one the agent cannot reach. The
// lists are hand-written on purpose: reading them back would test nothing.
func TestTheAddedOperationsArePublished(t *testing.T) {
	want := map[string][]string{
		"export": {"gif", "spritesheet"},
		"look":   {"onion", "compare"},
		"cel":    {"copy", "tween", "oscillate"},
	}
	got := publishedEnums(t)

	for toolName, ops := range want {
		set, ok := got[toolName+".op"]
		if !ok {
			t.Errorf("%s publishes no op enum", toolName)
			continue
		}
		for _, opName := range ops {
			if !set[opName] {
				t.Errorf("%s does not publish op %q", toolName, opName)
			}
		}
	}
}

// draw kind=text is compiled in Go, so it has to be in the enum or the schema
// refuses it.
func TestDrawPublishesTheTextKind(t *testing.T) {
	if !publishedEnums(t)["draw.ops.items.kind"]["text"] {
		t.Fatal("draw kind enum does not include text, so the schema rejects it")
	}
}

// publishedEnums decodes every schema once and keys enums by path:
// "tool.property" or "tool.property.items.child".
func publishedEnums(t *testing.T) map[string]map[string]bool {
	t.Helper()

	srv := server.NewMCPServer("test", "0")
	Register(srv, newTestService(t))

	out := map[string]map[string]bool{}
	for name, tool := range srv.ListTools() {
		raw := schemaJSON(t, tool.Tool.RawInputSchema, tool.Tool.InputSchema)
		var schema struct {
			Properties map[string]struct {
				Enum  []string `json:"enum"`
				Items struct {
					Properties map[string]struct {
						Enum []string `json:"enum"`
					} `json:"properties"`
				} `json:"items"`
			} `json:"properties"`
		}
		if err := json.Unmarshal(raw, &schema); err != nil {
			t.Fatalf("%s: %v", name, err)
		}

		for prop, def := range schema.Properties {
			if len(def.Enum) > 0 {
				set := map[string]bool{}
				for _, v := range def.Enum {
					set[v] = true
				}
				out[name+"."+prop] = set
			}
			for child, childDef := range def.Items.Properties {
				if len(childDef.Enum) > 0 {
					set := map[string]bool{}
					for _, v := range childDef.Enum {
						set[v] = true
					}
					out[name+"."+prop+".items."+child] = set
				}
			}
		}
	}
	return out
}
