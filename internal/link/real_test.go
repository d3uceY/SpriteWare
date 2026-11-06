package link

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/d3uceY/spriteware/internal/doc"
)

// These tests drive a real Aseprite, so they are opt-in:
//
//	$env:SPRITEWARE_ASEPRITE = "C:\path\to\aseprite.exe"
//	go test ./internal/link -run Integration -v
//
// Everything above is tested against a fake peer, which cannot tell a wrong Lua
// call from a missing one. A handler is untested until it runs in Aseprite.

// realAseprite returns the configured binary, or skips.
func realAseprite(t *testing.T) string {
	t.Helper()
	exe := os.Getenv("SPRITEWARE_ASEPRITE")
	if exe == "" {
		t.Skip("set SPRITEWARE_ASEPRITE to the Aseprite executable to run these")
	}
	if _, err := os.Stat(exe); err != nil {
		t.Fatalf("SPRITEWARE_ASEPRITE points at %q: %v", exe, err)
	}
	return exe
}

// realHeadless starts batch Aseprite with its own temp user folder, so a test
// never touches the user's settings or extensions.
func realHeadless(t *testing.T) *Headless {
	t.Helper()
	h, err := NewHeadless(HeadlessOptions{
		Exe:            realAseprite(t),
		Timeout:        60 * time.Second,
		StartupTimeout: 60 * time.Second,
	})
	if err != nil {
		t.Fatalf("headless: %v", err)
	}
	t.Cleanup(func() { _ = h.Close() })

	// One cheap call proves the link is up before a test spends time on drawing.
	var pong struct {
		Pong bool `json:"pong"`
	}
	if err := h.Call(context.Background(), "ping", nil, &pong); err != nil {
		t.Fatalf("ping: %v", err)
	}
	if !pong.Pong {
		t.Fatal("Aseprite answered ping without pong")
	}
	return h
}

// newDoc creates a fresh document and returns its dump.
func newDoc(t *testing.T, h *Headless, w, hgt int) *doc.Dump {
	t.Helper()
	var out map[string]any
	if err := h.Call(context.Background(), "doc.new", map[string]any{"width": w, "height": hgt}, &out); err != nil {
		t.Fatalf("doc.new: %v", err)
	}
	return mustDump(t, h)
}

func mustDump(t *testing.T, h *Headless) *doc.Dump {
	t.Helper()
	d, err := h.Dump(context.Background(), nil)
	if err != nil {
		t.Fatalf("dump: %v", err)
	}
	return d
}

// pixelOf reads one pixel of the composite, which is what the caller sees.
func pixelOf(t *testing.T, d *doc.Dump, frame, x, y int) (r, g, b, a uint8) {
	t.Helper()
	img := d.Composite(frame)
	c := img.NRGBAAt(x, y)
	return c.R, c.G, c.B, c.A
}

// The smoke test for the live stack: real binary, one draw, read it back.
func TestIntegrationSmoke(t *testing.T) {
	h := realHeadless(t)
	newDoc(t, h, 8, 8)

	if err := h.Call(context.Background(), "draw.apply", map[string]any{
		"ops": []any{map[string]any{
			"kind": "pixels", "color": "#ff0000", "points": []any{
				[]any{1, 1}, []any{2, 1}, []any{1, 2}, []any{2, 2},
			},
		}},
	}, &map[string]any{}); err != nil {
		t.Fatalf("draw: %v", err)
	}

	d := mustDump(t, h)
	r, g, b, a := pixelOf(t, d, 1, 1, 1)
	if r != 255 || g != 0 || b != 0 || a != 255 {
		t.Fatalf("pixel (1,1) = %d,%d,%d,%d; the grid did not land", r, g, b, a)
	}
}
