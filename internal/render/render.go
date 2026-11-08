// Package render turns a document into what an agent can look at: an upscaled
// picture, an exact text grid, a filmstrip, a diff.
package render

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"sort"
	"strings"
	"unicode"

	"github.com/d3uceY/spriteware/internal/craft"
	"github.com/d3uceY/spriteware/internal/doc"
)

// Transparent is the glyph for "nothing here", never assigned to a colour.
const Transparent = '.'

// glyphPoolSize is how many distinct glyphs the pool holds. A reused glyph would
// misreport which colour is where, so the renderer always takes the next one.
const glyphPoolSize = 261

var glyphPoolCache []rune

func glyphPool() []rune {
	if glyphPoolCache != nil {
		return glyphPoolCache
	}
	out := make([]rune, 0, glyphPoolSize)
	// Printable Latin, skipping characters that read as blank in a terminal.
	for r := rune(0x21); r < 0x024F && len(out) < glyphPoolSize; r++ {
		if r == Transparent || unicode.IsSpace(r) || unicode.IsControl(r) {
			continue
		}
		out = append(out, r)
	}
	glyphPoolCache = out
	return out
}

// Grid is a text form of a picture, written so it can be read and edited.
type Grid struct {
	Width   int
	Height  int
	OriginX int
	OriginY int
	// Text is the rendering with rulers, for reading.
	Text string
	// Rows are the bare rows: exactly what `draw` kind=grid takes back.
	Rows   []string
	Legend map[string]string
	// OffPalette lists glyphs bound to colours outside the palette. The order is
	// by hex, so the same picture always renders the same glyphs.
	OffPalette []string
}

// ASCIIOptions narrows what a grid covers.
type ASCIIOptions struct {
	// Rulers adds the coordinate labels. Off leaves rows that `draw` kind=grid
	// takes back unchanged.
	Rulers bool
	// Region crops the grid to this rectangle. Empty means the whole image.
	Region image.Rectangle
}

// ASCII renders an image one character per pixel.
//
// Glyphs bind to palette entries, not to colours in this image, so B is the same
// colour on every frame. Off-palette colours are ordered by hex, so an unchanged
// picture renders identically and two grids compare line by line.
func ASCII(img *image.NRGBA, palette []craft.RGB, opts ASCIIOptions) (Grid, error) {
	b := img.Bounds()
	if !opts.Region.Empty() {
		b = b.Intersect(opts.Region)
	}
	w, h := b.Dx(), b.Dy()
	if w == 0 || h == 0 {
		return Grid{}, fmt.Errorf("nothing to render: the region holds no pixels")
	}

	glyphFor := map[craft.RGB]rune{}
	legend := map[string]string{}
	offPalette := []string{}
	pool := glyphPool()

	// Palette-bound colours claim their glyph first, so an off-palette colour
	// can never take a glyph that already means something.
	off := make([]craft.RGB, 0, 8)
	seen := map[craft.RGB]bool{}
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			c := img.NRGBAAt(x, y)
			if c.A == 0 {
				continue
			}
			rgb := craft.RGB{R: c.R, G: c.G, B: c.B, A: c.A}
			if seen[rgb] {
				continue
			}
			seen[rgb] = true
			idx, _ := craft.Nearest(rgb, palette)
			if idx >= 0 && idx < len(pool) && palette[idx] == rgb {
				glyphFor[rgb] = pool[idx]
				legend[string(pool[idx])] = rgb.Hex()
				continue
			}
			off = append(off, rgb)
		}
	}

	sort.Slice(off, func(i, j int) bool { return off[i].Hex() < off[j].Hex() })

	used := map[rune]bool{}
	for _, g := range glyphFor {
		used[g] = true
	}
	next := len(pool) / 2
	for _, rgb := range off {
		for next < len(pool) && used[pool[next]] {
			next++
		}
		if next >= len(pool) {
			return Grid{}, fmt.Errorf("too many colours: %d palette entries plus %d off-palette leaves no glyphs free; use a smaller region",
				len(palette), len(offPalette)+1)
		}
		g := pool[next]
		next++
		used[g] = true
		glyphFor[rgb] = g
		legend[string(g)] = rgb.Hex()
		offPalette = append(offPalette, string(g))
	}

	rows := make([]string, 0, h)
	for y := b.Min.Y; y < b.Max.Y; y++ {
		var sb strings.Builder
		for x := b.Min.X; x < b.Max.X; x++ {
			c := img.NRGBAAt(x, y)
			if c.A == 0 {
				sb.WriteRune(Transparent)
				continue
			}
			sb.WriteRune(glyphFor[craft.RGB{R: c.R, G: c.G, B: c.B, A: c.A}])
		}
		rows = append(rows, sb.String())
	}

	grid := Grid{
		Width:      w,
		Height:     h,
		OriginX:    b.Min.X,
		OriginY:    b.Min.Y,
		Rows:       rows,
		Legend:     legend,
		OffPalette: offPalette,
	}
	if opts.Rulers {
		grid.Text = withRulers(rows, b.Min.X, b.Min.Y)
	} else {
		grid.Text = strings.Join(rows, "\n")
	}
	return grid, nil
}

