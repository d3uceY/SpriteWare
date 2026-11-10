package link

import (
	"context"
	"image"
	"testing"

	"github.com/d3uceY/spriteware/internal/doc"
)

func layerImage(t *testing.T, d *doc.Dump, name string, frame int) *image.NRGBA {
	t.Helper()
	idx := d.IndexOfLayer(name)
	if idx < 0 {
		t.Fatalf("no layer named %q", name)
	}
	return d.LayerImage(idx, frame)
}

// inkBounds is where the pixels are: what every transform must preserve or move
// predictably.
func inkBounds(img *image.NRGBA) (x, y, w, h int, ok bool) {
	r, has := doc.Bounds(img)
	if !has {
		return 0, 0, 0, 0, false
	}
	return r.Min.X, r.Min.Y, r.Dx(), r.Dy(), true
}

func TestIntegrationTransformFlipMirrorsTheCel(t *testing.T) {
	h := realHeadless(t)
	newDoc(t, h, 10, 4)

	draw(t, h, map[string]any{"kind": "pixels", "color": "#ff0000", "points": []any{[]any{1, 1}}})

	if err := h.Call(context.Background(), "transform.apply", map[string]any{
		"ops": []any{map[string]any{"kind": "flip", "direction": "horizontal", "target": map[string]any{"layer": "Layer 1"}}},
	}, &map[string]any{}); err != nil {
		t.Fatalf("flip: %v", err)
	}

	d := mustDump(t, h)
	x, y, _, _, ok := inkBounds(layerImage(t, d, "Layer 1", 1))
	if !ok {
		t.Fatal("the flip erased the art")
	}
	// A 10-wide cel mirrors x=1 to x=8.
	if x != 8 || y != 1 {
		t.Fatalf("the pixel landed at (%d,%d), want (8,1)", x, y)
	}
}

func TestIntegrationTransformRotate90SwapsTheAspect(t *testing.T) {
	h := realHeadless(t)
	newDoc(t, h, 10, 10)

	// A vertical bar, one pixel wide and four tall.
	draw(t, h, solid(3, 2, 1, 4, "#00ff00"))

	if err := h.Call(context.Background(), "transform.apply", map[string]any{
		"ops": []any{map[string]any{"kind": "rotate", "angle": 90, "target": map[string]any{"layer": "Layer 1"}}},
	}, &map[string]any{}); err != nil {
		t.Fatalf("rotate: %v", err)
	}

	d := mustDump(t, h)
	_, _, w, hgt, ok := inkBounds(layerImage(t, d, "Layer 1", 1))
	if !ok {
		t.Fatal("the rotate erased the art")
	}
	if w != 4 || hgt != 1 {
		t.Fatalf("the bar is %dx%d after a 90 degree turn, want 4x1", w, hgt)
	}
}

// A non-multiple of 90 would resample, so it is refused.
func TestIntegrationTransformRefusesAFreeRotate(t *testing.T) {
	h := realHeadless(t)
	newDoc(t, h, 8, 8)
	draw(t, h, solid(2, 2, 2, 2, "#ffffff"))

	err := h.Call(context.Background(), "transform.apply", map[string]any{
		"ops": []any{map[string]any{"kind": "rotate", "angle": 45, "target": map[string]any{"layer": "Layer 1"}}},
	}, &map[string]any{})
	if err == nil {
		t.Fatal("a 45 degree rotate was accepted")
	}
}

func TestIntegrationTransformScaleIsNearestNeighbour(t *testing.T) {
	h := realHeadless(t)
	newDoc(t, h, 12, 12)

	draw(t, h, solid(1, 1, 2, 2, "#0000ff"))

	if err := h.Call(context.Background(), "transform.apply", map[string]any{
		"ops": []any{map[string]any{"kind": "scale", "factor": 3, "target": map[string]any{"layer": "Layer 1"}}},
	}, &map[string]any{}); err != nil {
		t.Fatalf("scale: %v", err)
	}

	d := mustDump(t, h)
	x, y, w, hgt, ok := inkBounds(layerImage(t, d, "Layer 1", 1))
	if !ok {
		t.Fatal("the scale erased the art")
	}
	if x != 3 || y != 3 || w != 6 || hgt != 6 {
		t.Fatalf("a 2x2 at (1,1) scaled 3x is at (%d,%d) %dx%d, want (3,3) 6x6", x, y, w, hgt)
	}
	// Nearest neighbour means no new colours: every pixel is still the one blue.
	img := layerImage(t, d, "Layer 1", 1)
	c := img.NRGBAAt(4, 4)
	if c.R != 0 || c.G != 0 || c.B != 255 || c.A != 255 {
		t.Fatalf("the scaled art picked up a new colour: %v", c)
	}
}

