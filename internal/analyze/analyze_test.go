package analyze

import (
	"image"
	"image/color"
	"testing"

	"github.com/d3uceY/spriteware/internal/craft"
	"github.com/d3uceY/spriteware/internal/doc"
)

// ---------------------------------------------------------------------------
// Fixtures: a sprite is built in memory, so a defect is one line to inject.
// ---------------------------------------------------------------------------

func newDump(w, h int, layers ...string) *doc.Dump {
	d := &doc.Dump{
		Sprite: doc.Sprite{
			ID: "test.aseprite", Name: "test", Path: "test.aseprite",
			Width: w, Height: h, ColorMode: "rgb", Saved: true,
		},
		Frames: []doc.Frame{{DurationMS: 100}},
		Tags:   []doc.Tag{{Name: "idle", From: 1, To: 1, Direction: "forward"}},
	}
	for _, name := range layers {
		d.Layers = append(d.Layers, doc.Layer{Name: name, Visible: true, Opacity: 255})
	}
	return d
}

func addFrame(d *doc.Dump, durationMS int) {
	d.Frames = append(d.Frames, doc.Frame{DurationMS: durationMS})
	d.Tags[0].To = len(d.Frames)
}

// cel returns the layer's canvas for a frame, creating it on first use.
func cel(t *testing.T, d *doc.Dump, layer, frame int) *image.NRGBA {
	t.Helper()
	if layer < 0 || layer >= len(d.Layers) {
		t.Fatalf("layer %d does not exist", layer)
	}
	for i := range d.Layers[layer].Cels {
		if d.Layers[layer].Cels[i].Frame == frame {
			return d.Layers[layer].Cels[i].Pixels
		}
	}
	img := image.NewNRGBA(image.Rect(0, 0, d.Sprite.Width, d.Sprite.Height))
	d.Layers[layer].Cels = append(d.Layers[layer].Cels, doc.Cel{
		Frame: frame, Width: d.Sprite.Width, Height: d.Sprite.Height, Pixels: img,
	})
	return img
}

func fillRect(img *image.NRGBA, x, y, w, h int, c craft.RGB) {
	for py := y; py < y+h; py++ {
		for px := x; px < x+w; px++ {
			img.SetNRGBA(px, py, rgba(c))
		}
	}
}

func outlineRect(img *image.NRGBA, x, y, w, h int, fill, line craft.RGB, skipRightEdge bool) {
	fillRect(img, x, y, w, h, fill)
	for px := x; px < x+w; px++ {
		img.SetNRGBA(px, y, rgba(line))
		img.SetNRGBA(px, y+h-1, rgba(line))
	}
	for py := y; py < y+h; py++ {
		img.SetNRGBA(x, py, rgba(line))
		if !skipRightEdge {
			img.SetNRGBA(x+w-1, py, rgba(line))
		}
	}
}

func rgba(c craft.RGB) color.NRGBA {
	return color.NRGBA{R: c.R, G: c.G, B: c.B, A: c.A}
}

func findingsFor(r *Report, check string) []Finding {
	var out []Finding
	for _, f := range r.Findings {
		if f.Check == check {
			out = append(out, f)
		}
	}
	return out
}

// ---------------------------------------------------------------------------
// The two defects the field report says actually shipped.
// ---------------------------------------------------------------------------

// A floating arm passes every check either reference project has. Only "this
// frame is more than one connected shape" catches it.
func TestDetachedLimbIsReported(t *testing.T) {
	d := newDump(32, 32, "body", "arm")
	body := cel(t, d, 0, 1)
	fillRect(body, 8, 8, 12, 16, craft.MustHex("#404040"))

	arm := cel(t, d, 1, 1)
	// One clear column between the body and the arm: the exact bug.
	fillRect(arm, 21, 10, 4, 6, craft.MustHex("#404040"))

	report := Run(d, Options{Checks: []string{CheckGaps}, Expect: &Expect{SingleComponent: true}})

	found := findingsFor(report, CheckGaps)
	if len(found) != 1 {
		t.Fatalf("expected one gaps finding, got %d: %+v", len(found), report.Findings)
	}
	if found[0].Severity != Error {
		t.Errorf("severity = %s, want error when the plan asks for one shape", found[0].Severity)
	}
	if found[0].Count != 2 {
		t.Errorf("component count = %d, want 2", found[0].Count)
	}
	if report.Passed {
		t.Error("report passed with a detached limb")
	}
}