// withRulers adds the coordinate labels: the number on the ruler is the number
// the caller types.
func withRulers(rows []string, originX, originY int) string {
	width := 0
	if len(rows) > 0 {
		width = len([]rune(rows[0]))
	}
	gutter := len(fmt.Sprintf("%d", originY+len(rows))) + 1

	var sb strings.Builder

	// Two header rows keep a 40-wide grid readable: tens where the decade
	// changes, then every unit digit.
	sb.WriteString(strings.Repeat(" ", gutter))
	sb.WriteString(tensRow(width, originX))
	sb.WriteByte('\n')

	sb.WriteString(strings.Repeat(" ", gutter))
	sb.WriteString(unitsRow(width, originX))

	for i, row := range rows {
		sb.WriteByte('\n')
		sb.WriteString(fmt.Sprintf("%*d ", gutter-1, originY+i))
		sb.WriteString(row)
	}
	return sb.String()
}

// tensRow shows a digit only where the decade changes.
func tensRow(width, originX int) string {
	out := make([]rune, width)
	last := -1
	for i := 0; i < width; i++ {
		digit := ((originX + i) / 10) % 10
		if digit != last {
			out[i] = rune('0' + digit)
			last = digit
			continue
		}
		out[i] = ' '
	}
	return string(out)
}

func unitsRow(width, originX int) string {
	out := make([]rune, width)
	for i := 0; i < width; i++ {
		out[i] = rune('0' + (originX+i)%10)
	}
	return string(out)
}

// ---------------------------------------------------------------------------
// Images
// ---------------------------------------------------------------------------

// Crop copies r out of src into a new image whose origin is (0,0), so callers
// that scale or checkerboard it never see an offset.
func Crop(src *image.NRGBA, r image.Rectangle) *image.NRGBA {
	r = r.Intersect(src.Bounds())
	if r.Empty() {
		return image.NewNRGBA(image.Rect(0, 0, 0, 0))
	}
	out := image.NewNRGBA(image.Rect(0, 0, r.Dx(), r.Dy()))
	for y := 0; y < r.Dy(); y++ {
		for x := 0; x < r.Dx(); x++ {
			out.SetNRGBA(x, y, src.NRGBAAt(r.Min.X+x, r.Min.Y+y))
		}
	}
	return out
}

// Scale enlarges by a whole number with nearest-neighbour, so a pixel stays a
// pixel; fractional scaling would blur the information being checked.
func Scale(src *image.NRGBA, factor int) *image.NRGBA {
	if factor < 1 {
		factor = 1
	}
	b := src.Bounds()
	out := image.NewNRGBA(image.Rect(0, 0, b.Dx()*factor, b.Dy()*factor))
	for y := 0; y < b.Dy()*factor; y++ {
		for x := 0; x < b.Dx()*factor; x++ {
			out.SetNRGBA(x, y, src.NRGBAAt(b.Min.X+x/factor, b.Min.Y+y/factor))
		}
	}
	return out
}

// AutoScale picks the whole-number zoom nearest the target long edge: 1x is
// unreadable for a vision model, and past ~1024 px is wasted tokens.
func AutoScale(w, h, target int) int {
	longest := w
	if h > longest {
		longest = h
	}
	if longest <= 0 {
		return 1
	}
	factor := target / longest
	if factor < 1 {
		factor = 1
	}
	if factor > 64 {
		factor = 64
	}
	return factor
}

// WithChecker puts a light checkerboard behind transparency, so "empty" differs
// visibly from "white".
func WithChecker(src *image.NRGBA, cell int) *image.NRGBA {
	if cell < 2 {
		cell = 4
	}
	b := src.Bounds()
	out := image.NewNRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	const light, dark = 0xC8, 0xA8
	for y := 0; y < b.Dy(); y++ {
		for x := 0; x < b.Dx(); x++ {
			srcPixel := src.NRGBAAt(b.Min.X+x, b.Min.Y+y)
			if srcPixel.A == 255 {
				out.SetNRGBA(x, y, srcPixel)
				continue
			}
			shade := uint8(light)
			if ((x/cell)+(y/cell))%2 == 1 {
				shade = dark
			}
			under := color.NRGBA{R: shade, G: shade, B: shade, A: 255}
			out.SetNRGBA(x, y, over(srcPixel, under))
		}
	}
	return out
}

