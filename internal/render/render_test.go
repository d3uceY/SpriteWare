package render

import (
	"image"
	"image/color"
	"strings"
	"testing"

	"github.com/d3uceY/spriteware/internal/craft"
)

func img(w, h int) *image.NRGBA { return image.NewNRGBA(image.Rect(0, 0, w, h)) }

func paint(i *image.NRGBA, x, y int, c craft.RGB) {
	i.SetNRGBA(x, y, color.NRGBA{R: c.R, G: c.G, B: c.B, A: c.A})
}

// A glyph must mean the same colour on every frame and in every crop.
func TestGlyphsAreStableAcrossFrames(t *testing.T) {
	red, blue := craft.MustHex("#ff0000"), craft.MustHex("#0000ff")
	palette := []craft.RGB{red, blue}

	first := img(4, 4)
	paint(first, 0, 0, red)
	paint(first, 1, 0, blue)

	// Same colours, different arrangement.
	second := img(4, 4)
	paint(second, 2, 2, blue)
	paint(second, 3, 2, red)

	a, err := ASCII(first, palette, ASCIIOptions{})
	if err != nil {
		t.Fatalf("ASCII: %v", err)
	}
	b, err := ASCII(second, palette, ASCIIOptions{})
	if err != nil {
		t.Fatalf("ASCII: %v", err)
	}

	for glyph, hex := range a.Legend {
		if b.Legend[glyph] != hex {
			t.Errorf("glyph %q means %s on frame 1 and %s on frame 2", glyph, hex, b.Legend[glyph])
		}
	}
}

func TestASCIIRowsMatchTheCanvas(t *testing.T) {
	palette := []craft.RGB{craft.MustHex("#00ff00")}
	i := img(5, 3)
	paint(i, 0, 0, palette[0])

	grid, err := ASCII(i, palette, ASCIIOptions{Rulers: true})
	if err != nil {
		t.Fatalf("ASCII: %v", err)
	}
	if grid.Width != 5 || grid.Height != 3 {
		t.Fatalf("grid is %dx%d, want 5x3", grid.Width, grid.Height)
	}
	if len(grid.Rows) != 3 {
		t.Fatalf("%d rows, want 3", len(grid.Rows))
	}
	for _, row := range grid.Rows {
		if len([]rune(row)) != 5 {
			t.Errorf("row %q is not 5 characters", row)
		}
	}
	if !strings.Contains(grid.Text, "0123") {
		t.Errorf("rulers are missing or unreadable:\n%s", grid.Text)
	}
	if grid.Rows[1][0] != Transparent {
		t.Errorf("row 1 column 0 should be transparent, got %q", grid.Rows[1][0])
	}
}

// Off-palette colours must be called out, and the same picture must always get
// the same glyphs: a grid is only comparable between reads if it is stable.
func TestOffPaletteColoursAreListed(t *testing.T) {
	palette := []craft.RGB{craft.MustHex("#ff0000")}
	i := img(3, 1)
	paint(i, 0, 0, craft.MustHex("#123456"))
	paint(i, 1, 0, craft.MustHex("#00aa00"))
	paint(i, 2, 0, craft.MustHex("#123456"))

	first, err := ASCII(i, palette, ASCIIOptions{})
	if err != nil {
		t.Fatalf("ASCII: %v", err)
	}
	second, err := ASCII(i, palette, ASCIIOptions{})
	if err != nil {
		t.Fatalf("ASCII: %v", err)
	}

	if len(first.OffPalette) != 2 {
		t.Fatalf("off-palette glyphs = %v, want 2", first.OffPalette)
	}
	if first.Legend[first.OffPalette[0]] != "#00aa00" || first.Legend[first.OffPalette[1]] != "#123456" {
		t.Fatalf("off-palette glyphs are not ordered by colour: %v", first.Legend)
	}
	if first.Rows[0] != second.Rows[0] || len(first.Legend) != len(second.Legend) {
		t.Fatalf("two reads of one picture disagree: %q then %q", first.Rows[0], second.Rows[0])
	}
}

