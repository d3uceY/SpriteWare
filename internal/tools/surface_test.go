package tools

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/server"
)

// A feature missing from the published enum is one the agent cannot reach. The
// lists are hand-written on purpose: reading them back would test nothing.
func TestTheAddedOperationsArePublished(t *testing.T) {
	want := map[string][]string{
		"export":  {"gif", "spritesheet", "tileset"},
		"look":    {"onion", "compare"},
		"cel":     {"copy", "tween", "oscillate"},
		"layer":   {"group", "ungroup", "merge"},
		"palette": {"snap"},
		"select":  {"set", "all", "none", "info"},
		"slice":   {"list", "create", "update", "delete"},
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

// The kinds added for the UI-art workflow must be reachable: a handler branch
// the schema does not publish cannot be called.
func TestDrawPublishesTheNewKinds(t *testing.T) {
	set := publishedEnums(t)["draw.ops.items.kind"]
	for _, kind := range []string{"remap", "frame", "shift"} {
		if !set[kind] {
			t.Errorf("draw kind enum does not include %q", kind)
		}
	}
}

// draw closes its schema with additionalProperties:false, so an argument its
// description offers but its schema omits is rejected by the host before any
// Lua runs. Every top-level argument the tool supports has to be declared.
func TestDrawDeclaresEveryTopLevelArgument(t *testing.T) {
	got := publishedProperties(t)["draw"]

	for _, want := range []string{"ops", "label", "sprite", "ignoreSelection"} {
		if !got[want] {
			t.Errorf("draw does not declare %q, so a caller passing it is rejected", want)
		}
	}
}

// The catalogue is what the Tools screen and doctor show. A tool registered but
// not catalogued is invisible in the UI; a catalogued name no tool answers is a
// lie the user only finds by clicking it.
func TestCatalogMatchesTheRegisteredTools(t *testing.T) {
	srv := server.NewMCPServer("test", "0")
	Register(srv, newTestService(t))

	registered := map[string]bool{}
	for name := range srv.ListTools() {
		registered[name] = true
	}

	catalogued := map[string]bool{}
	for _, info := range Catalog {
		catalogued[info.Name] = true
		if strings.TrimSpace(info.Blurb) == "" {
			t.Errorf("%s has no blurb", info.Name)
		}
		if !registered[info.Name] {
			t.Errorf("the catalogue lists %q, which no tool answers", info.Name)
		}
	}

	for name := range registered {
		if !catalogued[name] {
			t.Errorf("%q is registered but missing from the catalogue", name)
		}
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

// publishedProperties returns each tool's declared top-level argument names.
func publishedProperties(t *testing.T) map[string]map[string]bool {
	t.Helper()

	srv := server.NewMCPServer("test", "0")
	Register(srv, newTestService(t))

	out := map[string]map[string]bool{}
	for name, tool := range srv.ListTools() {
		raw := schemaJSON(t, tool.Tool.RawInputSchema, tool.Tool.InputSchema)
		var schema struct {
			Properties map[string]any `json:"properties"`
		}
		if err := json.Unmarshal(raw, &schema); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		names := map[string]bool{}
		for prop := range schema.Properties {
			names[prop] = true
		}
		out[name] = names
	}
	return out
}
