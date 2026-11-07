package export

import (
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/gif"
	"os"
	"path/filepath"

	"github.com/d3uceY/spriteware/internal/doc"
	"github.com/d3uceY/spriteware/internal/render"
)

// ---------------------------------------------------------------------------
// GIF
// ---------------------------------------------------------------------------

// maxGIFColours is 255 entries plus one for transparency. Refusing to guess
// beats averaging two colours into a third that was never in the art.
const maxGIFColours = 255

// GIF writes every frame as an animated GIF and returns the path.
//
// Delays come from the document, so the export matches the timeline. A GIF delay
// is in centiseconds, rounded, and never zero: a zero delay plays at the
// browser's own speed and hides a timing bug.
func GIF(frames []*image.NRGBA, durations []int, path string, scale int) (string, error) {
	if len(frames) == 0 {
		return "", fmt.Errorf("nothing to export: the document has no frames")
	}
	if scale < 1 {
		scale = 1
	}

	scaled := make([]*image.NRGBA, 0, len(frames))
	for _, f := range frames {
		scaled = append(scaled, render.Scale(f, scale))
	}

	palette, lookup, err := gifPalette(scaled)
	if err != nil {
		return "", err
	}

	out := &gif.GIF{
		Image:     make([]*image.Paletted, 0, len(scaled)),
		Delay:     make([]int, 0, len(scaled)),
		LoopCount: 0, // 0 means loop forever, which is what an asset preview wants
	}

	for i, frame := range scaled {
		paletted := image.NewPaletted(frame.Bounds(), palette)
		b := frame.Bounds()
		for y := b.Min.Y; y < b.Max.Y; y++ {
			for x := b.Min.X; x < b.Max.X; x++ {
				c := frame.NRGBAAt(x, y)
				paletted.SetColorIndex(x, y, lookup[c])
			}
		}
		out.Image = append(out.Image, paletted)

		delay := 10
		if i < len(durations) && durations[i] > 0 {
			delay = durations[i] / 10
			if delay < 1 {
				delay = 1
			}
		}
		out.Delay = append(out.Delay, delay)
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", fmt.Errorf("create %s: %w", filepath.Dir(path), err)
	}
	f, err := os.Create(path)
	if err != nil {
		return "", fmt.Errorf("create %s: %w", path, err)
	}
	defer f.Close()

	if err := gif.EncodeAll(f, out); err != nil {
		return "", fmt.Errorf("encode %s: %w", path, err)
	}
	return path, nil
}

// gifPalette builds one palette for the whole animation, so a colour does not
// change meaning between frames.
func gifPalette(frames []*image.NRGBA) (color.Palette, map[color.NRGBA]uint8, error) {
	palette := color.Palette{color.NRGBA{}}
	lookup := map[color.NRGBA]uint8{color.NRGBA{}: 0}
	seen := map[color.NRGBA]bool{}

	for _, frame := range frames {
		b := frame.Bounds()
		for y := b.Min.Y; y < b.Max.Y; y++ {
			for x := b.Min.X; x < b.Max.X; x++ {
				c := frame.NRGBAAt(x, y)
				if seen[c] {
					continue
				}
				seen[c] = true
				if c.A != 0 && len(palette) > maxGIFColours {
					return nil, nil, fmt.Errorf(
						"the art has more than %d colours, which a GIF cannot hold without inventing new ones", maxGIFColours)
				}
				// A GIF has one transparent entry, so anything fully transparent
				// shares index 0; partial alpha is refused earlier.
				if c.A == 0 {
					lookup[c] = 0
					continue
				}
				lookup[c] = uint8(len(palette))
				palette = append(palette, c)
			}
		}
	}

	if len(palette) == 1 {
		return nil, nil, fmt.Errorf("nothing to export: every frame is empty")
	}
	return palette, lookup, nil
}

// ---------------------------------------------------------------------------
// Spritesheet
// ---------------------------------------------------------------------------

