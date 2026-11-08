// Package doc is the in-memory picture of a sprite: layers, frames, tags and the
// pixels on each cel.
//
// It is the boundary between Aseprite and everything else: the Lua script writes
// it, and every check, renderer and exporter reads it, so a broken sprite can be
// analysed with no art program installed.
package doc

import (
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"sort"

	"github.com/d3uceY/spriteware/internal/craft"
)

// Sprite is the header information about the document.
type Sprite struct {
	// ID is the absolute file path, stable across restarts. A short handle is a
	// convenience; this is the identity.
	ID        string `json:"id"`
	Name      string `json:"name"`
	Path      string `json:"path"`
	Width     int    `json:"width"`
	Height    int    `json:"height"`
	ColorMode string `json:"colorMode"`
	// Saved is false for a document that has never been written to disk.
	Saved bool `json:"saved"`
}

// Layer is one layer or group. Groups have no cels of their own.
type Layer struct {
	Name      string `json:"name"`
	Depth     int    `json:"depth"`
	IsGroup   bool   `json:"isGroup"`
	Visible   bool   `json:"visible"`
	Opacity   int    `json:"opacity"`
	BlendMode string `json:"blendMode"`
	Cels      []Cel  `json:"cels"`
}

// Cel is one layer's pixels on one frame, at (X, Y) in sprite coordinates. The
// image may be smaller than the canvas.
type Cel struct {
	Frame  int `json:"frame"`
	X      int `json:"x"`
	Y      int `json:"y"`
	Width  int `json:"width"`
	Height int `json:"height"`
	// PNG is the file name inside the dump directory. Empty for an in-memory cel.
	PNG string `json:"png,omitempty"`

	// Pixels is the decoded image. Filled by Load, or set directly in tests.
	Pixels *image.NRGBA `json:"-"`
}

// Frame carries per-frame timing. DurationMS is milliseconds.
type Frame struct {
	DurationMS int `json:"durationMs"`
}

// Tag is a named range. From and To are 1-based and inclusive, as in Aseprite.
type Tag struct {
	Name      string `json:"name"`
	From      int    `json:"from"`
	To        int    `json:"to"`
	Direction string `json:"direction"`
}

// Box is a rectangle in sprite pixels.
type Box struct {
	X      int `json:"x"`
	Y      int `json:"y"`
	Width  int `json:"width"`
	Height int `json:"height"`
}

