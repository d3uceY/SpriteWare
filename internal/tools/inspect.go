package tools

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/d3uceY/spriteware/internal/analyze"
	"github.com/d3uceY/spriteware/internal/craft"
	"github.com/d3uceY/spriteware/internal/doc"
	"github.com/d3uceY/spriteware/internal/export"
	"github.com/d3uceY/spriteware/internal/protocol"
	"github.com/d3uceY/spriteware/internal/render"
)

// previewTarget is the long edge a preview is scaled to: 1x is unreadable for a
// vision model, and past ~1024 px is wasted tokens.
const previewTarget = 1024

func (s *Service) registerLook(srv *server.MCPServer) {
	tool := mcp.NewTool("look",
		mcp.WithDescription(
			"See the work. Ops: preview, ascii, diff, filmstrip, onion, compare.\n"+
				"`preview` returns the frame as an image, nearest-neighbour scaled, over a checkerboard so "+"transparency is visible.\n"+
				"`ascii` returns an exact text grid with coordinate rulers and a colour legend. Glyphs are bound to "+
				"palette entries, so a glyph means the same colour on every frame and in every crop. The result "+
				"carries `rows` and `origin`, which `draw` kind=grid takes back unchanged: read, edit, write.\n"+
				"`diff` compares two frames and reports what changed, where, and by how much.\n"+
				"`filmstrip` puts every frame in one continuous row. A vision model reads only the first frame "+
				"of a GIF, and a grid of frames is not reviewable — a strip is.\n"+
				"Every image op also returns a text summary, so a caller with no image reading can check itself."),
		mcp.WithString("op", mcp.Required(), mcp.Enum("preview", "ascii", "diff", "filmstrip", "onion", "compare")),
		mcp.WithNumber("frame", mcp.Description("1-based frame. Defaults to the active frame.")),
		mcp.WithString("against", mcp.Description("compare only: the second layer to show beside `layer`. Omit to compare layers `layer` and `reference`.")),
		mcp.WithNumber("fromFrame", mcp.Description("For diff: the earlier frame.")),
		mcp.WithNumber("toFrame", mcp.Description("For diff: the later frame.")),
		mcp.WithString("layer", mcp.Description("Read one layer instead of the composite.")),
		mcp.WithNumber("x", mcp.Description("For ascii and preview: left edge of the region to read. Default the whole image.")),
		mcp.WithNumber("y", mcp.Description("For ascii and preview: top edge of the region.")),
		mcp.WithNumber("width", mcp.Description("For ascii and preview: region width.")),
		mcp.WithNumber("height", mcp.Description("For ascii and preview: region height.")),
		mcp.WithNumber("scale", mcp.Description("For preview: whole-number zoom. Omit to fit the preview budget.")),
		mcp.WithString("path", mcp.Description("For ascii: read this PNG from disk instead of a document, so an exported file can be checked without opening it.")),
		mcp.WithBoolean("rulers", mcp.Description("ascii/diff: false drops the rulers, leaving rows you can send straight back to draw.")),
		mcp.WithString("sprite", mcp.Description("Target sprite by absolute path.")),
	)

	srv.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		opName := op(req, "preview")

		// `path` reads a PNG instead of a document, so an export can be checked
		// without opening it. Any other op reading it would ignore it.
		if p := strings.TrimSpace(req.GetString("path", "")); p != "" {
			if opName != "ascii" {
				return fail(protocol.New(protocol.CodeInvalidArgs,
					"look op=%s does not read `path`; only ascii does", opName)), nil
			}
			if strings.TrimSpace(req.GetString("sprite", "")) != "" {
				return fail(protocol.New(protocol.CodeInvalidArgs,
					"look ascii reads either `path` or `sprite`, not both")), nil
			}
			return asciiFromFile(req)
		}

		d, err := s.dump(ctx, req)
		if err != nil {
			return fail(err), nil
		}

		frame := req.GetInt("frame", d.ActiveFrame)
		if frame < 1 || frame > d.FrameCount() {
			return fail(protocol.New(protocol.CodeFrameOutOfBounds,
				"frame %d does not exist; this sprite has %d frame(s)", frame, d.FrameCount()).
				WithDetail("frames", d.FrameCount())), nil
		}

		img, err := selectImage(d, req.GetString("layer", ""), frame)
		if err != nil {
			return fail(err), nil
		}
		palette := d.PaletteRGB()
		rulers := req.GetBool("rulers", true)

		switch opName {
		case "ascii":
			grid, err := render.ASCII(img, palette, render.ASCIIOptions{Rulers: rulers, Region: lookRegion(req)})
			if err != nil {
				return fail(protocol.New(protocol.CodeInvalidArgs, "%v", err)), nil
			}
			return ok(asciiPayload(grid), asciiText(grid)), nil

		case "diff":
			from := req.GetInt("fromFrame", 1)
			to := req.GetInt("toFrame", from+1)
			before, err := selectImage(d, req.GetString("layer", ""), from)
			if err != nil {
				return fail(err), nil
			}
			after, err := selectImage(d, req.GetString("layer", ""), to)
			if err != nil {
				return fail(err), nil
			}
			diff, err := render.Diff(before, after, palette, rulers)
			if err != nil {
				return fail(protocol.New(protocol.CodeInvalidArgs, "%v", err)), nil
			}
			summary := fmt.Sprintf("frame %d -> %d: %d of %d pixels changed (%.1f%%)",
				from, to, diff.ChangedPixels, diff.TotalPixels, diff.PercentChanged)
			out := map[string]any{
				"op":             "diff",
				"from":           from,
				"to":             to,
				"text":           diff.Text,
				"changedPixels":  diff.ChangedPixels,
				"totalPixels":    diff.TotalPixels,
				"percentChanged": round2(diff.PercentChanged),
				"summary":        summary,
			}
			if diff.HasChanges {
				out["changedBounds"] = map[string]int{
					"x": diff.ChangedBounds.Min.X, "y": diff.ChangedBounds.Min.Y,
					"width": diff.ChangedBounds.Dx(), "height": diff.ChangedBounds.Dy(),
				}
			} else {
				out["changedBounds"] = nil
			}
			return ok(out, diff.Text+"\n\n"+summary), nil

		case "filmstrip":
			frames := make([]*image.NRGBA, 0, d.FrameCount())
			for f := 1; f <= d.FrameCount(); f++ {
				img, err := selectImage(d, req.GetString("layer", ""), f)
				if err != nil {
					return fail(err), nil
				}
				frames = append(frames, img)
			}
			strip := render.Filmstrip(frames, 1)
			scale := render.AutoScale(strip.Bounds().Dx(), strip.Bounds().Dy(), previewTarget)
			summary := fmt.Sprintf("%d frame(s) across %dpx wide", len(frames), strip.Bounds().Dx())
			return imageResult(render.Scale(render.WithChecker(strip, 4), scale), summary)

		case "preview":
			if region := lookRegion(req); !region.Empty() {
				img = render.Crop(img, region)
			}
			scale := render.AutoScale(img.Bounds().Dx(), img.Bounds().Dy(), previewTarget)
			if want := req.GetInt("scale", 0); want >= 1 {
				scale = want
				if scale > 16 {
					scale = 16
				}
			}
			bounds, hasInk := doc.Bounds(img)
			summary := fmt.Sprintf("frame %d at %dx, %dx%d%s", frame, scale,
				img.Bounds().Dx(), img.Bounds().Dy(), inkNote(bounds, hasInk))
			return imageResult(render.Scale(render.WithChecker(img, 4), scale), summary)

		case "onion":
			// The neighbours are the point: an in-between is judged by the gap
			// either side of it.
			frames := make([]*image.NRGBA, 0, d.FrameCount())
			for f := 1; f <= d.FrameCount(); f++ {
				layerImg, err := selectImage(d, req.GetString("layer", ""), f)
				if err != nil {
					return fail(err), nil
				}
				frames = append(frames, layerImg)
			}
			onion := render.Onion(frames, frame-1)
			scale := render.AutoScale(onion.Bounds().Dx(), onion.Bounds().Dy(), previewTarget)
			summary := fmt.Sprintf("frame %d of %d over its ghosted neighbours", frame, d.FrameCount())
			return imageResult(render.Scale(render.WithChecker(onion, 4), scale), summary)

		case "compare":
			// Comparing judges a redraw against a reference: two pictures in a
			// row, one scale, one image.
			first := req.GetString("layer", "")
			second := req.GetString("against", "")
			if first == "" {
				first = "reference"
			}
			if second == "" {
				second = "base"
			}
			if first == second {
				return fail(protocol.New(protocol.CodeInvalidArgs,
					"compare needs two different layers; got %q twice", first)), nil
			}

			left, err := selectImage(d, first, frame)
			if err != nil {
				return fail(err), nil
			}
			right, err := selectImage(d, second, frame)
			if err != nil {
				return fail(err), nil
			}

			strip := render.Filmstrip([]*image.NRGBA{left, right}, 2)
			scale := render.AutoScale(strip.Bounds().Dx(), strip.Bounds().Dy(), previewTarget)
			summary := fmt.Sprintf("%s (left) beside %s (right), %dx%d", first, second,
				strip.Bounds().Dx(), strip.Bounds().Dy())
			return imageResult(render.Scale(render.WithChecker(strip, 4), scale), summary)
		}

		return fail(unknownOp("look", opName, "preview", "ascii", "diff", "filmstrip", "onion", "compare")), nil
	})
}

