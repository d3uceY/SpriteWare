package link

import "testing"

func TestIntegrationFillStopsAtTheEdge(t *testing.T) {
	h := realHeadless(t)
	newDoc(t, h, 10, 10)

	draw(t, h, solid(2, 2, 6, 6, "#ff0000"))
	draw(t, h, map[string]any{"kind": "fill", "x": 4, "y": 4, "color": "#0000ff"})

	d := mustDump(t, h)
	if _, _, b, a := pixelOf(t, d, 1, 4, 4); b != 255 || a != 255 {
		t.Fatalf("the seed was not filled: b=%d a=%d", b, a)
	}
	// A 6x6 rect at (2,2) covers 2..7, so the diagonal and corner beyond stay
	// empty.
	if _, _, _, a := pixelOf(t, d, 1, 0, 0); a != 0 {
		t.Fatalf("the fill leaked to the corner (alpha %d)", a)
	}
	if _, _, _, a := pixelOf(t, d, 1, 8, 8); a != 0 {
		t.Fatalf("the fill leaked past the square (alpha %d)", a)
	}
}

// Flood fill must not escape through a diagonal gap.
func TestIntegrationFillDoesNotLeakDiagonally(t *testing.T) {
	h := realHeadless(t)
	newDoc(t, h, 8, 8)

	// A diagonal wall of two pixels leaves a corner-to-corner gap.
	draw(t, h, map[string]any{
		"kind": "pixels", "color": "#ffffff",
		"points": []any{[]any{3, 0}, []any{4, 1}, []any{5, 2}, []any{6, 3}, []any{7, 4}},
	})
	draw(t, h, map[string]any{"kind": "fill", "x": 1, "y": 1, "color": "#00ff00"})

	d := mustDump(t, h)
	// The left of the wall is filled; the right of it never is.
	if _, g, _, _ := pixelOf(t, d, 1, 1, 1); g != 255 {
		t.Fatalf("the region left of the wall was not filled")
	}
	if _, g, _, _ := pixelOf(t, d, 1, 7, 0); g == 255 {
		t.Fatal("the fill leaked through the diagonal wall")
	}
}

func TestIntegrationPolylineDrawsEverySegment(t *testing.T) {
	h := realHeadless(t)
	newDoc(t, h, 10, 10)

	draw(t, h, map[string]any{
		"kind": "polyline", "color": "#ff00ff",
		"points": []any{[]any{1, 1}, []any{6, 1}, []any{6, 6}},
	})

	d := mustDump(t, h)
	for _, p := range [][2]int{{1, 1}, {4, 1}, {6, 1}, {6, 4}, {6, 6}} {
		if _, _, _, a := pixelOf(t, d, 1, p[0], p[1]); a != 255 {
			t.Errorf("the polyline missed (%d,%d)", p[0], p[1])
		}
	}
	if _, _, _, a := pixelOf(t, d, 1, 3, 3); a != 0 {
		t.Error("the polyline painted a pixel no segment crosses")
	}
}

func TestIntegrationDitherAlternatesTwoColours(t *testing.T) {
	h := realHeadless(t)
	newDoc(t, h, 8, 8)

	draw(t, h, map[string]any{
		"kind": "dither", "x": 0, "y": 0, "width": 4, "height": 4,
		"from": "#000000", "to": "#ffffff", "pattern": "checker",
	})

	d := mustDump(t, h)
	black, white := 0, 0
	for y := 0; y < 4; y++ {
		for x := 0; x < 4; x++ {
			r, _, _, a := pixelOf(t, d, 1, x, y)
			if a == 0 {
				t.Fatalf("(%d,%d) was left empty", x, y)
			}
			if r == 0 {
				black++
			} else {
				white++
			}
		}
	}
	if black != 8 || white != 8 {
		t.Fatalf("checker gave %d black and %d white, want 8 and 8", black, white)
	}
}

func TestIntegrationGradientRunsBetweenTheTwoColours(t *testing.T) {
	h := realHeadless(t)
	newDoc(t, h, 6, 8)

	draw(t, h, map[string]any{
		"kind": "gradient", "x": 0, "y": 0, "width": 6, "height": 8,
		"from": "#000000", "to": "#ffffff", "direction": "vertical", "steps": 4,
	})

	d := mustDump(t, h)
	top, _, _, _ := pixelOf(t, d, 1, 0, 0)
	bottom, _, _, _ := pixelOf(t, d, 1, 0, 7)
	if top != 0 {
		t.Errorf("the top row is %d, want the start colour", top)
	}
	if bottom != 255 {
		t.Errorf("the bottom row is %d, want the end colour", bottom)
	}

	// Four steps means four distinct values, not a smooth ramp of 8.
	seen := map[uint8]bool{}
	for y := 0; y < 8; y++ {
		r, _, _, _ := pixelOf(t, d, 1, 0, y)
		seen[r] = true
	}
	if len(seen) != 4 {
		t.Errorf("the gradient has %d bands, want 4", len(seen))
	}
}

func TestIntegrationHueShiftRotatesRedToGreen(t *testing.T) {
	h := realHeadless(t)
	newDoc(t, h, 6, 6)

	draw(t, h, solid(1, 1, 4, 4, "#ff0000"))
	draw(t, h, map[string]any{"kind": "hue_shift", "degrees": 120})

	d := mustDump(t, h)
	r, g, b, a := pixelOf(t, d, 1, 2, 2)
	if a != 255 {
		t.Fatalf("alpha was lost")
	}
	if g < 250 || r > 5 || b > 5 {
		t.Fatalf("red rotated by 120 degrees is (%d,%d,%d), want green", r, g, b)
	}
}

func TestIntegrationDesaturateRemovesHue(t *testing.T) {
	h := realHeadless(t)
	newDoc(t, h, 6, 6)

	draw(t, h, solid(1, 1, 4, 4, "#ff0000"))
	draw(t, h, map[string]any{"kind": "desaturate", "amount": 1})

	d := mustDump(t, h)
	r, g, b, _ := pixelOf(t, d, 1, 2, 2)
	if r != g || g != b {
		t.Fatalf("full desaturation left (%d,%d,%d), which is still a colour", r, g, b)
	}
	if r > 60 || r < 20 {
		t.Errorf("the luminance of red came out %d, which is not the expected ~54", r)
	}
}
