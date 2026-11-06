package tools

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/server"

	"github.com/d3uceY/spriteware/internal/config"
	"github.com/d3uceY/spriteware/internal/link"
)

func newTestService(t *testing.T) *Service {
	t.Helper()
	store, err := config.LoadFrom(filepath.Join(t.TempDir(), "config.json"))
	if err != nil {
		t.Fatalf("config: %v", err)
	}
	links := link.NewSelector(link.Options{Mode: config.ModeHeadless, BridgePort: 19999})
	t.Cleanup(func() { _ = links.Close() })
	return New(store, links)
}

// Every description is sent on every turn: a budget that drifts silently is not
// a budget.
func TestToolCountStaysUnderTheBudget(t *testing.T) {
	if len(ToolNames) > maxTools {
		t.Fatalf("%d tools, budget is %d", len(ToolNames), maxTools)
	}
}

// mcp-go panics on a duplicate name, so building the server proves coherence.
func TestRegisterBuildsEveryTool(t *testing.T) {
	srv := server.NewMCPServer("test", "0")
	Register(srv, newTestService(t))
}

// Every array schema has to say what it holds.
//
// A bare {"type": "array"} is invalid JSON Schema, and hosts are strict: VS Code
// drops the whole tool. This walks every published schema and fails on the first
// array without `items`.
func TestEveryArraySchemaDeclaresItems(t *testing.T) {
	srv := server.NewMCPServer("test", "0")
	Register(srv, newTestService(t))

	tools := srv.ListTools()
	if len(tools) == 0 {
		t.Fatal("no tools registered")
	}

	for name, tool := range tools {
		raw := schemaJSON(t, tool.Tool.RawInputSchema, tool.Tool.InputSchema)
		var schema any
		if err := json.Unmarshal(raw, &schema); err != nil {
			t.Fatalf("%s: schema does not decode: %v", name, err)
		}
		walkSchema(t, name, "input", schema)
	}
}

// walkSchema reports every array node that lacks `items`.
func walkSchema(t *testing.T, tool, path string, node any) {
	t.Helper()

	switch v := node.(type) {
	case map[string]any:
		if kind, _ := v["type"].(string); kind == "array" {
			if _, ok := v["items"]; !ok {
				t.Errorf("%s: %s is an array with no items", tool, path)
			}
		}
		for key, child := range v {
			walkSchema(t, tool, path+"."+key, child)
		}
	case []any:
		for i, child := range v {
			walkSchema(t, tool, fmt.Sprintf("%s[%d]", path, i), child)
		}
	}
}

// ---------------------------------------------------------------------------
// Grid compilation
// ---------------------------------------------------------------------------

func gridEntry(rows []string, legend map[string]any, transparent string) map[string]any {
	entry := map[string]any{
		"kind": "grid",
		"x":    float64(4),
		"y":    float64(6),
		"rows": toAnySlice(rows),
	}
	if legend != nil {
		entry["legend"] = legend
	}
	if transparent != "" {
		entry["transparent"] = transparent
	}
	return entry
}

func toAnySlice(in []string) []any {
	out := make([]any, len(in))
	for i, s := range in {
		out[i] = s
	}
	return out
}

func TestGridCompilesToClearPlusOneOpPerColour(t *testing.T) {
	ops, err := compileGrid(gridEntry(
		[]string{"AA..", "ABBA"},
		map[string]any{"A": "#ff0000", "B": "#00ff00"},
		"",
	), 0)
	if err != nil {
		t.Fatalf("compileGrid: %v", err)
	}
	if len(ops) != 3 {
		t.Fatalf("got %d ops, want 3 (clear + one per colour): %+v", len(ops), ops)
	}
	if ops[0]["kind"] != "clear" {
		t.Errorf("first op is %v, want clear", ops[0]["kind"])
	}
	if ops[0]["width"] != 4 || ops[0]["height"] != 2 {
		t.Errorf("clear rect is %vx%v, want 4x2", ops[0]["width"], ops[0]["height"])
	}

	// Coordinates must be sprite-absolute, offset by x/y.
	var total int
	for _, op := range ops[1:] {
		points, _ := op["points"].([][2]int)
		total += len(points)
		for _, p := range points {
			if p[0] < 4 || p[1] < 6 {
				t.Errorf("point %v is outside the grid origin (4,6)", p)
			}
		}
	}
	if total != 6 {
		t.Errorf("got %d painted points, want 6", total)
	}
}

func TestGridRefusesARaggedRow(t *testing.T) {
	_, err := compileGrid(gridEntry(
		[]string{"AAAA", "AAA"},
		map[string]any{"A": "#ff0000"},
		"",
	), 2)
	if err == nil {
		t.Fatal("a ragged row was accepted")
	}
	if !strings.Contains(err.Error(), "rows[1]") {
		t.Errorf("error does not name the offending row: %v", err)
	}
}

func TestGridRefusesAnUnknownGlyph(t *testing.T) {
	_, err := compileGrid(gridEntry(
		[]string{"AZ"},
		map[string]any{"A": "#ff0000"},
		"",
	), 0)
	if err == nil {
		t.Fatal("an unknown glyph was accepted")
	}
	if !strings.Contains(err.Error(), `"Z"`) {
		t.Errorf("error does not name the offending glyph: %v", err)
	}
}

func TestGridSkipsClearWhenTransparentIsSkip(t *testing.T) {
	ops, err := compileGrid(gridEntry(
		[]string{"AA"},
		map[string]any{"A": "#ff0000"},
		"skip",
	), 0)
	if err != nil {
		t.Fatalf("compileGrid: %v", err)
	}
	for _, op := range ops {
		if op["kind"] == "clear" {
			t.Fatal("skip mode emitted a clear op")
		}
	}
}

// A '.' is transparent unless the legend redefines it, which makes a
// read-edit-write round trip work.
func TestGridDotIsTransparentUnlessRedefined(t *testing.T) {
	ops, err := compileGrid(gridEntry(
		[]string{".A"},
		map[string]any{"A": "#ff0000"},
		"",
	), 0)
	if err != nil {
		t.Fatalf("compileGrid: %v", err)
	}
	total := 0
	for _, op := range ops {
		if points, ok := op["points"].([][2]int); ok {
			total += len(points)
		}
	}
	if total != 1 {
		t.Errorf("painted %d points, want 1", total)
	}

	ops, err = compileGrid(gridEntry(
		[]string{".."},
		map[string]any{".": "#00ff00"},
		"",
	), 0)
	if err != nil {
		t.Fatalf("compileGrid with a redefined dot: %v", err)
	}
	total = 0
	for _, op := range ops {
		if points, ok := op["points"].([][2]int); ok {
			total += len(points)
		}
	}
	if total != 2 {
		t.Errorf("a redefined '.' painted %d points, want 2", total)
	}
}