// lookRegion reads the crop a look op is limited to. A missing or empty width
// or height means the whole image.
func lookRegion(req mcp.CallToolRequest) image.Rectangle {
	w, h := req.GetInt("width", 0), req.GetInt("height", 0)
	if w <= 0 || h <= 0 {
		return image.Rectangle{}
	}
	x, y := req.GetInt("x", 0), req.GetInt("y", 0)
	return image.Rect(x, y, x+w, y+h)
}

// asciiPayload is the one shape an ascii grid is returned in, whether it came
// from a document or from a PNG on disk.
func asciiPayload(grid render.Grid) map[string]any {
	return map[string]any{
		"op":         "ascii",
		"width":      grid.Width,
		"height":     grid.Height,
		"origin":     map[string]int{"x": grid.OriginX, "y": grid.OriginY},
		"text":       grid.Text,
		"rows":       grid.Rows,
		"legend":     grid.Legend,
		"offPalette": grid.OffPalette,
		"summary":    asciiSummary(grid),
	}
}

func asciiSummary(grid render.Grid) string {
	return fmt.Sprintf("%dx%d at (%d,%d), %d colour(s)%s",
		grid.Width, grid.Height, grid.OriginX, grid.OriginY, len(grid.Legend),
		offPaletteNote(grid.OffPalette))
}