// Art on the canvas edge is either a deliberate bleed or a clipped move.
// Neither reference project checks it.
func TestClippedAtCanvasEdgeIsReported(t *testing.T) {
	d := newDump(32, 32, "body")
	body := cel(t, d, 0, 1)
	fillRect(body, 10, 0, 8, 20, craft.MustHex("#404040"))

	report := Run(d, Options{Checks: []string{CheckBounds}})

	found := findingsFor(report, CheckBounds)
	if len(found) != 1 {
		t.Fatalf("expected one bounds finding, got %d", len(found))
	}
	if found[0].At == nil || found[0].At.Y != 0 {
		t.Errorf("finding should point at the top row, got %+v", found[0].At)
	}
}

// ---------------------------------------------------------------------------
// Composite scope versus layer scope
// ---------------------------------------------------------------------------

// The false positive that turned a correct rig into warnings: alone, the body's
// edge is dotted with fill where the arm covers it; as a picture, the arm covers
// those pixels and the edge is uniformly outlined.
func TestRiggedSpritePassesCompositeButNotLayerScope(t *testing.T) {
	d := newDump(40, 40, "body", "arm")
	grey := craft.MustHex("#7a7a7a")
	black := craft.MustHex("#000000")

	body := cel(t, d, 0, 1)
	// The right-hand column is left un-outlined because the arm sits on top.
	outlineRect(body, 6, 8, 16, 20, grey, black, true)

	arm := cel(t, d, 1, 1)
	// The arm covers the body's whole un-outlined right edge: interior edges
	// need no outline.
	outlineRect(arm, 20, 8, 8, 20, grey, black, false)

	composite := Run(d, Options{Checks: []string{CheckOutline}, Scope: ScopeComposite})
	if got := findingsFor(composite, CheckOutline); len(got) != 0 {
		t.Errorf("composite scope reported %d outline finding(s) on a correct rig: %+v", len(got), got)
	}

	perLayer := Run(d, Options{Checks: []string{CheckOutline}, Scope: ScopeLayer})
	if got := findingsFor(perLayer, CheckOutline); len(got) == 0 {
		t.Error("layer scope found nothing; the test no longer proves the scope distinction")
	}
}

// ---------------------------------------------------------------------------
// Palette drift
// ---------------------------------------------------------------------------

func TestOffPaletteColoursAreNamed(t *testing.T) {
	d := newDump(16, 16, "art")
	// A green palette, and art in three colours that are not in it.
	d.Palette = []string{"#0d1b0e", "#1d3b20", "#2f5d33", "#4d8061"}
	art := cel(t, d, 0, 1)
	fillRect(art, 2, 2, 4, 4, craft.MustHex("#291c06"))
	fillRect(art, 8, 2, 4, 4, craft.MustHex("#858d0d"))
	fillRect(art, 2, 8, 4, 4, craft.MustHex("#54410b"))

	report := Run(d, Options{Checks: []string{CheckPalette}})

	found := findingsFor(report, CheckPalette)
	if len(found) != 1 {
		t.Fatalf("expected one palette finding, got %d", len(found))
	}
	if found[0].Count != 3 {
		t.Errorf("named %d off-palette colours, want 3", found[0].Count)
	}
	if report.Metrics.OffPalettePixels != 48 {
		t.Errorf("offPalettePixels = %d, want 48", report.Metrics.OffPalettePixels)
	}
}

func TestOnPaletteArtIsClean(t *testing.T) {
	d := newDump(16, 16, "art")
	d.Palette = []string{"#2f5d33"}
	art := cel(t, d, 0, 1)
	fillRect(art, 4, 4, 6, 6, craft.MustHex("#2f5d33"))

	report := Run(d, Options{Checks: []string{CheckPalette}})
	if got := findingsFor(report, CheckPalette); len(got) != 0 {
		t.Errorf("on-palette art reported %d palette finding(s): %+v", len(got), got)
	}
}

// ---------------------------------------------------------------------------
// Structure checks
// ---------------------------------------------------------------------------

func TestUntaggedAnimationIsAnError(t *testing.T) {
	d := newDump(16, 16, "art")
	addFrame(d, 100)
	d.Tags = nil
	fillRect(cel(t, d, 0, 1), 4, 4, 6, 6, craft.MustHex("#888888"))
	fillRect(cel(t, d, 0, 2), 5, 4, 6, 6, craft.MustHex("#888888"))

	report := Run(d, Options{Checks: []string{CheckAnimation}})
	found := findingsFor(report, CheckAnimation)
	if len(found) != 1 || found[0].Severity != Error {
		t.Fatalf("expected one error for untagged frames, got %+v", found)
	}
}