// Atlas is the metadata beside a sheet: rectangles, not layout rules, so an
// importer needs no knowledge of how the pack was made.
type Atlas struct {
	Image    string       `json:"image"`
	Width    int          `json:"width"`
	Height   int          `json:"height"`
	FrameW   int          `json:"frameWidth"`
	FrameH   int          `json:"frameHeight"`
	Columns  int          `json:"columns"`
	Padding  int          `json:"padding"`
	Frames   []AtlasCell  `json:"frames"`
	Tags     []AtlasTag   `json:"tags,omitempty"`
	Slices   []AtlasSlice `json:"slices,omitempty"`
	Scale    int          `json:"scale"`
	Comments string       `json:"comments,omitempty"`
	// Sheet is the packed picture. Not part of the JSON: it lets the caller
	// report what the file holds without reading it back.
	Sheet *image.NRGBA `json:"-"`
}

// AtlasCell is one frame's place in the sheet. Index is 1-based, like every
// other index SpriteWare reports.
type AtlasCell struct {
	Index      int `json:"index"`
	X          int `json:"x"`
	Y          int `json:"y"`
	W          int `json:"w"`
	H          int `json:"h"`
	DurationMS int `json:"durationMs,omitempty"`
	// BackgroundPosition shows this cell from a one-frame-wide element:
	// `background-position: <value>`. A CSS sprite needs no other maths.
	BackgroundPosition string `json:"backgroundPosition"`
}

// AtlasTag is one named range, 1-based and inclusive, as the tag tool takes it.
type AtlasTag struct {
	Name      string `json:"name"`
	From      int    `json:"from"`
	To        int    `json:"to"`
	Direction string `json:"direction,omitempty"`
}

// AtlasSlice is a named rectangle carried through from the document, with the
// 9-patch centre when the art declares one.
type AtlasSlice struct {
	Name   string    `json:"name"`
	X      int       `json:"x"`
	Y      int       `json:"y"`
	Width  int       `json:"width"`
	Height int       `json:"height"`
	Center *AtlasBox `json:"center,omitempty"`
	// BorderImage is the CSS shorthand for this 9-patch, at the sheet's own
	// scale: `border-image: url(<image>) top right bottom left fill / ...`.
	BorderImage string `json:"borderImage,omitempty"`
}

// AtlasBox is a rectangle in sheet pixels.
type AtlasBox struct {
	X      int `json:"x"`
	Y      int `json:"y"`
	Width  int `json:"width"`
	Height int `json:"height"`
}

// SheetOptions describes one sheet export. A struct rather than six positional
// arguments, so adding one more thing to carry through does not touch every
// call site.
type SheetOptions struct {
	Frames    []*image.NRGBA
	Durations []int
	Tags      []doc.Tag
	Slices    []doc.Slice
	Path      string
	Scale     int
	Padding   int
}