func asciiText(grid render.Grid) string {
	return grid.Text + "\n\nLegend: " + formatLegend(grid.Legend) +
		"\n'.' = transparent" + offPaletteNote(grid.OffPalette)
}

// asciiFromFile renders a PNG as a grid without opening a document.
//
// The palette is the file's own colours, sorted, so the glyphs are stable for
// the same file and a re-export can be compared with the previous one.
func asciiFromFile(req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	path, err := resolveOutput(req.GetString("path", ""))
	if err != nil {
		return fail(err), nil
	}
	payload, text, err := asciiFile(path, lookRegion(req), req.GetBool("rulers", true))
	if err != nil {
		return fail(err), nil
	}
	return ok(payload, text), nil
}

// asciiFile is the part of asciiFromFile that does not need a request, so the
// PNG path can be tested without one.
func asciiFile(path string, region image.Rectangle, rulers bool) (map[string]any, string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, "", protocol.New(protocol.CodeNotFound, "could not read %s: %v", path, err).
			WithRemediation("check the path, or export the sprite first")
	}
	defer f.Close()

	src, err := png.Decode(f)
	if err != nil {
		return nil, "", protocol.New(protocol.CodeInvalidArgs, "%s is not a readable PNG: %v", path, err)
	}
	img := asNRGBA(src)
	grid, err := render.ASCII(img, imagePalette(img), render.ASCIIOptions{
		Rulers: rulers,
		Region: region,
	})
	if err != nil {
		return nil, "", protocol.New(protocol.CodeInvalidArgs, "%v", err)
	}

	payload := asciiPayload(grid)
	payload["path"] = path
	return payload, asciiText(grid), nil
}

