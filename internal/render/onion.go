package render

import (
	"image"
	"image/color"
)

// Onion composites one frame at full strength over its neighbours, ghosted.
//
// Otherwise judging an in-between means flipping between two images. Here the
// neighbours show where the shape came from and where it is going.
//
// index is 0-based into frames. With a single frame the result is that frame.
func Onion(frames []*image.NRGBA, index int) *image.NRGBA {
	if len(frames) == 0 {
		return image.NewNRGBA(image.Rect(0, 0, 1, 1))
	}
	if index < 0 || index >= len(frames) {
		index = 0
	}

	b := frames[index].Bounds()
	out := image.NewNRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))

	// Oldest first, so the nearest neighbour sits on top of the older one.
	if index-1 >= 0 {
		ghost(out, frames[index-1], 0x40, b)
	}
	if index+1 < len(frames) {
		ghost(out, frames[index+1], 0x40, b)
	}
	ghost(out, frames[index], 0xFF, b)
	return out
}

// ghost paints src over dst at alpha/255, ignoring anything fully transparent.
//
// Not over(), which forces the result opaque: a ghost has to stay see-through,
// or the current frame cannot be told from its neighbours.
func ghost(dst, src *image.NRGBA, alpha uint8, bounds image.Rectangle) {
	sb := src.Bounds()
	for y := 0; y < bounds.Dy(); y++ {
		for x := 0; x < bounds.Dx(); x++ {
			if y+sb.Min.Y >= sb.Max.Y || x+sb.Min.X >= sb.Max.X || sb.Min.Y+y < 0 || sb.Min.X+x < 0 {
				continue
			}
			c := src.NRGBAAt(sb.Min.X+x, sb.Min.Y+y)
			if c.A == 0 {
				continue
			}
			c.A = alpha
			dst.SetNRGBA(x, y, blend(c, dst.NRGBAAt(x, y)))
		}
	}
}

// blend is source-over that keeps the result's alpha, so two ghosts layer darker
// instead of turning opaque.
func blend(src, dst color.NRGBA) color.NRGBA {
	if src.A == 0 {
		return dst
	}
	if dst.A == 0 {
		return src
	}
	sa := float64(src.A) / 255
	da := float64(dst.A) / 255
	outA := sa + da*(1-sa)
	if outA == 0 {
		return color.NRGBA{}
	}
	mix := func(s, d uint8) uint8 {
		return uint8((float64(s)*sa+float64(d)*da*(1-sa))/outA + 0.5)
	}
	return color.NRGBA{R: mix(src.R, dst.R), G: mix(src.G, dst.G), B: mix(src.B, dst.B), A: uint8(outA*255 + 0.5)}
}