// Cropping changes the cel, not the picture.
func TestIntegrationTransformCropKeepsTheArtInPlace(t *testing.T) {
	h := realHeadless(t)
	newDoc(t, h, 12, 12)

	draw(t, h, solid(4, 5, 3, 2, "#ff00ff"))

	before := mustDump(t, h)
	bx, by, bw, bh, _ := inkBounds(layerImage(t, before, "Layer 1", 1))

	if err := h.Call(context.Background(), "transform.apply", map[string]any{
		"ops": []any{map[string]any{"kind": "crop_to_content", "target": map[string]any{"layer": "Layer 1"}}},
	}, &map[string]any{}); err != nil {
		t.Fatalf("crop: %v", err)
	}

	after := mustDump(t, h)
	ax, ay, aw, ah, ok := inkBounds(layerImage(t, after, "Layer 1", 1))
	if !ok {
		t.Fatal("the crop erased the art")
	}
	if ax != bx || ay != by || aw != bw || ah != bh {
		t.Fatalf("the art moved from (%d,%d) %dx%d to (%d,%d) %dx%d", bx, by, bw, bh, ax, ay, aw, ah)
	}
}

func TestIntegrationTweenMovesThroughTheMiddle(t *testing.T) {
	h := realHeadless(t)
	newDoc(t, h, 20, 8)
	for i := 0; i < 4; i++ {
		if err := h.Call(context.Background(), "frame.add", map[string]any{"count": 1}, &map[string]any{}); err != nil {
			t.Fatalf("frame.add: %v", err)
		}
	}
	first := solid(0, 2, 2, 2, "#ffffff")
	first["target"] = map[string]any{"frame": 1}
	draw(t, h, first)

	if err := h.Call(context.Background(), "cel.tween", map[string]any{
		"layer":     "Layer 1",
		"fromFrame": 1,
		"toFrame":   5,
		"fromX":     0,
		"toX":       8,
		"easing":    "linear",
	}, &map[string]any{}); err != nil {
		t.Fatalf("tween: %v", err)
	}

	d := mustDump(t, h)
	lastX := -1
	for f := 1; f <= 5; f++ {
		x, _, _, _, ok := inkBounds(layerImage(t, d, "Layer 1", f))
		if !ok {
			t.Fatalf("frame %d is empty: the tween did not fill it", f)
		}
		if x < lastX {
			t.Fatalf("frame %d moved backwards (%d after %d)", f, x, lastX)
		}
		lastX = x
	}
	if lastX != 8 {
		t.Fatalf("the tween ended at x=%d, want 8", lastX)
	}
}

// Easing has to change the spacing, or the curve is decoration.
func TestIntegrationTweenEasingChangesSpacing(t *testing.T) {
	positions := func(easing string) int {
		h := realHeadless(t)
		newDoc(t, h, 20, 8)
		for i := 0; i < 4; i++ {
			if err := h.Call(context.Background(), "frame.add", map[string]any{"count": 1}, &map[string]any{}); err != nil {
				t.Fatalf("frame.add: %v", err)
			}
		}
		first := solid(0, 2, 2, 2, "#ffffff")
		first["target"] = map[string]any{"frame": 1}
		draw(t, h, first)
		if err := h.Call(context.Background(), "cel.tween", map[string]any{
			"layer": "Layer 1", "fromFrame": 1, "toFrame": 5,
			"fromX": 0, "toX": 16, "easing": easing,
		}, &map[string]any{}); err != nil {
			t.Fatalf("tween %s: %v", easing, err)
		}
		d := mustDump(t, h)
		x, _, _, _, _ := inkBounds(layerImage(t, d, "Layer 1", 2))
		return x
	}

	linear := positions("linear")
	eased := positions("ease_in")
	if eased >= linear {
		t.Fatalf("ease_in started at %d and linear at %d: the curve did nothing", eased, linear)
	}
}

func TestIntegrationOscillateBobs(t *testing.T) {
	h := realHeadless(t)
	newDoc(t, h, 8, 8)
	for i := 0; i < 5; i++ {
		if err := h.Call(context.Background(), "frame.add", map[string]any{"count": 1}, &map[string]any{}); err != nil {
			t.Fatalf("frame.add: %v", err)
		}
	}
	first := solid(2, 2, 2, 2, "#00ffff")
	first["target"] = map[string]any{"frame": 1}
	draw(t, h, first)

	if err := h.Call(context.Background(), "cel.oscillate", map[string]any{
		"layer":      "Layer 1",
		"fromFrame":  1,
		"toFrame":    6,
		"amplitudeY": 1,
		"period":     6,
	}, &map[string]any{}); err != nil {
		t.Fatalf("oscillate: %v", err)
	}

	d := mustDump(t, h)
	seen := map[int]bool{}
	for f := 1; f <= 6; f++ {
		_, y, _, _, ok := inkBounds(layerImage(t, d, "Layer 1", f))
		if !ok {
			t.Fatalf("frame %d is empty", f)
		}
		seen[y] = true
	}
	if len(seen) < 2 {
		t.Fatalf("the sprite never moved: offsets %v", seen)
	}
}

func TestIntegrationCelTweenRefusesAFrameThatDoesNotExist(t *testing.T) {
	h := realHeadless(t)
	newDoc(t, h, 6, 6)
	draw(t, h, solid(1, 1, 2, 2, "#ffffff"))

	err := h.Call(context.Background(), "cel.tween", map[string]any{
		"layer": "Layer 1", "fromFrame": 1, "toFrame": 9, "toX": 4,
	}, &map[string]any{})
	if err == nil {
		t.Fatal("a tween past the last frame was accepted")
	}
}