// asNRGBA is the copy that makes a decoded PNG drawable by the rest of the
// package: a sub-image shares its parent's stride and offset.
func asNRGBA(src image.Image) *image.NRGBA {
	b := src.Bounds()
	out := image.NewNRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	for y := 0; y < b.Dy(); y++ {
		for x := 0; x < b.Dx(); x++ {
			out.SetNRGBA(x, y, color.NRGBAModel.Convert(src.At(b.Min.X+x, b.Min.Y+y)).(color.NRGBA))
		}
	}
	return out
}

// imagePalette is the distinct opaque colours of an image, ordered by hex so the
// glyph a colour gets does not depend on where it appears.
func imagePalette(img *image.NRGBA) []craft.RGB {
	seen := map[craft.RGB]bool{}
	out := make([]craft.RGB, 0, 16)
	b := img.Bounds()
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
			out = append(out, rgb)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Hex() < out[j].Hex() })
	return out
}

// selectImage returns the composite or one layer of a frame.
func selectImage(d *doc.Dump, layer string, frame int) (*image.NRGBA, error) {
	layer = strings.TrimSpace(layer)
	if layer == "" {
		return d.Composite(frame), nil
	}
	idx := d.IndexOfLayer(layer)
	if idx < 0 {
		return nil, protocol.New(protocol.CodeLayerNotFound,
			"no layer named %q", layer).
			WithDetail("layers", d.LayerNames()).
			WithRemediation("use one of: %s", strings.Join(d.LayerNames(), ", "))
	}
	return d.LayerImage(idx, frame), nil
}

func inkNote(r image.Rectangle, hasInk bool) string {
	if !hasInk {
		return ", empty"
	}
	return fmt.Sprintf(", ink at (%d,%d)..(%d,%d)", r.Min.X, r.Min.Y, r.Max.X-1, r.Max.Y-1)
}

func offPaletteNote(off []string) string {
	if len(off) == 0 {
		return ""
	}
	return fmt.Sprintf("; %d off-palette glyph(s): %s (ordered by colour, so the same picture always gets the same glyphs)", len(off), strings.Join(off, ""))
}

func formatLegend(legend map[string]string) string {
	keys := make([]string, 0, len(legend))
	for k := range legend {
		keys = append(keys, k)
	}
	// Sorted by glyph so two reads of the same sprite print the same legend.
	sortStrings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, k+"="+legend[k])
	}
	return strings.Join(parts, " ")
}

func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}

// imageResult returns a picture plus one line of text, so a caller that cannot
// read images can still verify its work.
func imageResult(img *image.NRGBA, summary string) (*mcp.CallToolResult, error) {
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return mcp.NewToolResultError("could not encode the image: " + err.Error()), nil
	}
	encoded := base64.StdEncoding.EncodeToString(buf.Bytes())
	return mcp.NewToolResultImage(summary, encoded, "image/png"), nil
}

// ---------------------------------------------------------------------------
// validate
// ---------------------------------------------------------------------------

