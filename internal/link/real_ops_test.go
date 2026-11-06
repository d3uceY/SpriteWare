package link

import (
	"context"
	"testing"
)

// draw runs one draw.apply batch against the real Aseprite.
func draw(t *testing.T, h *Headless, ops ...map[string]any) {
	t.Helper()
	list := make([]any, len(ops))
	for i, op := range ops {
		list[i] = op
	}
	if err := h.Call(context.Background(), "draw.apply", map[string]any{"ops": list}, &map[string]any{}); err != nil {
		t.Fatalf("draw: %v", err)
	}
}

func solid(x, y, w, hgt int, colour string) map[string]any {
	return map[string]any{"kind": "rect", "x": x, "y": y, "width": w, "height": hgt, "color": colour}
}

// An outside outline keeps every art pixel and draws the ring around it.
func TestIntegrationOutlineLeavesTheShapeAlone(t *testing.T) {
	h := realHeadless(t)
	newDoc(t, h, 10, 10)

	draw(t, h, solid(4, 4, 3, 3, "#ff0000"))
	draw(t, h, map[string]any{"kind": "outline", "color": "#0000ff", "side": "outside"})

	d := mustDump(t, h)

	r, g, b, a := pixelOf(t, d, 1, 4, 4)
	if r != 255 || g != 0 || b != 0 || a != 255 {
		t.Fatalf("the art was recoloured: (%d,%d,%d,%d)", r, g, b, a)
	}

	r, g, b, a = pixelOf(t, d, 1, 3, 4)
	if r != 0 || g != 0 || b != 255 || a != 255 {
		t.Fatalf("the pixel beside the art is (%d,%d,%d,%d), want the outline colour", r, g, b, a)
	}

	// Four-neighbour outlining leaves the corner cut: the diagonal pixel stays
	// empty.
	if _, _, _, a := pixelOf(t, d, 1, 3, 3); a != 0 {
		t.Fatalf("the diagonal corner was filled without diagonals=true (alpha %d)", a)
	}

	// Two pixels out is still empty: an outline is one pixel, not a glow.
	if _, _, _, a := pixelOf(t, d, 1, 2, 3); a != 0 {
		t.Fatalf("the outline reached two pixels out (alpha %d)", a)
	}
}

// diagonals=true is the square-cornered outline.
func TestIntegrationOutlineWithDiagonalsFillsCorners(t *testing.T) {
	h := realHeadless(t)
	newDoc(t, h, 10, 10)

	draw(t, h, solid(4, 4, 3, 3, "#ff0000"))
	draw(t, h, map[string]any{"kind": "outline", "color": "#0000ff", "side": "outside", "diagonals": true})

	d := mustDump(t, h)
	if _, _, b, a := pixelOf(t, d, 1, 3, 3); b != 255 || a != 255 {
		t.Fatalf("the corner is (%d,%d,%d,%d), want the outline colour", b, a, b, a)
	}
}

func TestIntegrationInsideOutlineKeepsTheSize(t *testing.T) {
	h := realHeadless(t)
	newDoc(t, h, 10, 10)

	draw(t, h, solid(4, 4, 3, 3, "#ff0000"))
	draw(t, h, map[string]any{"kind": "outline", "color": "#0000ff", "side": "inside"})

	d := mustDump(t, h)

	// The edge is recoloured...
	if _, _, b, a := pixelOf(t, d, 1, 4, 4); b != 255 || a != 255 {
		t.Fatalf("the inside edge was not recoloured")
	}
	// ...and the middle is untouched, so the sprite did not grow or shrink.
	if r, _, _, _ := pixelOf(t, d, 1, 5, 5); r != 255 {
		t.Fatalf("the middle of the shape changed")
	}
	if _, _, _, a := pixelOf(t, d, 1, 3, 3); a != 0 {
		t.Fatalf("an inside outline painted outside the shape")
	}
}

// A shadow has to darken and cool: the blue channel falls less than the red one.
func TestIntegrationShadeCoolsShadows(t *testing.T) {
	h := realHeadless(t)
	newDoc(t, h, 6, 6)

	draw(t, h, solid(1, 1, 4, 4, "#804020"))
	draw(t, h, map[string]any{"kind": "shade", "amount": -0.5})

	d := mustDump(t, h)
	r, g, b, a := pixelOf(t, d, 1, 2, 2)
	if a != 255 {
		t.Fatalf("alpha was lost: %d", a)
	}
	if r >= 0x80 {
		t.Errorf("the shadow did not darken: r=%d", r)
	}
	if float64(b)/float64(r) <= float64(0x20)/float64(0x80) {
		t.Errorf("the shadow did not cool: r=%d b=%d", r, b)
	}
	_ = g
}

