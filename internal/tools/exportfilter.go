package tools

import (
	"image"
	"image/draw"
	"strings"

	"github.com/d3uceY/spriteware/internal/doc"
	"github.com/d3uceY/spriteware/internal/protocol"
)

// exportFrames turns tag and layer filters into frames to write and how to build
// each.
//
// An unknown name is an error: an agent that asked for "walk" and got every
// frame cannot tell that from a tag that matched nothing.
func exportFrames(d *doc.Dump, tags, layers []string) ([]int, func(int) (*image.NRGBA, error), error) {
	frames, err := framesForTags(d, tags)
	if err != nil {
		return nil, nil, err
	}

	indexes, err := layerIndexes(d, layers)
	if err != nil {
		return nil, nil, err
	}

	if len(indexes) == 0 {
		return frames, func(f int) (*image.NRGBA, error) { return d.Composite(f), nil }, nil
	}

	return frames, func(f int) (*image.NRGBA, error) { return compositeOf(d, indexes, f), nil }, nil
}

// framesForTags returns every frame the named tags cover, or all frames. Order
// is document order, de-duplicated.
func framesForTags(d *doc.Dump, tags []string) ([]int, error) {
	if len(tags) == 0 {
		out := make([]int, 0, d.FrameCount())
		for f := 1; f <= d.FrameCount(); f++ {
			out = append(out, f)
		}
		return out, nil
	}

	keep := map[int]bool{}
	for _, want := range tags {
		want = strings.TrimSpace(want)
		if want == "" {
			continue
		}
		found := false
		for _, t := range d.Tags {
			if !strings.EqualFold(t.Name, want) {
				continue
			}
			found = true
			for f := t.From; f <= t.To; f++ {
				if f >= 1 && f <= d.FrameCount() {
					keep[f] = true
				}
			}
		}
		if !found {
			return nil, protocol.New(protocol.CodeInvalidArgs,
				"no tag named %q", want).
				WithDetail("tags", tagNames(d)).
				WithRemediation("use one of: %s", strings.Join(tagNames(d), ", "))
		}
	}

	out := make([]int, 0, len(keep))
	for f := 1; f <= d.FrameCount(); f++ {
		if keep[f] {
			out = append(out, f)
		}
	}
	if len(out) == 0 {
		return nil, protocol.New(protocol.CodeInvalidArgs, "the tags you named cover no frames")
	}
	return out, nil
}

// layerIndexes resolves the layer names a caller asked for.
func layerIndexes(d *doc.Dump, layers []string) ([]int, error) {
	out := make([]int, 0, len(layers))
	for _, want := range layers {
		want = strings.TrimSpace(want)
		if want == "" {
			continue
		}
		idx := d.IndexOfLayer(want)
		if idx < 0 {
			return nil, protocol.New(protocol.CodeLayerNotFound,
				"no layer named %q", want).
				WithDetail("layers", d.LayerNames()).
				WithRemediation("use one of: %s", strings.Join(d.LayerNames(), ", "))
		}
		out = append(out, idx)
	}
	return out, nil
}

// compositeOf flattens a subset of layers for one frame.
func compositeOf(d *doc.Dump, indexes []int, frame int) *image.NRGBA {
	out := image.NewNRGBA(image.Rect(0, 0, d.Sprite.Width, d.Sprite.Height))
	for _, idx := range indexes {
		draw.Draw(out, out.Bounds(), d.LayerImage(idx, frame), image.Point{}, draw.Over)
	}
	return out
}

func tagNames(d *doc.Dump) []string {
	out := make([]string, 0, len(d.Tags))
	for _, t := range d.Tags {
		out = append(out, t.Name)
	}
	return out
}
