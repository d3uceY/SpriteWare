package backend

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
)

// The `target` in a reply must name the document the command acted on, not the
// active one: that field is how a caller answers "did this touch the right
// document".
func TestE2ETargetNamesTheDocumentThatWasActedOn(t *testing.T) {
	c := e2eClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	callTool(t, ctx, c, "sprite", map[string]any{
		"op": "new", "name": "e2e-target-a", "width": 8, "height": 8, "frames": 1,
	})
	// Created second, so it is the active document while we act on the first.
	callTool(t, ctx, c, "sprite", map[string]any{
		"op": "new", "name": "e2e-target-b", "width": 8, "height": 8, "frames": 1,
	})

	result := callTool(t, ctx, c, "draw", map[string]any{
		"sprite": "e2e-target-a",
		"ops": []any{map[string]any{
			"kind": "rect", "x": 1, "y": 1, "width": 2, "height": 2,
			"fill": true, "color": "#00ff00",
			"target": map[string]any{"frame": 1, "layer": "Layer 1"},
		}},
	})
	if result.IsError {
		t.Fatalf("draw refused: %s", textOf(result))
	}

	target := targetOf(t, result)
	if target != "e2e-target-a" {
		t.Fatalf("the reply names %q as the target, but the draw was aimed at e2e-target-a", target)
	}

	// And the pixels really are in a, not b.
	if got := textOf(callTool(t, ctx, c, "look", map[string]any{
		"op": "ascii", "sprite": "e2e-target-a", "frame": 1, "rulers": false,
	})); !strings.Contains(got, "#00ff00") {
		t.Errorf("e2e-target-a holds no green pixels: %s", got)
	}
	if got := textOf(callTool(t, ctx, c, "look", map[string]any{
		"op": "ascii", "sprite": "e2e-target-b", "frame": 1, "rulers": false,
	})); strings.Contains(got, "#00ff00") {
		t.Errorf("e2e-target-b was drawn into: %s", got)
	}
}

// targetOf digs the id out of the structured `target` object.
func targetOf(t *testing.T, r mcp.CallToolResult) string {
	t.Helper()
	raw, err := json.Marshal(r)
	if err != nil {
		t.Fatalf("reply does not encode: %v", err)
	}
	var envelope struct {
		StructuredContent struct {
			Target struct {
				ID     string `json:"id"`
				Name   string `json:"name"`
				Active bool   `json:"active"`
			} `json:"target"`
		} `json:"structuredContent"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		t.Fatalf("reply does not decode: %v", err)
	}
	if envelope.StructuredContent.Target.ID == "" {
		return envelope.StructuredContent.Target.Name
	}
	return envelope.StructuredContent.Target.ID
}