func TestUniformTimingIsANote(t *testing.T) {
	d := newDump(16, 16, "art")
	for i := 0; i < 4; i++ {
		if i > 0 {
			addFrame(d, 100)
		}
		fillRect(cel(t, d, 0, i+1), 4+i, 4, 6, 6, craft.MustHex("#888888"))
	}

	report := Run(d, Options{Checks: []string{CheckAnimation}})
	found := findingsFor(report, CheckAnimation)
	if len(found) != 1 || found[0].Severity != Note {
		t.Fatalf("expected one note about uniform timing, got %+v", found)
	}
}

func TestHalfTransparentPixelsAreReported(t *testing.T) {
	d := newDump(16, 16, "art")
	art := cel(t, d, 0, 1)
	fillRect(art, 4, 4, 6, 6, craft.MustHex("#888888"))
	art.SetNRGBA(6, 6, rgba(craft.RGB{R: 0x88, G: 0x88, B: 0x88, A: 128}))

	report := Run(d, Options{Checks: []string{CheckAlpha}})
	if report.Metrics.HalfAlphaPixels != 1 {
		t.Errorf("halfAlphaPixels = %d, want 1", report.Metrics.HalfAlphaPixels)
	}
	if len(findingsFor(report, CheckAlpha)) != 1 {
		t.Errorf("expected one antialiasing finding, got %+v", report.Findings)
	}
}

func TestStrayPixelIsFound(t *testing.T) {
	d := newDump(16, 16, "art")
	art := cel(t, d, 0, 1)
	fillRect(art, 4, 4, 6, 6, craft.MustHex("#888888"))
	art.SetNRGBA(14, 14, rgba(craft.MustHex("#888888")))

	report := Run(d, Options{Checks: []string{CheckStrays}})
	if report.Metrics.StrayPixels != 1 {
		t.Fatalf("strayPixels = %d, want 1", report.Metrics.StrayPixels)
	}
	if report.Findings[0].Severity != Warning {
		t.Errorf("one stray should be a warning, got %s", report.Findings[0].Severity)
	}
}

// ---------------------------------------------------------------------------
// The plan the caller already knows
// ---------------------------------------------------------------------------

func TestExpectLayerFramesFlagsInkOutsideItsBeat(t *testing.T) {
	d := newDump(16, 16, "base", "fx")
	addFrame(d, 100)
	fillRect(cel(t, d, 0, 1), 4, 4, 6, 6, craft.MustHex("#888888"))
	fillRect(cel(t, d, 0, 2), 4, 4, 6, 6, craft.MustHex("#888888"))
	// The effect layer was planned for frame 2 only, but has ink on frame 1.
	fillRect(cel(t, d, 1, 1), 12, 4, 2, 2, craft.MustHex("#ff0000"))
	fillRect(cel(t, d, 1, 2), 12, 4, 2, 2, craft.MustHex("#ff0000"))

	report := Run(d, Options{
		Checks: []string{CheckPalette},
		Expect: &Expect{LayerFrames: map[string][][2]int{"fx": {{2, 2}}}},
	})

	found := findingsFor(report, CheckExpect)
	if len(found) != 1 {
		t.Fatalf("expected one expect finding, got %+v", found)
	}
	if found[0].Frame != 1 || found[0].Severity != Error {
		t.Errorf("expected an error on frame 1, got %+v", found[0])
	}
}

func TestExpectOverlapIsAnError(t *testing.T) {
	d := newDump(16, 16, "sword", "head")
	img := cel(t, d, 0, 1)
	fillRect(img, 6, 6, 6, 6, craft.MustHex("#aaaaaa"))
	head := cel(t, d, 1, 1)
	fillRect(head, 8, 8, 4, 4, craft.MustHex("#bbbbbb"))

	report := Run(d, Options{
		Checks: []string{CheckPalette},
		Expect: &Expect{MustNotOverlap: [][2]string{{"sword", "head"}}},
	})

	found := findingsFor(report, CheckExpect)
	if len(found) != 1 || found[0].Count != 16 {
		t.Fatalf("expected one overlap finding of 16 pixels, got %+v", found)
	}
}

// A clean sprite must stay clean, or the checker is unusable.
func TestCleanSpriteReportsNothing(t *testing.T) {
	d := newDump(32, 32, "art")
	d.Palette = []string{"#000000", "#555555", "#999999"}
	art := cel(t, d, 0, 1)
	outlineRect(art, 8, 8, 16, 16, craft.MustHex("#555555"), craft.MustHex("#000000"), false)
	fillRect(art, 12, 12, 8, 4, craft.MustHex("#999999"))

	report := Run(d, Options{})
	if !report.Passed {
		t.Fatalf("clean sprite did not pass: %+v", report.Findings)
	}
	if report.Score != 100 {
		t.Errorf("score = %d, want 100", report.Score)
	}
}