// Slice is a named rectangle: a 9-patch region of a UI panel, or a character's
// bounding box. Center is the stretchable middle, present only when the art
// declares one.
type Slice struct {
	Name   string `json:"name"`
	X      int    `json:"x"`
	Y      int    `json:"y"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
	Center *Box   `json:"center,omitempty"`
}

// Dump is a complete, read-only copy of a sprite.
type Dump struct {
	Sprite      Sprite   `json:"sprite"`
	Layers      []Layer  `json:"layers"`
	Frames      []Frame  `json:"frames"`
	Tags        []Tag    `json:"tags"`
	Slices      []Slice  `json:"slices"`
	Palette     []string `json:"palette"`
	ActiveFrame int      `json:"activeFrame"`
	ActiveLayer int      `json:"activeLayer"`
}

// Filter narrows what a dump contains. The zero value means everything.
type Filter struct {
	// Sprite is the document to read, by absolute path. Empty means the active
	// one. The Lua side never sees it; it travels beside the filter as `sprite`.
	Sprite string `json:"-"`
	// Layers are layer names to include. Empty means all.
	Layers []string `json:"layers,omitempty"`
	// Frames are 1-based frames to include. Empty means all.
	Frames []int `json:"frames,omitempty"`
	// Composite asks for one flattened image per frame instead of per-cel
	// pixels, which is far smaller for big sprites.
	Composite bool `json:"composite,omitempty"`
}

// FrameCount returns how many frames the document has.
func (d *Dump) FrameCount() int { return len(d.Frames) }

// LayerNames returns the layer names, bottom to top, groups included.
func (d *Dump) LayerNames() []string {
	out := make([]string, 0, len(d.Layers))
	for _, l := range d.Layers {
		out = append(out, l.Name)
	}
	return out
}

// IndexOfLayer returns the bottom-up index of the first layer with this name, or
// -1. Groups are searchable too.
func (d *Dump) IndexOfLayer(name string) int {
	for i, l := range d.Layers {
		if l.Name == name {
			return i
		}
	}
	return -1
}

// Cel returns the cel on a layer and 1-based frame, or nil.
func (d *Dump) Cel(layerIdx, frame int) *Cel {
	if layerIdx < 0 || layerIdx >= len(d.Layers) {
		return nil
	}
	for i := range d.Layers[layerIdx].Cels {
		if d.Layers[layerIdx].Cels[i].Frame == frame {
			return &d.Layers[layerIdx].Cels[i]
		}
	}
	return nil
}

// Composite draws every visible, non-group layer of one frame into a new
// canvas-sized image, bottom to top.
func (d *Dump) Composite(frame int) *image.NRGBA {
	out := image.NewNRGBA(image.Rect(0, 0, d.Sprite.Width, d.Sprite.Height))
	for i := range d.Layers {
		l := &d.Layers[i]
		if l.IsGroup || !l.Visible {
			continue
		}
		if cel := d.Cel(i, frame); cel != nil {
			drawInto(out, cel)
		}
	}
	return out
}

// LayerImage renders one layer alone, ignoring visibility, so a hidden draft can
// still be inspected.
func (d *Dump) LayerImage(layerIdx, frame int) *image.NRGBA {
	out := image.NewNRGBA(image.Rect(0, 0, d.Sprite.Width, d.Sprite.Height))
	cel := d.Cel(layerIdx, frame)
	if cel == nil {
		return out
	}
	drawInto(out, cel)
	return out
}

func drawInto(dst *image.NRGBA, cel *Cel) {
	if cel.Pixels == nil {
		return
	}
	b := cel.Pixels.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			c := cel.Pixels.NRGBAAt(x, y)
			if c.A == 0 {
				continue
			}
			dst.SetNRGBA(cel.X+x, cel.Y+y, blend(dst.NRGBAAt(cel.X+x, cel.Y+y), c))
		}
	}
}

// blend is straight-alpha source-over on 8-bit channels. Pixel art is usually
// opaque or clear, so predictability matters more than precision.
func blend(dst, src color.NRGBA) color.NRGBA {
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
		v := (float64(s)*sa + float64(d)*da*(1-sa)) / oa
		return uint8(v + 0.5)
	}
	return color.NRGBA{R: mix(src.R, dst.R), G: mix(src.G, dst.G), B: mix(src.B, dst.B), A: uint8(oa*255 + 0.5)}
}

// Colors counts how many pixels use each colour, ignoring transparent ones.
func Colors(img *image.NRGBA) map[craft.RGB]int {
	out := map[craft.RGB]int{}
	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			c := img.NRGBAAt(x, y)
			if c.A == 0 {
				continue
			}
			out[craft.RGB{R: c.R, G: c.G, B: c.B, A: c.A}]++
		}
	}
	return out
}

// Bounds returns the tight box around opaque pixels, and false when the image is
// empty.
func Bounds(img *image.NRGBA) (image.Rectangle, bool) {
	b := img.Bounds()
	minX, minY := b.Max.X, b.Max.Y
	maxX, maxY := b.Min.X-1, b.Min.Y-1
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			if img.NRGBAAt(x, y).A == 0 {
				continue
			}
			if x < minX {
				minX = x
			}
			if y < minY {
				minY = y
			}
			if x > maxX {
				maxX = x
			}
			if y > maxY {
				maxY = y
			}
		}
	}
	if maxX < minX {
		return image.Rectangle{}, false
	}
	return image.Rect(minX, minY, maxX+1, maxY+1), true
}

// PaletteRGB decodes the palette hex strings.
func (d *Dump) PaletteRGB() []craft.RGB {
	out := make([]craft.RGB, 0, len(d.Palette))
	for _, h := range d.Palette {
		if c, err := craft.ParseHex(h); err == nil {
			out = append(out, c)
		}
	}
	return out
}

// ---------------------------------------------------------------------------
// Loading
// ---------------------------------------------------------------------------

// DumpFileName is the metadata file inside a dump directory.
const DumpFileName = "dump.json"

// Load reads a dump directory: DumpFileName plus the PNGs it names.
func Load(dir string) (*Dump, error) {
	raw, err := os.ReadFile(filepath.Join(dir, DumpFileName))
	if err != nil {
		return nil, fmt.Errorf("read dump: %w", err)
	}
	d := &Dump{}
	if err := json.Unmarshal(raw, d); err != nil {
		return nil, fmt.Errorf("decode dump: %w", err)
	}
	for li := range d.Layers {
		for ci := range d.Layers[li].Cels {
			cel := &d.Layers[li].Cels[ci]
			if cel.PNG == "" {
				continue
			}
			img, err := readPNG(filepath.Join(dir, cel.PNG))
			if err != nil {
				return nil, fmt.Errorf("layer %q frame %d: %w", d.Layers[li].Name, cel.Frame, err)
			}
			cel.Pixels = img
			cel.Width = img.Bounds().Dx()
			cel.Height = img.Bounds().Dy()
		}
	}
	return d, nil
}

func readPNG(path string) (*image.NRGBA, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open cel image: %w", err)
	}
	defer f.Close()
	src, err := png.Decode(f)
	if err != nil {
		return nil, fmt.Errorf("decode cel image %s: %w", filepath.Base(path), err)
	}
	return toNRGBA(src), nil
}

func toNRGBA(src image.Image) *image.NRGBA {
	if n, ok := src.(*image.NRGBA); ok {
		return n
	}
	b := src.Bounds()
	out := image.NewNRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	for y := 0; y < b.Dy(); y++ {
		for x := 0; x < b.Dx(); x++ {
			out.Set(x, y, src.At(b.Min.X+x, b.Min.Y+y))
		}
	}
	return out
}

// SortedColors orders colours most used first, then by value, so two runs over
// the same sprite always agree.
func SortedColors(colors map[craft.RGB]int) []craft.RGB {
	out := make([]craft.RGB, 0, len(colors))
	for c := range colors {
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool {
		if colors[out[i]] != colors[out[j]] {
			return colors[out[i]] > colors[out[j]]
		}
		return out[i].Hex() < out[j].Hex()
	})
	return out
}
