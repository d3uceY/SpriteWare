package tools

import (
	"sort"

	"github.com/d3uceY/spriteware/internal/protocol"
)

// drawOpArgs lists the arguments each kind actually reads.
//
// The published schema is one union of every kind's fields, so without this
// table a caller can pass `tolerance` to `rect` or a region to `outline` and get
// no error and no effect. Silent acceptance is the defect the table exists to
// stop.
var drawOpArgs = map[string][]string{
	"pixels":     {"color", "points"},
	"rect":       {"color", "fill", "height", "width", "x", "y"},
	"line":       {"color", "x0", "x1", "y0", "y1"},
	"polyline":   {"color", "points"},
	"ellipse":    {"color", "fill", "radiusX", "radiusY", "x", "y"},
	"clear":      {"height", "width", "x", "y"},
	"replace":    {"from", "to", "height", "width", "x", "x0", "x1", "y", "y0", "y1"},
	"remap":      {"map", "height", "width", "x", "x0", "x1", "y", "y0", "y1"},
	"frame":      {"body", "cut", "hi", "height", "lo", "openBottom", "outline", "width", "x", "y"},
	"grid":       {"legend", "rows", "transparent", "x", "y"},
	"text":       {"color", "text", "x", "y"},
	"outline":    {"color", "diagonals", "side"},
	"shade":      {"amount", "height", "width", "x", "x0", "x1", "y", "y0", "y1"},
	"blit":       {"source", "x", "y"},
	"shift":      {"color", "dx", "dy", "onlyTransparent"},
	"fill":       {"color", "tolerance", "x", "y"},
	"dither":     {"from", "height", "pattern", "to", "width", "x", "y"},
	"gradient":   {"direction", "from", "height", "steps", "to", "width", "x", "y"},
	"hue_shift":  {"degrees", "height", "width", "x", "x0", "x1", "y", "y0", "y1"},
	"desaturate": {"amount", "height", "width", "x", "x0", "x1", "y", "y0", "y1"},
}

// validateDrawOps refuses an argument the named kind will never read.
//
// It runs before the Go-side compilation of grid and text, so the check sees
// what the caller wrote, not what it compiled to.
func validateDrawOps(list []any) error {
	for i, item := range list {
		op, ok := item.(map[string]any)
		if !ok {
			return protocol.New(protocol.CodeInvalidArgs, "ops[%d] is not an object", i)
		}
		kind, _ := op["kind"].(string)
		allowed, known := drawOpArgs[kind]
		if !known {
			return protocol.New(protocol.CodeInvalidArgs,
				"ops[%d] has unknown kind %q", i, kind).
				WithDetail("kinds", drawKinds()).
				WithRemediation("use one of the published kinds, or check `draw`'s schema")
		}

		reads := map[string]bool{"kind": true, "target": true}
		for _, name := range allowed {
			reads[name] = true
		}

		keys := make([]string, 0, len(op))
		for key := range op {
			keys = append(keys, key)
		}
		sort.Strings(keys)

		for _, key := range keys {
			if reads[key] {
				continue
			}
			return protocol.New(protocol.CodeInvalidArgs,
				"ops[%d]: kind=%s does not read %q", i, kind, key).
				WithDetail("kind", kind).
				WithDetail("accepts", allowed).
				WithRemediation("remove %q, or use a kind that reads it", key)
		}
	}
	return nil
}

// drawKinds is the table's key list, sorted, for an error that names the options.
func drawKinds() []string {
	out := make([]string, 0, len(drawOpArgs))
	for kind := range drawOpArgs {
		out = append(out, kind)
	}
	sort.Strings(out)
	return out
}