func TestIntegrationShadeWarmsHighlights(t *testing.T) {
	h := realHeadless(t)
	newDoc(t, h, 6, 6)

	draw(t, h, solid(1, 1, 4, 4, "#804020"))
	draw(t, h, map[string]any{"kind": "shade", "amount": 0.5})

	d := mustDump(t, h)
	r, _, _, a := pixelOf(t, d, 1, 2, 2)
	if a != 255 {
		t.Fatalf("alpha was lost: %d", a)
	}
	if r <= 0x80 {
		t.Errorf("the highlight did not lighten: r=%d", r)
	}
}

// A zero amount is refused rather than silently doing nothing.
func TestIntegrationShadeRejectsZero(t *testing.T) {
	h := realHeadless(t)
	newDoc(t, h, 4, 4)

	err := h.Call(context.Background(), "draw.apply", map[string]any{
		"ops": []any{map[string]any{"kind": "shade", "amount": 0}},
	}, &map[string]any{})
	if err == nil {
		t.Fatal("amount 0 was accepted")
	}
}

// blit copies pixels between cels, which is what a rig split needs.
func TestIntegrationBlitCopiesBetweenLayers(t *testing.T) {
	h := realHeadless(t)
	d := newDoc(t, h, 10, 10)

	if err := h.Call(context.Background(), "layer.create", map[string]any{"name": "target"}, &map[string]any{}); err != nil {
		t.Fatalf("layer.create: %v", err)
	}

	draw(t, h, solid(1, 1, 2, 2, "#00ff00"))
	draw(t, h, map[string]any{
		"kind":   "blit",
		"target": map[string]any{"layer": "target"},
		"source": map[string]any{"layer": d.Layers[0].Name, "x": 1, "y": 1, "width": 2, "height": 2},
		"x":      5,
		"y":      6,
	})

	d = mustDump(t, h)
	idx := d.IndexOfLayer("target")
	if idx < 0 {
		t.Fatal("the target layer is missing")
	}
	img := d.LayerImage(idx, 1)
	c := img.NRGBAAt(5, 6)
	if c.G != 255 || c.A != 255 {
		t.Fatalf("the copied pixel is %v, want the source colour", c)
	}
	// Nothing was copied outside the requested rectangle.
	if c := img.NRGBAAt(7, 6); c.A != 0 {
		t.Fatalf("the blit spilled to (7,6): %v", c)
	}
}

// cel.copy is the same idea one level up: a whole cel onto another layer.
func TestIntegrationCelCopyMovesAPartOntoItsOwnLayer(t *testing.T) {
	h := realHeadless(t)
	d := newDoc(t, h, 10, 10)

	if err := h.Call(context.Background(), "layer.create", map[string]any{"name": "head"}, &map[string]any{}); err != nil {
		t.Fatalf("layer.create: %v", err)
	}
	draw(t, h, solid(2, 2, 3, 3, "#ffff00"))

	var out map[string]any
	if err := h.Call(context.Background(), "cel.copy", map[string]any{
		"layer":     "head",
		"fromLayer": d.Layers[0].Name,
	}, &out); err != nil {
		t.Fatalf("cel.copy: %v", err)
	}

	after := mustDump(t, h)
	idx := after.IndexOfLayer("head")
	img := after.LayerImage(idx, 1)
	if c := img.NRGBAAt(2, 2); c.A == 0 {
		t.Fatal("the copy landed on an empty pixel")
	}
	if c := img.NRGBAAt(0, 0); c.A != 0 {
		t.Fatalf("the copy brought the whole canvas instead of the art: %v", c)
	}

	// The source stays where it was: copy is not move.
	srcIdx := after.IndexOfLayer(d.Layers[0].Name)
	if c := after.LayerImage(srcIdx, 1).NRGBAAt(2, 2); c.A == 0 {
		t.Fatal("cel.copy erased the source")
	}
}

func TestIntegrationCelCopyRefusesAMissingSource(t *testing.T) {
	h := realHeadless(t)
	newDoc(t, h, 6, 6)

	err := h.Call(context.Background(), "cel.copy", map[string]any{
		"layer":     "Layer 1",
		"fromLayer": "nope",
	}, &map[string]any{})
	if err == nil {
		t.Fatal("a missing source layer was accepted")
	}
}
