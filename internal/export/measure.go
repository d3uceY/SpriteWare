package export

import (
	"image"
	"sort"

	"github.com/d3uceY/spriteware/internal/craft"
)

// Stats describes what a picture holds, so an export can be checked without
// opening the file again.
type Stats struct {
	Width        int      `json:"width"`
	Height       int      `json:"height"`
	OpaquePixels int      `json:"opaquePixels"`
	ColourCount  int      `json:"colourCount"`
	Colours      []string `json:"colours"`
}

// Measure reports the size, the ink and the colours of an export.
//
// Several frames are measured as one picture: a pixel counts as opaque if any
// frame paints it, and the colours are the union. That answers "did the file get
// the right sprite" for a sheet and a GIF as well as for a PNG.
func Measure(frames ...*image.NRGBA) Stats {
	out := Stats{Colours: []string{}}
	if len(frames) == 0 || frames[0] == nil {
		return out
	}

	first := frames[0].Bounds()
	w, h := first.Dx(), first.Dy()
	out.Width, out.Height = w, h

	opaque := make([]bool, w*h)
	colours := map[craft.RGB]bool{}
	for _, f := range frames {
		if f == nil {
			continue
		}
		b := f.Bounds()
		for y := 0; y < min(h, b.Dy()); y++ {
			for x := 0; x < min(w, b.Dx()); x++ {
				c := f.NRGBAAt(b.Min.X+x, b.Min.Y+y)
				if c.A == 0 {
					continue
				}
				opaque[y*w+x] = true
				colours[craft.RGB{R: c.R, G: c.G, B: c.B, A: c.A}] = true
			}
		}
	}

	for _, on := range opaque {
		if on {
			out.OpaquePixels++
		}
	}
	out.ColourCount = len(colours)
	hexes := make([]string, 0, len(colours))
	for c := range colours {
		hexes = append(hexes, c.Hex())
	}
	sort.Strings(hexes)
	out.Colours = hexes
	return out
}