const validateSchema = `{
  "type": "object",
  "properties": {
    "sprite": {"type": "string"},
    "checks": {
      "type": "array",
      "items": {"type": "string", "enum": ["palette","strays","orphans","gaps","bounds","outline","banding","antialiasing","layers","animation","export_readiness","volume","ui"]}
    },
    "scope": {"type": "string", "enum": ["composite","layer"], "description": "composite (default) judges the whole picture; layer judges each layer alone, which produces false alarms on a correct rig."},
    "strict": {"type": "boolean", "description": "Treat style warnings as failures."},
    "expect": {
      "type": "object",
      "description": "The plan you already know. Runs on top of the checks.",
      "properties": {
        "layerFrames": {"type": "object", "additionalProperties": {"type": "array", "items": {"type": "array", "items": {"type": "integer"}}}},
        "mustNotOverlap": {"type": "array", "items": {"type": "array", "items": {"type": "string"}}},
        "paletteSize": {"type": "integer"},
        "singleComponent": {"type": "boolean"},
        "bounds": {"type": "object", "properties": {"x": {"type": "integer"},"y": {"type": "integer"},"width": {"type": "integer"},"height": {"type": "integer"}}}
      },
      "additionalProperties": false
    }
  },
  "additionalProperties": false
}`

func (s *Service) registerValidate(srv *server.MCPServer) {
	tool := mcp.NewTool("validate",
		mcp.WithDescription(
			"Check the sprite against pixel-art rules and report located problems.\n"+
				"Checks: palette, strays, orphans, gaps, bounds, outline, banding, antialiasing, layers, "+
				"animation, export_readiness, volume, ui. Omit `checks` to run all of them.\n"+
				"`ui` judges an interface asset by its own rules: canvas size, colour count, hard alpha, "+
				"matching canvas corners, slices that fit, even margins. The character checks flag a correct "+
				"notched panel, so ask for `ui` explicitly rather than relying on the default set.\n"+
				"`scope` defaults to composite: judging a layer alone reports a correct rigged character as "+
				"broken, so that is opt-in.\n"+
				"`expect` runs on top of whatever checks you pick and checks your own plan: per-layer frame "+
				"ranges, layer pairs that must never overlap, a palette cap, a bounding box.\n"+
				"Run this before telling anyone a sprite is finished. It answers \"is this done\" with evidence."),
		mcp.WithRawInputSchema([]byte(validateSchema)),
	)

	srv.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		d, err := s.dump(ctx, req)
		if err != nil {
			return fail(err), nil
		}

		options := analyze.Options{
			Scope:  analyze.Scope(req.GetString("scope", string(analyze.ScopeComposite))),
			Strict: req.GetBool("strict", false),
		}
		if checks := req.GetStringSlice("checks", nil); len(checks) > 0 {
			options.Checks = checks
		}
		var expect analyze.Expect
		if err := decodeArg(req, "expect", &expect); err != nil {
			return fail(err), nil
		}
		if !isEmptyExpect(expect) {
			options.Expect = &expect
		}

		report := analyze.Run(d, options)
		return ok(report, report.Summary), nil
	})
}

func isEmptyExpect(e analyze.Expect) bool {
	return len(e.LayerFrames) == 0 && len(e.MustNotOverlap) == 0 &&
		e.PaletteSize == nil && e.Bounds == nil && !e.SingleComponent
}

// ---------------------------------------------------------------------------
// export
// ---------------------------------------------------------------------------

// writtenEntry is one image file plus what it holds, so a wrong export is
// visible in the reply instead of after opening the file.
type writtenEntry struct {
	Path   string       `json:"path"`
	Frames int          `json:"frames,omitempty"`
	Stats  export.Stats `json:"stats"`
}

// describeFile measures the frames that produced one file.
func describeFile(path string, frames ...*image.NRGBA) writtenEntry {
	return writtenEntry{Path: path, Frames: len(frames), Stats: export.Measure(frames...)}
}

