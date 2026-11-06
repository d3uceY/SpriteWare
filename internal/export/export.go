// Package export writes a document out as files a game project can consume.
//
// It works from the dump, not from Aseprite, so an export never depends on the
// editor's state and every format is unit-testable.
package export

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/d3uceY/spriteware/internal/craft"
	"github.com/d3uceY/spriteware/internal/render"
)

// PNG writes one scaled image and returns the path it actually wrote.
func PNG(img *image.NRGBA, path string, scale int) (string, error) {
	if scale < 1 {
		scale = 1
	}
	out := img
	if scale > 1 {
		out = render.Scale(img, scale)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", fmt.Errorf("create %s: %w", filepath.Dir(path), err)
	}
	f, err := os.Create(path)
	if err != nil {
		return "", fmt.Errorf("create %s: %w", path, err)
	}
	defer f.Close()
	if err := png.Encode(f, out); err != nil {
		return "", fmt.Errorf("encode %s: %w", path, err)
	}
	return path, nil
}

// Frames writes one PNG per frame using pattern, where {frame} is a 0-based
// index and {n} a 1-based one.
//
// It returns the concrete paths: an echoed pattern is why an agent cannot tell
// whether an export worked.
func Frames(frames []*image.NRGBA, dir, base string, scale int) ([]string, error) {
	if base == "" {
		base = "frame"
	}
	written := make([]string, 0, len(frames))
	for i, img := range frames {
		name := base
		name = strings.ReplaceAll(name, "{frame}", fmt.Sprintf("%d", i))
		name = strings.ReplaceAll(name, "{n}", fmt.Sprintf("%d", i+1))
		if name == base {
			name = fmt.Sprintf("%s%d", base, i)
		}
		path, err := PNG(img, filepath.Join(dir, name+".png"), scale)
		if err != nil {
			return written, err
		}
		written = append(written, path)
	}
	return written, nil
}

// SVG emits one <rect> per run of identical pixels, which is the shape pixel art
// has and keeps the file small.
//
// Frames are groups `frame-1`, `frame-2`, … so one line of CSS can animate them.
func SVG(frames []*image.NRGBA, path string) (string, error) {
	if len(frames) == 0 {
		return "", fmt.Errorf("nothing to export: the document has no frames")
	}
	b := frames[0].Bounds()
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf(
		`<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="%d" viewBox="0 0 %d %d" shape-rendering="crispEdges">`+"\n",
		b.Dx(), b.Dy(), b.Dx(), b.Dy()))

	for fi, img := range frames {
		sb.WriteString(fmt.Sprintf("  <g id=\"frame-%d\"", fi+1))
		if fi > 0 {
			// Only the first frame is visible; hiding the rest keeps the static
			// render correct.
			sb.WriteString(` style="display:none"`)
		}
		sb.WriteString(">\n")

		// Group runs by colour, then emit them sorted, so two exports are
		// byte-identical.
		runs := map[craft.RGB][][3]int{}
		bounds := img.Bounds()
		for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
			x := bounds.Min.X
			for x < bounds.Max.X {
				c := img.NRGBAAt(x, y)
				if c.A == 0 {
					x++
					continue
				}
				start := x
				for x < bounds.Max.X && samePixel(img.NRGBAAt(x, y), c) {
					x++
				}
				key := craft.RGB{R: c.R, G: c.G, B: c.B, A: c.A}
				runs[key] = append(runs[key], [3]int{start - bounds.Min.X, y - bounds.Min.Y, x - start})
			}
		}

		colours := make([]craft.RGB, 0, len(runs))
		for c := range runs {
			colours = append(colours, c)
		}
		sort.Slice(colours, func(i, j int) bool { return colours[i].Hex() < colours[j].Hex() })

		for _, c := range colours {
			for _, r := range runs[c] {
				sb.WriteString(fmt.Sprintf(
					`    <rect x="%d" y="%d" width="%d" height="1" fill="%s"/>`+"\n",
					r[0], r[1], r[2], svgColour(c)))
			}
		}
		sb.WriteString("  </g>\n")
	}
	sb.WriteString("</svg>\n")

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", fmt.Errorf("create %s: %w", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(sb.String()), 0o644); err != nil {
		return "", fmt.Errorf("write %s: %w", path, err)
	}
	return path, nil
}

func samePixel(a, b color.NRGBA) bool {
	return a.R == b.R && a.G == b.G && a.B == b.B && a.A == b.A
}

func svgColour(c craft.RGB) string {
	if c.A == 255 {
		return fmt.Sprintf("#%02x%02x%02x", c.R, c.G, c.B)
	}
	return fmt.Sprintf("rgba(%d,%d,%d,%.3f)", c.R, c.G, c.B, float64(c.A)/255)
}