func over(src, dst color.NRGBA) color.NRGBA {
	if src.A == 0 {
		return dst
	}
	if src.A == 255 {
		return src
	}
	sa := float64(src.A) / 255
	da := float64(dst.A) / 255
	oa := sa + da*(1-sa)
	if oa == 0 {
		return color.NRGBA{}
	}
	mix := func(s, d uint8) uint8 {
		return uint8((float64(s)*sa+float64(d)*da*(1-sa))/oa + 0.5)
	}
	return color.NRGBA{R: mix(src.R, dst.R), G: mix(src.G, dst.G), B: mix(src.B, dst.B), A: 255}
}

// Filmstrip lays frames side by side with a one-pixel separator. A grid of frames
// cannot show a cycle that wraps onto a second row, and a vision model reads only
// a GIF's first frame.
func Filmstrip(frames []*image.NRGBA, gap int) *image.NRGBA {
	if len(frames) == 0 {
		return image.NewNRGBA(image.Rect(0, 0, 1, 1))
	}
	if gap < 1 {
		gap = 1
	}
	width := 0
	height := 0
	for _, f := range frames {
		width += f.Bounds().Dx()
		if f.Bounds().Dy() > height {
			height = f.Bounds().Dy()
		}
	}
	width += gap * (len(frames) - 1)

	out := image.NewNRGBA(image.Rect(0, 0, width, height))
	// Separators in the accent-free grey, so they read as structure.
	sep := color.NRGBA{R: 0x30, G: 0x36, B: 0x42, A: 255}
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			out.SetNRGBA(x, y, sep)
		}
	}

	x := 0
	for _, f := range frames {
		b := f.Bounds()
		for y := 0; y < b.Dy(); y++ {
			for px := 0; px < b.Dx(); px++ {
				c := f.NRGBAAt(b.Min.X+px, b.Min.Y+y)
				if c.A == 0 {
					continue
				}
				out.SetNRGBA(x+px, y, c)
			}
		}
		x += b.Dx() + gap
	}
	return out
}

// Bounds is re-exported so callers do not need two imports for one question.
func Bounds(img *image.NRGBA) (image.Rectangle, bool) { return doc.Bounds(img) }

// ---------------------------------------------------------------------------
// Diff
// ---------------------------------------------------------------------------

// DiffResult is what changed between two frames.
type DiffResult struct {
	Text           string
	ChangedPixels  int
	TotalPixels    int
	PercentChanged float64
	ChangedBounds  image.Rectangle
	HasChanges     bool
}

// Diff renders a text diff: unchanged cells '.', erased '-', everything else the
// glyph of the new colour.
func Diff(before, after *image.NRGBA, palette []craft.RGB, showRulers bool) (DiffResult, error) {
	beforeGrid, err := ASCII(before, palette, ASCIIOptions{})
	if err != nil {
		return DiffResult{}, err
	}
	afterGrid, err := ASCII(after, palette, ASCIIOptions{})
	if err != nil {
		return DiffResult{}, err
	}

	rows := make([]string, 0, afterGrid.Height)
	changed := 0
	minX, minY := math.MaxInt, math.MaxInt
	maxX, maxY := -1, -1

	for y := 0; y < afterGrid.Height; y++ {
		beforeRow := []rune(beforeGrid.Rows[y])
		afterRow := []rune(afterGrid.Rows[y])
		var sb strings.Builder
		for x := 0; x < afterGrid.Width; x++ {
			oldCh := rune(Transparent)
			if x < len(beforeRow) {
				oldCh = beforeRow[x]
			}
			newCh := rune(Transparent)
			if x < len(afterRow) {
				newCh = afterRow[x]
			}
			switch {
			case oldCh == newCh:
				sb.WriteRune(Transparent)
			case newCh == Transparent:
				sb.WriteByte('-')
				changed++
				minX, minY = min2(minX, x), min2(minY, y)
				maxX, maxY = max2(maxX, x), max2(maxY, y)
			default:
				sb.WriteRune(newCh)
				changed++
				minX, minY = min2(minX, x), min2(minY, y)
				maxX, maxY = max2(maxX, x), max2(maxY, y)
			}
		}
		rows = append(rows, sb.String())
	}

	total := afterGrid.Width * afterGrid.Height
	result := DiffResult{
		ChangedPixels: changed,
		TotalPixels:   total,
		HasChanges:    changed > 0,
	}
	if total > 0 {
		result.PercentChanged = float64(changed) / float64(total) * 100
	}
	if changed > 0 {
		result.ChangedBounds = image.Rect(minX, minY, maxX+1, maxY+1)
	}
	if showRulers {
		result.Text = withRulers(rows, afterGrid.OriginX, afterGrid.OriginY)
	} else {
		result.Text = strings.Join(rows, "\n")
	}
	return result, nil
}

func min2(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max2(a, b int) int {
	if a > b {
		return a
	}
	return b
}
