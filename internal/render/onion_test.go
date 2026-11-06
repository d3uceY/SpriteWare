package render

import (
	"image"
	"image/color"
	"testing"
)

func frame(w, h int, c color.NRGBA) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetNRGBA(x, y, c)
		}
	}
	return img
}

func TestOnionGhostsTheNeighboursUnderTheCurrentFrame(t *testing.T) {
	prev := frame(4, 4, color.NRGBA{R: 255, A: 255})
	cur := frame(4, 4, color.NRGBA{G: 255, A: 255})
	next := frame(4, 4, color.NRGBA{B: 255, A: 255})

	out := Onion([]*image.NRGBA{prev, cur, next}, 1)
	if b := out.Bounds(); b.Dx() != 4 || b.Dy() != 4 {
		t.Fatalf("bounds = %v", b)
	}

	// The current frame is painted last and is opaque, so it wins outright.
	if c := out.NRGBAAt(0, 0); c != cur.NRGBAAt(0, 0) {
		t.Errorf("the current frame is not on top: %v", c)
	}
}

func TestOnionShowsNeighbourWhereTheCurrentFrameIsEmpty(t *testing.T) {
	prev := frame(4, 4, color.NRGBA{R: 255, A: 255})
	empty := image.NewNRGBA(image.Rect(0, 0, 4, 4))

	out := Onion([]*image.NRGBA{prev, empty}, 1)
	c := out.NRGBAAt(0, 0)
	if c.A == 0 {
		t.Fatal("the ghost neighbour did not appear in the empty area")
	}
	if c.A == 255 {
		t.Errorf("the neighbour is drawn at full strength, so it cannot be told apart from the current frame: %v", c)
	}
	if c.R == 0 {
		t.Errorf("the ghost lost its colour: %v", c)
	}
}

func TestOnionHandlesTheEdgesAndBadIndexes(t *testing.T) {
	a := frame(2, 2, color.NRGBA{R: 10, A: 255})
	b := frame(2, 2, color.NRGBA{R: 20, A: 255})

	// First frame: only the frame after it exists.
	first := Onion([]*image.NRGBA{a, b}, 0)
	if c := first.NRGBAAt(0, 0); c.R != 10 {
		t.Errorf("first frame is not on top: %v", c)
	}

	// Out-of-range and empty input must not panic or return nil.
	if out := Onion([]*image.NRGBA{a}, 7); out == nil || out.Bounds().Dx() != 2 {
		t.Error("an out-of-range index should fall back to the first frame")
	}
	if out := Onion(nil, 0); out == nil {
		t.Error("no frames should still return an image")
	}
}

func TestOnionKeepsFramesOfDifferentSizesInsideTheFirst(t *testing.T) {
	cur := frame(4, 4, color.NRGBA{G: 100, A: 255})
	bigger := frame(8, 8, color.NRGBA{R: 200, A: 255})

	out := Onion([]*image.NRGBA{bigger, cur}, 1)
	if b := out.Bounds(); b.Dx() != 4 || b.Dy() != 4 {
		t.Fatalf("bounds = %v, want the current frame's size", b)
	}
	if c := out.NRGBAAt(3, 3); c.G != 100 {
		t.Errorf("the current frame does not cover its own bounds: %v", c)
	}
}