// Sheet writes the frames into one packed PNG plus a JSON atlas, and returns
// both paths with the atlas.
//
// One row with `padding` transparent pixels between frames. Not decoration: at
// non-integer zoom a GPU samples past a frame's edge into its neighbour, which
// shows as a one-pixel line along the sprite.
func Sheet(opts SheetOptions) (string, string, Atlas, error) {
	frames, durations, tags, path := opts.Frames, opts.Durations, opts.Tags, opts.Path
	scale, padding := opts.Scale, opts.Padding
	if len(frames) == 0 {
		return "", "", Atlas{}, fmt.Errorf("nothing to export: the document has no frames")
	}
	if scale < 1 {
		scale = 1
	}
	if padding < 0 {
		padding = 0
	}

	scaled := make([]*image.NRGBA, 0, len(frames))
	for _, f := range frames {
		scaled = append(scaled, render.Scale(f, scale))
	}

	frameW, frameH := scaled[0].Bounds().Dx(), scaled[0].Bounds().Dy()
	for i, f := range scaled {
		if f.Bounds().Dx() != frameW || f.Bounds().Dy() != frameH {
			return "", "", Atlas{}, fmt.Errorf(
				"frame %d is %dx%d while frame 1 is %dx%d; a sheet needs one frame size, so resize before exporting a sheet",
				i+1, f.Bounds().Dx(), f.Bounds().Dy(), frameW, frameH)
		}
	}

	columns := len(scaled)
	sheetW := columns*frameW + padding*(columns-1)
	sheet := image.NewNRGBA(image.Rect(0, 0, sheetW, frameH))

	atlas := Atlas{
		Image:   filepath.Base(path),
		Width:   sheetW,
		Height:  frameH,
		FrameW:  frameW,
		FrameH:  frameH,
		Columns: columns,
		Padding: padding,
		Scale:   scale,
	}

	for i, frame := range scaled {
		x := i * (frameW + padding)
		blit(sheet, frame, x, 0)

		cell := AtlasCell{Index: i + 1, X: x, Y: 0, W: frameW, H: frameH,
			BackgroundPosition: fmt.Sprintf("-%dpx -%dpx", x, 0)}
		if i < len(durations) {
			cell.DurationMS = durations[i]
		}
		atlas.Frames = append(atlas.Frames, cell)
	}
	for _, t := range tags {
		atlas.Tags = append(atlas.Tags, AtlasTag{Name: t.Name, From: t.From, To: t.To, Direction: t.Direction})
	}
	for _, s := range opts.Slices {
		entry := AtlasSlice{
			Name: s.Name, X: s.X * scale, Y: s.Y * scale,
			Width: s.Width * scale, Height: s.Height * scale,
			Center: scaleBox(s.Center, scale),
		}
		if s.Center != nil {
			entry.BorderImage = borderImage(filepath.Base(path), s, scale)
		}
		atlas.Slices = append(atlas.Slices, entry)
	}

	atlas.Sheet = sheet

	pngPath, err := PNG(sheet, path, 1)
	if err != nil {
		return "", "", Atlas{}, err
	}

	jsonPath := jsonPathFor(path)
	body, err := json.MarshalIndent(atlas, "", "  ")
	if err != nil {
		return "", "", Atlas{}, fmt.Errorf("encode the atlas: %w", err)
	}
	if err := os.WriteFile(jsonPath, append(body, '\n'), 0o644); err != nil {
		return "", "", Atlas{}, fmt.Errorf("write %s: %w", jsonPath, err)
	}
	return pngPath, jsonPath, atlas, nil
}

// scaleBox keeps a slice's 9-patch centre in the same space as the sheet.
func scaleBox(b *doc.Box, scale int) *AtlasBox {
	if b == nil {
		return nil
	}
	return &AtlasBox{X: b.X * scale, Y: b.Y * scale, Width: b.Width * scale, Height: b.Height * scale}
}

// borderImage writes the shorthand a browser needs for a 9-patch.
//
// The four slice values are source pixels and the four lengths are the rendered
// border, so at the sheet's own scale the two are equal. A consumer drawing the
// panel larger multiplies only the lengths.
func borderImage(image string, s doc.Slice, scale int) string {
	c := s.Center
	left := c.X * scale
	top := c.Y * scale
	right := (s.Width - c.X - c.Width) * scale
	bottom := (s.Height - c.Y - c.Height) * scale
	for _, v := range []int{left, top, right, bottom} {
		if v < 0 {
			return ""
		}
	}
	return fmt.Sprintf("border-image: url(%s) %d %d %d %d fill / %dpx %dpx %dpx %dpx",
		image, top, right, bottom, left, top, right, bottom, left)
}

func jsonPathFor(pngPath string) string {
	ext := filepath.Ext(pngPath)
	if ext == "" {
		return pngPath + ".json"
	}
	return pngPath[:len(pngPath)-len(ext)] + ".json"
}

// blit copies src into dst with its top-left at (x, y).
func blit(dst, src *image.NRGBA, x, y int) {
	b := src.Bounds()
	for sy := b.Min.Y; sy < b.Max.Y; sy++ {
		for sx := b.Min.X; sx < b.Max.X; sx++ {
			c := src.NRGBAAt(sx, sy)
			if c.A == 0 {
				continue
			}
			dst.SetNRGBA(x+sx-b.Min.X, y+sy-b.Min.Y, c)
		}
	}
}
