package backend

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
)

// Two dumps in flight must not read each other's manifest.
//
// The live and headless links cached ONE scratch directory per link, so parallel
// look/validate/export calls wrote the same dump.json and cel_N_M.png names and
// read back a half-written file:
//
//	internal: decode dump: unexpected end of JSON input
//
// A single request hides it, which is why it survived until two were issued at
// once.
func TestE2EParallelReadsDoNotCorruptTheDump(t *testing.T) {
	c := e2eClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	callTool(t, ctx, c, "sprite", map[string]any{
		"op": "new", "name": "e2e-parallel", "width": 16, "height": 16, "frames": 4,
	})
	callTool(t, ctx, c, "draw", map[string]any{
		"sprite": "e2e-parallel",
		"ops": []any{map[string]any{
			"kind": "rect", "x": 2, "y": 2, "width": 6, "height": 6,
			"fill": true, "color": "#ff0000",
			"target": map[string]any{"frame": 1, "layer": "Layer 1"},
		}},
	})

	const readers = 8
	var wg sync.WaitGroup
	errs := make([]error, readers)
	bodies := make([]string, readers)

	for i := 0; i < readers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			result, err := c.CallTool(ctx, mcp.CallToolRequest{
				Params: mcp.CallToolParams{
					Name: "look",
					Arguments: map[string]any{
						"op": "ascii", "sprite": "e2e-parallel", "frame": 1,
					},
				},
			})
			if err != nil {
				errs[i] = err
				return
			}
			bodies[i] = textOf(*result)
			if result.IsError {
				errs[i] = errToolRefused(bodies[i])
			}
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Errorf("reader %d: %v", i, err)
		}
	}
	for i, body := range bodies {
		if body != "" && body != bodies[0] {
			t.Errorf("reader %d saw a different document than reader 0", i)
		}
	}
}

type errToolRefused string

func (e errToolRefused) Error() string { return "look refused: " + string(e) }
