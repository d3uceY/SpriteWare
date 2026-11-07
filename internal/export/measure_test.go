package export

import (
	"image"
	"image/color"
	"strings"
	"testing"

	"github.com/d3uceY/spriteware/internal/doc"
)

func frameWith(pixels map[[2]int]color.NRGBA) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, 4, 4))
	for at, c := range pixels {
		img.SetNRGBA(at[0], at[1], c)
	}
	return img
}

// Measure unions the frames, so a sheet and a GIF describe their whole content.
func TestMeasureUnionsFrames(t *testing.T) {
	red := color.NRGBA{R: 255, A: 255}
	blue := color.NRGBA{B: 255, A: 255}

	first := frameWith(map[[2]int]color.NRGBA{{0, 0}: red})
	second := frameWith(map[[2]int]color.NRGBA{{1, 1}: blue})

	stats := Measure(first, second)
	if stats.Width != 4 || stats.Height != 4 {
		t.Fatalf("size is %dx%d, want 4x4", stats.Width, stats.Height)
	}
	if stats.OpaquePixels != 2 {
		t.Fatalf("opaque pixels = %d, want 2", stats.OpaquePixels)
	}
	if stats.ColourCount != 2 || len(stats.Colours) != 2 {
		t.Fatalf("colours = %v, want two", stats.Colours)
	}
	if stats.Colours[0] != "#0000ff" || stats.Colours[1] != "#ff0000" {
		t.Fatalf("colours are not sorted by hex: %v", stats.Colours)
	}
}

func TestMeasureCountsAPixelOnceWhenTwoFramesPaintIt(t *testing.T) {
	red := color.NRGBA{R: 255, A: 255}
	stats := Measure(
		frameWith(map[[2]int]color.NRGBA{{2, 2}: red}),
		frameWith(map[[2]int]color.NRGBA{{2, 2}: red}),
	)
	if stats.OpaquePixels != 1 {
		t.Fatalf("opaque pixels = %d, want 1: two frames painting one pixel is one pixel of ink", stats.OpaquePixels)
	}
}

// A sheet carries the CSS a browser needs: a background position per frame and a
// border-image per 9-patch slice.
func TestSheetCarriesCSSForFramesAndSlices(t *testing.T) {
	red := color.NRGBA{R: 255, A: 255}
	frames := []*image.NRGBA{
		frameWith(map[[2]int]color.NRGBA{{0, 0}: red}),
		frameWith(map[[2]int]color.NRGBA{{1, 0}: red}),
	}
	slice := doc.Slice{
		Name: "panel", Width: 4, Height: 4,
		Center: &doc.Box{X: 1, Y: 1, Width: 2, Height: 2},
	}

	pngPath, _, atlas, err := Sheet(SheetOptions{
		Frames:  frames,
		Path:    t.TempDir() + "/sheet.png",
		Padding: 0,
		Scale:   1,
		Slices:  []doc.Slice{slice},
	})
	if err != nil {
		t.Fatalf("Sheet: %v", err)
	}
	if !strings.HasSuffix(pngPath, "sheet.png") {
		t.Fatalf("png path is %q", pngPath)
	}

	if len(atlas.Frames) != 2 {
		t.Fatalf("%d frames in the atlas, want 2", len(atlas.Frames))
	}
	if atlas.Frames[0].BackgroundPosition != "-0px -0px" {
		t.Errorf("frame 1 background-position = %q", atlas.Frames[0].BackgroundPosition)
	}
	if atlas.Frames[1].BackgroundPosition != "-4px -0px" {
		t.Errorf("frame 2 background-position = %q, want a 4px step with no padding", atlas.Frames[1].BackgroundPosition)
	}

	if len(atlas.Slices) != 1 {
		t.Fatalf("%d slices, want 1", len(atlas.Slices))
	}
	want := "border-image: url(sheet.png) 1 1 1 1 fill / 1px 1px 1px 1px"
	if atlas.Slices[0].BorderImage != want {
		t.Errorf("border-image = %q, want %q", atlas.Slices[0].BorderImage, want)
	}

	if atlas.Sheet == nil {
		t.Fatal("the atlas does not carry the packed sheet, so the export cannot report what it wrote")
	}
	if stats := Measure(atlas.Sheet); stats.OpaquePixels != 2 {
		t.Fatalf("the sheet holds %d opaque pixels, want 2", stats.OpaquePixels)
	}
}
