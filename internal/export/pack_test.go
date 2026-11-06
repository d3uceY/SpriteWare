package export

import (
	"encoding/json"
	"image"
	"image/color"
	"image/gif"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/d3uceY/spriteware/internal/doc"
)

func sprite(w, h int, c color.NRGBA) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetNRGBA(x, y, c)
		}
	}
	return img
}

func TestGIFKeepsFrameCountAndTiming(t *testing.T) {
	frames := []*image.NRGBA{
		sprite(4, 4, color.NRGBA{R: 255, A: 255}),
		sprite(4, 4, color.NRGBA{G: 255, A: 255}),
		sprite(4, 4, color.NRGBA{B: 255, A: 255}),
	}
	path := filepath.Join(t.TempDir(), "walk.gif")

	written, err := GIF(frames, []int{120, 80, 40}, path, 2)
	if err != nil {
		t.Fatalf("gif: %v", err)
	}
	if written != path {
		t.Fatalf("wrote %s, want %s", written, path)
	}

	f, err := os.Open(written)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	decoded, err := gif.DecodeAll(f)
	if err != nil {
		t.Fatalf("the file is not a readable GIF: %v", err)
	}
	if len(decoded.Image) != 3 {
		t.Fatalf("decoded %d frames, want 3", len(decoded.Image))
	}
	// Centiseconds, rounded down, never zero.
	want := []int{12, 8, 4}
	for i, d := range want {
		if decoded.Delay[i] != d {
			t.Errorf("frame %d delay = %d, want %d", i+1, decoded.Delay[i], d)
		}
	}
	if got := decoded.Image[0].Bounds().Dx(); got != 8 {
		t.Errorf("scale 2 gave a %dpx wide frame, want 8", got)
	}
}

// A zero GIF delay makes every viewer pick its own speed. Never export one.
func TestGIFNeverWritesAZeroDelay(t *testing.T) {
	frames := []*image.NRGBA{sprite(2, 2, color.NRGBA{A: 255}), sprite(2, 2, color.NRGBA{R: 9, A: 255})}
	path := filepath.Join(t.TempDir(), "fast.gif")

	if _, err := GIF(frames, []int{4, 0}, path, 1); err != nil {
		t.Fatalf("gif: %v", err)
	}
	f, _ := os.Open(path)
	defer f.Close()
	decoded, err := gif.DecodeAll(f)
	if err != nil {
		t.Fatal(err)
	}
	for i, d := range decoded.Delay {
		if d < 1 {
			t.Errorf("frame %d has delay %d", i+1, d)
		}
	}
}

func TestGIFRefusesTooManyColours(t *testing.T) {
	frames := []*image.NRGBA{sprite(300, 1, color.NRGBA{A: 255})}
	img := frames[0]
	for x := 0; x < 300; x++ {
		img.SetNRGBA(x, 0, color.NRGBA{R: uint8(x), G: uint8(x >> 8), A: 255})
	}
	if _, err := GIF(frames, []int{100}, filepath.Join(t.TempDir(), "x.gif"), 1); err == nil {
		t.Fatal("a 300-colour GIF should be refused, not quantised into invented colours")
	}
}

func TestSheetWritesFramesSideBySideWithAnAtlas(t *testing.T) {
	first := sprite(3, 2, color.NRGBA{R: 255, A: 255})
	second := sprite(3, 2, color.NRGBA{G: 255, A: 255})
	tags := []doc.Tag{{Name: "walk", From: 1, To: 2, Direction: "forward"}}
	path := filepath.Join(t.TempDir(), "hero.png")

	pngPath, jsonPath, atlas, err := Sheet(
		[]*image.NRGBA{first, second}, []int{100, 200}, tags, path, 1, 1)
	if err != nil {
		t.Fatalf("sheet: %v", err)
	}
	if pngPath != path || jsonPath != filepath.Join(filepath.Dir(path), "hero.json") {
		t.Fatalf("paths = %s and %s", pngPath, jsonPath)
	}

	if atlas.Width != 3+1+3 || atlas.Height != 2 || atlas.Columns != 2 || atlas.Padding != 1 {
		t.Fatalf("atlas geometry = %+v", atlas)
	}
	if atlas.Frames[1].X != 4 {
		t.Errorf("second frame starts at x=%d, want 4 (3 wide plus 1 padding)", atlas.Frames[1].X)
	}
	if atlas.Frames[0].Index != 1 || atlas.Frames[1].Index != 2 {
		t.Errorf("frame indexes are not 1-based: %+v", atlas.Frames)
	}
	if len(atlas.Tags) != 1 || atlas.Tags[0].Name != "walk" {
		t.Errorf("tags were not carried into the atlas: %+v", atlas.Tags)
	}

	// The atlas must be readable by anything that is not this package.
	raw, err := os.ReadFile(jsonPath)
	if err != nil {
		t.Fatal(err)
	}
	var back Atlas
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatalf("the atlas is not valid JSON: %v", err)
	}
	if back.Image != "hero.png" || len(back.Frames) != 2 {
		t.Errorf("round trip lost data: %+v", back)
	}

	// And the padding really is transparent, or a bleeding GPU still picks up
	// the neighbour's colour.
	sheetFile, err := os.Open(pngPath)
	if err != nil {
		t.Fatal(err)
	}
	defer sheetFile.Close()
	sheet, err := png.Decode(sheetFile)
	if err != nil {
		t.Fatal(err)
	}
	if c := color.NRGBAModel.Convert(sheet.At(3, 0)).(color.NRGBA); c.A != 0 {
		t.Errorf("the 1px gap is %v, want transparent", c)
	}
}

func TestSheetRefusesMixedFrameSizes(t *testing.T) {
	_, _, _, err := Sheet(
		[]*image.NRGBA{sprite(4, 4, color.NRGBA{A: 255}), sprite(4, 5, color.NRGBA{A: 255})},
		nil, nil, filepath.Join(t.TempDir(), "x.png"), 1, 0)
	if err == nil {
		t.Fatal("frames of different sizes cannot share one sheet")
	}
}

func TestSheetScalesFramesAndRecordsIt(t *testing.T) {
	_, _, atlas, err := Sheet(
		[]*image.NRGBA{sprite(4, 4, color.NRGBA{A: 255})},
		nil, nil, filepath.Join(t.TempDir(), "x.png"), 3, 0)
	if err != nil {
		t.Fatal(err)
	}
	if atlas.FrameW != 12 || atlas.Scale != 3 || atlas.Width != 12 {
		t.Errorf("scale 3 was not applied: %+v", atlas)
	}
}
