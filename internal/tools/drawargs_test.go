package tools

import (
	"encoding/json"
	"strings"
	"testing"
)

// The schema and the handler must agree in both directions: a published kind
// with no table entry is a kind every argument fails on, and a table entry the
// schema does not publish is unreachable.
func TestDrawOpArgsMatchThePublishedSchema(t *testing.T) {
	var schema struct {
		Properties struct {
			Ops struct {
				Items struct {
					Properties map[string]any `json:"properties"`
				} `json:"items"`
			} `json:"ops"`
		} `json:"properties"`
	}
	if err := json.Unmarshal([]byte(drawSchema), &schema); err != nil {
		t.Fatalf("drawSchema does not decode: %v", err)
	}

	props := schema.Properties.Ops.Items.Properties
	rawKind, ok := props["kind"].(map[string]any)
	if !ok {
		t.Fatal("the schema publishes no kind property")
	}
	kinds, _ := rawKind["enum"].([]any)
	if len(kinds) == 0 {
		t.Fatal("the kind enum is empty")
	}

	common := map[string]bool{"kind": true, "target": true}
	covered := map[string]bool{}

	for _, raw := range kinds {
		kind, _ := raw.(string)
		args, listed := drawOpArgs[kind]
		if !listed {
			t.Errorf("kind=%s is published but has no drawOpArgs entry, so every argument fails", kind)
			continue
		}
		for _, arg := range args {
			if _, inSchema := props[arg]; !inSchema {
				t.Errorf("kind=%s reads %q, which the schema does not publish, so it is rejected before the handler runs", kind, arg)
			}
			covered[arg] = true
		}
	}

	for name := range props {
		if common[name] {
			continue
		}
		if !covered[name] {
			t.Errorf("the schema publishes %q but no kind reads it", name)
		}
	}
}

func TestValidateDrawOpsNamesTheUselessArgument(t *testing.T) {
	err := validateDrawOps([]any{
		map[string]any{"kind": "rect", "x": float64(0), "y": float64(0), "tolerance": float64(3)},
	})
	if err == nil {
		t.Fatal("rect accepted tolerance without complaint")
	}
	message := err.Error()
	if !strings.Contains(message, `"tolerance"`) || !strings.Contains(message, "kind=rect") {
		t.Fatalf("the error does not name the argument and the kind: %s", message)
	}
	if !strings.Contains(message, "ops[0]") {
		t.Fatalf("the error does not name the op index: %s", message)
	}
}

func TestValidateDrawOpsAcceptsARegionOnReplace(t *testing.T) {
	ok := []any{
		map[string]any{"kind": "replace", "from": "#ff0000", "to": "#00ff00", "x": float64(0), "y": float64(0), "width": float64(4), "height": float64(2)},
		map[string]any{"kind": "replace", "from": "#ff0000", "to": "#00ff00", "x0": float64(0), "y0": float64(0), "x1": float64(3), "y1": float64(1)},
		map[string]any{"kind": "grid", "x": float64(0), "y": float64(0), "rows": []any{"AB"}, "legend": map[string]any{"A": "#ff0000", "B": "#00ff00"}},
		map[string]any{"kind": "text", "text": "hi", "color": "#ffffff", "x": float64(0), "y": float64(0), "target": map[string]any{"layer": "a", "frame": float64(1)}},
	}
	if err := validateDrawOps(ok); err != nil {
		t.Fatalf("a valid op list was refused: %v", err)
	}
}

func TestValidateDrawOpsRefusesAnUnknownKind(t *testing.T) {
	err := validateDrawOps([]any{map[string]any{"kind": "spray"}})
	if err == nil {
		t.Fatal("an unknown kind was accepted; the Lua side would be the first to notice")
	}
	if !strings.Contains(err.Error(), `unknown kind "spray"`) {
		t.Fatalf("the error does not name the kind: %s", err)
	}
}

// The colour echo has to count every way a kind names its paint colour, because
// a grid legend that maps two characters to one colour is invisible otherwise.
func TestPaintColoursReadsEveryPaintField(t *testing.T) {
	got := paintColours([]map[string]any{
		{"kind": "pixels", "color": "#ff0000"},
		{"kind": "replace", "from": "#00ff00", "to": "#0000ff"},
		{"kind": "frame", "outline": "#000000", "hi": "#ffffff", "lo": "#808080", "body": "#ff0000"},
		{"kind": "grid", "x": float64(0), "y": float64(0)},
	})

	want := []string{"#000000", "#0000ff", "#808080", "#ff0000", "#ffffff"}
	if len(got) != len(want) {
		t.Fatalf("colours = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("colours = %v, want %v (sorted, deduped, source colours excluded)", got, want)
		}
	}
}