func (s *Service) registerExport(srv *server.MCPServer) {
	tool := mcp.NewTool("export",
		mcp.WithDescription(
			"Write files. Ops: png, frames, svg, gif, spritesheet, tileset.\n"+
				"Every op returns the concrete paths it wrote, never the pattern you passed. `frames` numbers "+
				"files from 0 (`{frame}`) or 1 (`{n}`), and says which.\n"+
				"Every image file also comes back in `written` with its width, height, opaque pixels and "+
				"colour list, so an export of the wrong document is visible without opening the file.\n"+
				"SVG emits one rect per run of identical pixels, grouped per frame.\n"+
				"`gif` uses the frame durations you set. `spritesheet` writes one packed PNG in a row plus a "+
				"JSON atlas beside it with per-frame rectangles, the tags, and the document's slices with their "+
				"9-patch centres and the CSS `border-image` shorthand; each frame carries its "+
				"`backgroundPosition`; `padding` defaults to 1 so a GPU sampling at non-integer zoom cannot "+
				"pick up the neighbouring frame, and `padding=0` is the CSS strip.\n"+
				"`tileset` slices every frame into `tileWidth` x `tileHeight` tiles and keeps one copy of each "+
				"distinct tile, plus a grid of tile numbers per frame (0 is empty). `format=tiled` adds a .tsx "+
				"and one .tmx per frame; `format=godot` adds a Godot 4 .tres TileSet.\n"+
				"`tags` keeps only the frames those tags cover and `layers` exports only those layers, so one "+
				"command exports one animation. An unknown tag or layer name is refused rather than quietly "+
				"exporting everything."),
		mcp.WithString("op", mcp.Required(), mcp.Enum("png", "frames", "svg", "gif", "spritesheet", "tileset")),
		mcp.WithString("path", mcp.Description("Destination file, or destination directory for op=frames.")),
		mcp.WithNumber("frame", mcp.Description("For png: 1-based frame.")),
		mcp.WithNumber("scale", mcp.Description("Whole-number upscale, default 1.")),
		mcp.WithNumber("padding", mcp.Description("For spritesheet: transparent pixels between frames, default 1.")),
		mcp.WithNumber("tileWidth", mcp.Description("For tileset: tile width in pixels before `scale`.")),
		mcp.WithNumber("tileHeight", mcp.Description("For tileset: tile height in pixels before `scale`.")),
		mcp.WithString("format", mcp.Description("For tileset: json (default), tiled or godot. Adds that tool's import file beside the sheet.")),
		mcp.WithArray("tags", mcp.Description("Keep only the frames these tags cover."), stringItems()),
		mcp.WithArray("layers", mcp.Description("Export only these layers."), stringItems()),
		mcp.WithString("sprite", mcp.Description("Target sprite by absolute path.")),
	)

	srv.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		opName := op(req, "png")
		path, err := resolveOutput(req.GetString("path", ""))
		if err != nil {
			return fail(err), nil
		}
		d, err := s.dump(ctx, req)
		if err != nil {
			return fail(err), nil
		}
		scale := req.GetInt("scale", 1)
		if scale < 1 {
			scale = 1
		}

		frameList, imageAt, err := exportFrames(d, req.GetStringSlice("tags", nil), req.GetStringSlice("layers", nil))
		if err != nil {
			return fail(err), nil
		}
		all := func() ([]*image.NRGBA, error) {
			out := make([]*image.NRGBA, 0, len(frameList))
			for _, f := range frameList {
				img, err := imageAt(f)
				if err != nil {
					return nil, err
				}
				out = append(out, img)
			}
			return out, nil
		}
		durationsOf := func() []int {
			out := make([]int, 0, len(frameList))
			for _, f := range frameList {
				out = append(out, d.Frames[f-1].DurationMS)
			}
			return out
		}

		switch opName {
		case "png":
			frame := req.GetInt("frame", d.ActiveFrame)
			img, err := imageAt(frame)
			if err != nil {
				return fail(err), nil
			}
			written, err := export.PNG(img, path, scale)
			if err != nil {
				return fail(protocol.New(protocol.CodeOpenFailed, "%v", err)), nil
			}
			summary := "wrote " + written
			return ok(map[string]any{
				"files":   []string{written},
				"written": []writtenEntry{describeFile(written, img)},
				"summary": summary,
			}, summary), nil

		case "frames":
			frames, err := all()
			if err != nil {
				return fail(err), nil
			}
			base := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
			written, err := export.Frames(frames, filepath.Dir(path), base, scale)
			if err != nil {
				return fail(protocol.New(protocol.CodeOpenFailed, "%v", err)), nil
			}
			summary := fmt.Sprintf("wrote %d file(s), %s .. %s", len(written), written[0], written[len(written)-1])
			entries := make([]writtenEntry, 0, len(written))
			for i := range written {
				entries = append(entries, describeFile(written[i], frames[i]))
			}
			return ok(map[string]any{
				"files":   written,
				"written": entries,
				"frames":  frameList,
				"summary": summary,
			}, summary), nil

		case "svg":
			frames, err := all()
			if err != nil {
				return fail(err), nil
			}
			written, err := export.SVG(frames, path)
			if err != nil {
				return fail(protocol.New(protocol.CodeOpenFailed, "%v", err)), nil
			}
			summary := "wrote " + written
			return ok(map[string]any{
				"files":   []string{written},
				"written": []writtenEntry{describeFile(written, frames...)},
				"frames":  frameList,
				"summary": summary,
			}, summary), nil

		case "gif":
			frames, err := all()
			if err != nil {
				return fail(err), nil
			}
			written, err := export.GIF(frames, durationsOf(), path, scale)
			if err != nil {
				return fail(protocol.New(protocol.CodeOpenFailed, "%v", err)), nil
			}
			summary := fmt.Sprintf("wrote %s, %d frame(s)", written, len(frames))
			return ok(map[string]any{
				"files":   []string{written},
				"written": []writtenEntry{describeFile(written, frames...)},
				"frames":  frameList,
				"summary": summary,
			}, summary), nil

		case "spritesheet":
			frames, err := all()
			if err != nil {
				return fail(err), nil
			}
			padding := req.GetInt("padding", 1)
			pngPath, jsonPath, atlas, err := export.Sheet(export.SheetOptions{
				Frames:    frames,
				Durations: durationsOf(),
				Tags:      d.Tags,
				Slices:    d.Slices,
				Path:      path,
				Scale:     scale,
				Padding:   padding,
			})
			if err != nil {
				return fail(protocol.New(protocol.CodeOpenFailed, "%v", err)), nil
			}
			summary := fmt.Sprintf("wrote %s and %s: %dx%d, %d frame(s)",
				pngPath, jsonPath, atlas.Width, atlas.Height, len(atlas.Frames))
			return ok(map[string]any{
				"files":   []string{pngPath, jsonPath},
				"written": []writtenEntry{describeFile(pngPath, atlas.Sheet)},
				"atlas":   atlas,
				"frames":  frameList,
				"summary": summary,
			}, summary), nil

		case "tileset":
			frames, err := all()
			if err != nil {
				return fail(err), nil
			}
			tileW := req.GetInt("tileWidth", 0)
			tileH := req.GetInt("tileHeight", 0)
			if tileW <= 0 || tileH <= 0 {
				return fail(protocol.New(protocol.CodeInvalidArgs,
					"export op=tileset needs `tileWidth` and `tileHeight` in pixels, for example 16 and 16")), nil
			}
			written, tiles, err := export.PackTileset(export.TilesetOptions{
				Frames: frames,
				TileW:  tileW,
				TileH:  tileH,
				Path:   path,
				Scale:  scale,
				Format: strings.TrimSpace(req.GetString("format", "json")),
			})
			if err != nil {
				return fail(protocol.New(protocol.CodeOpenFailed, "%v", err)), nil
			}
			summary := fmt.Sprintf("wrote %d file(s): %d distinct tile(s) in a %d-column strip",
				len(written), tiles.TileCount, tiles.Columns)
			return ok(map[string]any{
				"files":   written,
				"written": []writtenEntry{describeFile(written[0], frames...)},
				"tileset": tiles,
				"frames":  frameList,
				"summary": summary,
			}, summary), nil
		}

		return fail(unknownOp("export", opName, "png", "frames", "svg", "gif", "spritesheet", "tileset")), nil
	})
}
