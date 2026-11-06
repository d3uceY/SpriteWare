package doc

import (
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/d3uceY/spriteware/internal/craft"
)

func testDump() *Dump {
	d := &Dump{
		Sprite: Sprite{ID: "a.aseprite", Width: 8, Height: 8, ColorMode: "rgb", Saved: true},
		Layers: []Layer{
			{Name: "body", Visible: true, Opacity: 255},
			{Name: "hidden", Visible: false, Opacity: 255},
		},
		Frames: []Frame{{DurationMS: 100}},
	}
	return d
}

func celImage(w, h int, c craft.RGB) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetNRGBA(x, y, color.NRGBA{R: c.R, G: c.G, B: c.B, A: c.A})
		}
	}
	return img
}

func TestCompositeSkipsHiddenLayers(t *testing.T) {
	d := testDump()
	d.Layers[0].Cels = []Cel{{Frame: 1, Width: 8, Height: 8, Pixels: celImage(8, 8, craft.MustHex("#ff0000"))}}
	d.Layers[1].Cels = []Cel{{Frame: 1, Width: 8, Height: 8, Pixels: celImage(8, 8, craft.MustHex("#0000ff"))}}

	got := d.Composite(1).NRGBAAt(0, 0)
	if got.R != 0xff || got.B != 0 {
		t.Errorf("composite = %v; a hidden layer leaked into the picture", got)
	}
}

// A cel at (2,3) must paint at (2,3): not at (0,0), and not twice.
func TestCompositeHonoursCelPosition(t *testing.T) {
	d := testDump()
	d.Layers[0].Cels = []Cel{{Frame: 1, X: 2, Y: 3, Width: 4, Height: 4, Pixels: celImage(4, 4, craft.MustHex("#00ff00"))}}

	img := d.Composite(1)
	if img.NRGBAAt(0, 0).A != 0 {
		t.Error("pixels appeared at the origin; the cel offset was ignored")
	}
	if img.NRGBAAt(2, 3).A == 0 {
		t.Error("the cel's top-left pixel was not drawn at its position")
	}
	if img.NRGBAAt(5, 6).A == 0 {
		t.Error("the cel's bottom-right pixel was not drawn at its position")
	}
	if img.NRGBAAt(6, 7).A != 0 {
		t.Error("the cel bled past its own bounds")
	}
}

func TestBoundsIsTightAndReportsEmpty(t *testing.T) {
	d := testDump()
	img := image.NewNRGBA(image.Rect(0, 0, 8, 8))
	img.SetNRGBA(3, 4, color.NRGBA{R: 9, G: 9, B: 9, A: 255})
	img.SetNRGBA(5, 6, color.NRGBA{R: 9, G: 9, B: 9, A: 255})
	d.Layers[0].Cels = []Cel{{Frame: 1, Width: 8, Height: 8, Pixels: img}}

	rect, ok := Bounds(d.Composite(1))
	if !ok {
		t.Fatal("Bounds reported an empty image")
	}
	if rect.Min.X != 3 || rect.Min.Y != 4 || rect.Max.X != 6 || rect.Max.Y != 7 {
		t.Errorf("bounds = %v, want (3,4)-(6,7)", rect)
	}

	if _, ok := Bounds(image.NewNRGBA(image.Rect(0, 0, 4, 4))); ok {
		t.Error("an empty image reported a bounding box")
	}
}

func TestColorsIgnoresTransparentPixels(t *testing.T) {
	d := testDump()
	img := image.NewNRGBA(image.Rect(0, 0, 4, 4))
	img.SetNRGBA(0, 0, color.NRGBA{R: 1, G: 2, B: 3, A: 255})
	img.SetNRGBA(1, 0, color.NRGBA{R: 1, G: 2, B: 3, A: 255})
	d.Layers[0].Cels = []Cel{{Frame: 1, Width: 4, Height: 4, Pixels: img}}

	counts := Colors(d.Composite(1))
	if len(counts) != 1 {
		t.Fatalf("got %d colours, want 1", len(counts))
	}
	if counts[craft.RGB{R: 1, G: 2, B: 3, A: 255}] != 2 {
		t.Errorf("pixel count = %d, want 2", counts[craft.RGB{R: 1, G: 2, B: 3, A: 255}])
	}
}

// The dump format is the contract with Lua, so a round trip through real files
// is the test that matters.
func TestLoadReadsADumpDirectory(t *testing.T) {
	dir := t.TempDir()

	cel := image.NewNRGBA(image.Rect(0, 0, 4, 4))
	cel.SetNRGBA(0, 0, color.NRGBA{R: 0xff, G: 0, B: 0, A: 255})
	f, err := os.Create(filepath.Join(dir, "cel_1_1.png"))
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := png.Encode(f, cel); err != nil {
		t.Fatalf("encode: %v", err)
	}
	_ = f.Close()

	meta := map[string]any{
		"sprite": map[string]any{
			"id": "hero.aseprite", "name": "hero", "path": "hero.aseprite",
			"width": 4, "height": 4, "colorMode": "rgb", "saved": true,
		},
		"layers": []any{
			map[string]any{
				"name": "body", "depth": 0, "isGroup": false, "visible": true,
				"opacity": 255, "blendMode": "normal",
				"cels": []any{
					map[string]any{"frame": 1, "x": 0, "y": 0, "width": 4, "height": 4, "png": "cel_1_1.png"},
				},
			},
		},
		"frames":      []any{map[string]any{"durationMs": 120}},
		"tags":        []any{},
		"palette":     []any{"#ff0000"},
		"activeFrame": 1,
		"activeLayer": 0,
		"dir":         dir,
	}
	body, _ := json.Marshal(meta)
	if err := os.WriteFile(filepath.Join(dir, DumpFileName), body, 0o644); err != nil {
		t.Fatalf("write dump.json: %v", err)
	}

	d, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if d.Sprite.Width != 4 || len(d.Layers) != 1 || len(d.Frames) != 1 {
		t.Fatalf("dump header did not load: %+v", d.Sprite)
	}
	cel2 := d.Cel(0, 1)
	if cel2 == nil || cel2.Pixels == nil {
		t.Fatal("the cel image did not load")
	}
	if cel2.Pixels.NRGBAAt(0, 0).R != 0xff {
		t.Error("the cel pixels are wrong")
	}
	if d.Frames[0].DurationMS != 120 {
		t.Errorf("duration = %d, want 120", d.Frames[0].DurationMS)
	}
}