// A region crops the grid and keeps the origin, so a row a caller edits is the
// row it read.
func TestASCIIRegionCropsAndKeepsTheOrigin(t *testing.T) {
	palette := []craft.RGB{craft.MustHex("#00ff00")}
	i := img(8, 4)
	for x := 0; x < 8; x++ {
		paint(i, x, 1, palette[0])
	}

	grid, err := ASCII(i, palette, ASCIIOptions{Region: image.Rect(3, 1, 6, 3)})
	if err != nil {
		t.Fatalf("ASCII: %v", err)
	}
	if grid.Width != 3 || grid.Height != 2 {
		t.Fatalf("grid is %dx%d, want 3x2", grid.Width, grid.Height)
	}
	if grid.OriginX != 3 || grid.OriginY != 1 {
		t.Fatalf("origin is (%d,%d), want (3,1)", grid.OriginX, grid.OriginY)
	}
	if grid.Rows[0][0] == Transparent {
		t.Fatal("the region kept a transparent row it should have cropped to ink")
	}
}

func TestDiffReportsErasedAndChangedPixels(t *testing.T) {
	palette := []craft.RGB{craft.MustHex("#ff0000"), craft.MustHex("#00ff00")}
	before := img(2, 1)
	paint(before, 0, 0, palette[0])
	paint(before, 1, 0, palette[0])

	after := img(2, 1)
	paint(after, 0, 0, palette[1])
	// (1,0) was erased.

	diff, err := Diff(before, after, palette, false)
	if err != nil {
		t.Fatalf("Diff: %v", err)
	}
	if diff.ChangedPixels != 2 {
		t.Errorf("changed = %d, want 2", diff.ChangedPixels)
	}
	if diff.PercentChanged != 100 {
		t.Errorf("percentChanged = %.1f, want 100", diff.PercentChanged)
	}
	if !diff.HasChanges {
		t.Error("HasChanges is false with two changed pixels")
	}
	if !strings.Contains(diff.Text, "-") {
		t.Errorf("the erased pixel is not shown as '-' in:\n%s", diff.Text)
	}
}

func TestDiffOfIdenticalFramesIsEmpty(t *testing.T) {
	palette := []craft.RGB{craft.MustHex("#ff0000")}
	i := img(3, 3)
	paint(i, 1, 1, palette[0])

	diff, err := Diff(i, i, palette, false)
	if err != nil {
		t.Fatalf("Diff: %v", err)
	}
	if diff.ChangedPixels != 0 || diff.HasChanges {
		t.Errorf("identical frames differ by %d pixels", diff.ChangedPixels)
	}
}

func TestFilmstripPlacesFramesSideBySide(t *testing.T) {
	a, b := img(4, 4), img(4, 4)
	paint(a, 0, 0, craft.MustHex("#ff0000"))
	paint(b, 0, 0, craft.MustHex("#0000ff"))

	strip := Filmstrip([]*image.NRGBA{a, b}, 1)
	if strip.Bounds().Dx() != 4+1+4 {
		t.Errorf("strip width = %d, want 9", strip.Bounds().Dx())
	}
	if strip.Bounds().Dy() != 4 {
		t.Errorf("strip height = %d, want 4", strip.Bounds().Dy())
	}
	// The gap is the separator, so frame two starts at x=5.
	if got := strip.NRGBAAt(5, 0); got.B != 0xff {
		t.Errorf("second frame is not where it should be: %v", got)
	}
}

func TestAutoScaleKeepsSpritesReadable(t *testing.T) {
	if got := AutoScale(32, 32, 1024); got != 32 {
		t.Errorf("AutoScale(32,32,1024) = %d, want 32", got)
	}
	if got := AutoScale(512, 512, 1024); got != 2 {
		t.Errorf("AutoScale(512,512,1024) = %d, want 2", got)
	}
	if got := AutoScale(4096, 4096, 1024); got != 1 {
		t.Errorf("a huge canvas should not be downscaled by a whole-number factor; got %d", got)
	}
}

func TestScaleIsNearestNeighbour(t *testing.T) {
	i := img(2, 1)
	paint(i, 0, 0, craft.MustHex("#ff0000"))
	paint(i, 1, 0, craft.MustHex("#0000ff"))

	out := Scale(i, 2)
	if out.Bounds().Dx() != 4 {
		t.Fatalf("width = %d, want 4", out.Bounds().Dx())
	}
	if out.NRGBAAt(1, 0).R != 0xff || out.NRGBAAt(2, 0).B != 0xff {
		t.Error("scaling blended the boundary instead of doubling pixels")
	}
}
