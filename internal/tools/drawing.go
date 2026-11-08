package tools

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/d3uceY/spriteware/internal/craft"
	"github.com/d3uceY/spriteware/internal/doc"
	"github.com/d3uceY/spriteware/internal/protocol"
	"github.com/d3uceY/spriteware/internal/render"
)

// drawSchema is hand-written because `ops` holds a union of shapes, which
// mcp-go's builders describe badly.
const drawSchema = `{
  "type": "object",
  "properties": {
    "label": {"type": "string", "description": "Shows up in the undo history, e.g. \"shade helmet\"."},
    "sprite": {"type": "string", "description": "Target sprite by absolute path, by the id a reply gave it, or by the name passed to sprite op=new."},
    "ignoreSelection": {"type": "boolean", "description": "Draw the whole cel even when a selection is set. Default false: a selection clips every draw."},
    "ops": {
      "type": "array",
      "description": "Applied in order as ONE undo step. Coordinate space is 0-based and sprite-absolute.",
      "items": {
        "type": "object",
        "properties": {
          "kind": {"type": "string", "enum": ["pixels", "rect", "line", "polyline", "ellipse", "clear", "replace", "remap", "frame", "grid", "text", "outline", "shade", "blit", "shift", "fill", "dither", "gradient", "hue_shift", "desaturate"]},
          "target": {"type": "object", "properties": {
            "layer": {"type": "string"},
            "frame": {"type": "integer", "minimum": 1}
          }, "additionalProperties": false},
          "color": {"type": "string", "description": "#rrggbb or #rrggbbaa"},
          "x": {"type": "integer", "description": "Left edge. For replace/shade/hue_shift/desaturate it starts the region the change is limited to; for a region, give width and height too."},
          "y": {"type": "integer", "description": "Top edge. See x."},
          "width": {"type": "integer", "description": "Region width for replace/shade/hue_shift/desaturate, or the shape width for rect/dither/gradient/clear."},
          "height": {"type": "integer"},
          "x0": {"type": "integer", "description": "Corner form of a region: x0,y0 to x1,y1 inclusive. Use this or x/y/width/height, not both."},
          "y0": {"type": "integer"},
          "x1": {"type": "integer"},
          "y1": {"type": "integer"},
          "radiusX": {"type": "integer"},
          "radiusY": {"type": "integer"},
          "fill": {"type": "boolean"},
          "points": {"type": "array", "items": {"type": "array", "items": {"type": "integer"}}},
          "from": {"type": "string"},
          "to": {"type": "string"},
          "legend": {"type": "object", "description": "grid only: character to #rrggbb, or null for transparent."},
          "rows": {"type": "array", "items": {"type": "string"}, "description": "grid only: one character per pixel."},
          "text": {"type": "string", "description": "text only: the string to draw. Capitals, digits and . , : ; ! ? - + / ' ( ) \" # % * = < > _ &"},
          "side": {"type": "string", "enum": ["outside", "inside"], "description": "outline only: outside grows the shape by a pixel, inside recolours its edge and keeps the size."},
          "diagonals": {"type": "boolean", "description": "outline only: also outline diagonal neighbours, for square corners."},
          "amount": {"type": "number", "description": "shade only: -1..1. Negative darkens and cools, positive lightens and warms. desaturate: 0..1."},
          "degrees": {"type": "number", "description": "hue_shift only: degrees to rotate hue by."},
          "tolerance": {"type": "number", "description": "fill only: how far from the seed colour counts as the same region."},
          "pattern": {"type": "string", "enum": ["checker", "bayer4"], "description": "dither only."},
          "direction": {"type": "string", "enum": ["vertical", "horizontal"], "description": "gradient only."},
          "steps": {"type": "integer", "description": "gradient only: how many bands. Fewer is better; default one per pixel."},
          "source": {"type": "object", "description": "blit only: where to copy from.", "properties": {"layer": {"type": "string"}, "frame": {"type": "integer", "minimum": 1}, "x": {"type": "integer"}, "y": {"type": "integer"}, "width": {"type": "integer"}, "height": {"type": "integer"}}, "additionalProperties": false},
          "map": {"type": "object", "description": "remap only: from-colour to to-colour, for example {\"#ff0000\": \"#00ff00\"}. Applied in one pass, so a target that is also a source cannot chain.", "additionalProperties": {"type": "string"}},
          "dx": {"type": "integer", "description": "shift only: pixels to move right."},
          "dy": {"type": "integer", "description": "shift only: pixels to move down."},
          "onlyTransparent": {"type": "boolean", "description": "shift only: paint only where the cel is empty, default true. That is what makes a drop shadow sit behind the art instead of over it."},
          "cut": {"type": "integer", "description": "frame only: notch size in pixels at each corner, default 1."},
          "outline": {"type": "string", "description": "frame only: outline colour, #rrggbb."},
          "hi": {"type": "string", "description": "frame only: the lit face along the top and left."},
          "lo": {"type": "string", "description": "frame only: the shaded face along the bottom and right."},
          "body": {"type": "string", "description": "frame only: the fill colour. Omit to leave the inside untouched."},
          "openBottom": {"type": "boolean", "description": "frame only: drop the bottom edge, for a tab."},
          "transparent": {"type": "string", "enum": ["erase", "skip"], "description": "grid only. erase (default) clears the rectangle first so it ends up exactly as written."}
        },
        "required": ["kind"],
        "additionalProperties": false
      }
    }
  },
  "required": ["ops"],
  "additionalProperties": false
}`

func (s *Service) registerDraw(srv *server.MCPServer) {
	tool := mcp.NewTool("draw",
		mcp.WithDescription(
			"Draw pixels. All ops in one call are applied as one transaction, so the user's Ctrl+Z "+
				"undoes the whole edit rather than one pixel of it.\n"+
				coordinateNote+"\n"+
				"Kinds: pixels, rect, line, polyline, ellipse, clear, replace, remap, frame, grid, text, outline, shade, blit, shift, fill, dither, gradient, hue_shift, desaturate.\n"+
				"`grid` is the readable one: one character per pixel with a legend mapping character to "+
				"colour, exactly the shape `look` op=ascii returns, so you can read a layer, edit the "+
				"rows and send them straight back. A ragged row or a character missing from the legend "+
				"is refused with its row and column rather than padded or guessed.\n"+
				"`text` draws a built-in 5x7 bitmap font at x,y; it is compiled to pixels here, so the "+
				"font cannot drift between the live and headless paths. `outline` grows or recolours the "+
				"silhouette by one pixel; `shade` lightens or darkens with a hue shift, so shadows cool and "+
				"highlights warm; `blit` copies a rectangle from another cel.\n"+
				"`fill` floods a region from one point, `polyline` draws many segments in one op, and "+
				"`hue_shift`/`desaturate` recolour a whole cel with the hue and value kept separate.\n"+
				"`replace`, `shade`, `hue_shift` and `desaturate` accept a region (`x`,`y`,`width`,`height` "+
				"or `x0`,`y0`,`x1`,`y1`) and change nothing outside it. An argument a kind does not read is "+
				"refused with the name of the argument, not ignored.\n"+"`remap` recolours from a whole `map` in one pass; `frame` draws a bevelled notched box "+
				"(`cut`, `outline`, `hi`, `lo`, `body`, `openBottom`); `shift` copies the cel moved by "+
				"`dx`/`dy`, painting behind the art by default so a shadow is one call.\n"+"The result's `pixelsChanged` counts pixels whose value differed, so a no-op draw reports 0; "+
				"`pixelsWritten` counts the in-bounds pixels the selection allowed."),
		mcp.WithRawInputSchema([]byte(drawSchema)),
	)

	srv.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		raw := req.GetArguments()["ops"]
		list, isList := raw.([]any)
		if !isList || len(list) == 0 {
			return fail(protocol.New(protocol.CodeInvalidArgs, "draw needs a non-empty `ops` array")), nil
		}
		if err := validateDrawOps(list); err != nil {
			return fail(err), nil
		}

		compiled := make([]map[string]any, 0, len(list))
		for i, item := range list {
			entry, isMap := item.(map[string]any)
			if !isMap {
				return fail(protocol.New(protocol.CodeInvalidArgs, "ops[%d] is not an object", i)), nil
			}
			kind, _ := entry["kind"].(string)
			if kind == "grid" {
				ops, err := compileGrid(entry, i)
				if err != nil {
					return fail(err), nil
				}
				compiled = append(compiled, ops...)
				continue
			}
			if kind == "text" {
				op, err := compileText(entry, i)
				if err != nil {
					return fail(err), nil
				}
				compiled = append(compiled, op)
				continue
			}
			compiled = append(compiled, entry)
		}

		args := map[string]any{"ops": compiled}
		if label := strings.TrimSpace(req.GetString("label", "")); label != "" {
			args["label"] = label
		}
		if sp := strings.TrimSpace(req.GetString("sprite", "")); sp != "" {
			// Passed through like every other tool's sprite argument. Resolving it
			// here turned the id or name a reply handed back into a path no
			// document answers to.
			args["sprite"] = sp
		}
		if req.GetBool("ignoreSelection", false) {
			args["ignoreSelection"] = true
		}

		var out map[string]any
		if err := s.call(ctx, "draw.apply", args, &out); err != nil {
			return fail(err), nil
		}
		changed := toInt(out["pixelsChanged"])
		written := toInt(out["pixelsWritten"])

		// Echo the distinct colours the batch asked for. A legend that maps two
		// characters to one colour, or misses one, is otherwise invisible until
		// the picture looks wrong.
		names := paintColours(compiled)
		out["colours"] = names

		summary := fmt.Sprintf("%d pixel(s) changed from %d op(s) in %d colour(s)"+undoLabelSuffix(args),
			changed, len(compiled), len(names))
		if changed == 0 && written > 0 {
			summary = fmt.Sprintf("nothing changed: %d pixel(s) were already that colour%s",
				written, undoLabelSuffix(args))
		}
		out["summary"] = summary
		return ok(out, summary), nil
	})
}

func undoLabelSuffix(args map[string]any) string {
	if label, ok := args["label"].(string); ok && label != "" {
		return " (undo step: " + label + ")"
	}
	return ""
}

// paintColours is the distinct colours a compiled batch names. Each kind spells
// its paint colour differently, so the echo reads all of them; a legend that
// collapsed two characters onto one colour shows up here.
func paintColours(compiled []map[string]any) []string {
	colours := map[string]bool{}
	for _, op := range compiled {
		for _, key := range []string{"color", "to", "outline", "hi", "lo", "body"} {
			if c, isString := op[key].(string); isString && c != "" {
				colours[c] = true
			}
		}
	}
	names := make([]string, 0, len(colours))
	for c := range colours {
		names = append(names, c)
	}
	sort.Strings(names)
	return names
}

// compileGrid turns a character grid into a clear rect plus one pixels op per
// colour.
//
// The input is the output of `look` op=ascii, so a caller can read, edit and
// send it back. Strict on purpose: padding a short row turns a visible mistake
// into silent wrong art.
func compileGrid(entry map[string]any, index int) ([]map[string]any, error) {
	rowsRaw, ok := entry["rows"].([]any)
	if !ok || len(rowsRaw) == 0 {
		return nil, protocol.New(protocol.CodeInvalidArgs, "ops[%d].grid needs `rows`", index)
	}

	rows := make([]string, 0, len(rowsRaw))
	for i, r := range rowsRaw {
		s, isString := r.(string)
		if !isString {
			return nil, protocol.New(protocol.CodeInvalidArgs, "ops[%d].rows[%d] is not a string", index, i)
		}
		rows = append(rows, s)
	}

	legend := map[string]string{}
	if rawLegend, exists := entry["legend"]; exists {
		if m, isMap := rawLegend.(map[string]any); isMap {
			for k, v := range m {
				if hex, isString := v.(string); isString {
					legend[k] = hex
				} else {
					// null means transparent, and is simply absent from the map.
					legend[k] = ""
				}
			}
		}
	}

	x0, y0 := toInt(entry["x"]), toInt(entry["y"])
	width := len([]rune(rows[0]))
	height := len(rows)

	for i, row := range rows {
		if n := len([]rune(row)); n != width {
			return nil, protocol.New(protocol.CodeInvalidArgs,
				"ops[%d].rows[%d] is %d characters but row 0 is %d; every row must be the same width",
				index, i, n, width)
		}
	}

	target, _ := entry["target"].(map[string]any)
	kind := "erase"
	if t, ok := entry["transparent"].(string); ok && t != "" {
		kind = t
	}

	var out []map[string]any
	if kind != "skip" {
		out = append(out, map[string]any{
			"kind": "clear", "x": x0, "y": y0, "width": width, "height": height, "target": target,
		})
	}

	byColour := map[string][][2]int{}
	for y, row := range rows {
		for x, ch := range []rune(row) {
			key := string(ch)
			hex, known := legend[key]
			if !known {
				if key == string(render.Transparent) {
					continue // '.' is transparent unless the legend redefines it
				}
				return nil, protocol.New(protocol.CodeInvalidArgs,
					"ops[%d].rows[%d] uses %q, which is not in the legend",
					index, y, key).
					WithDetail("column", x).
					WithRemediation("add %q to the legend, or use '.' for transparent", key)
			}
			if hex == "" {
				continue
			}
			if _, err := craft.ParseHex(hex); err != nil {
				return nil, protocol.New(protocol.CodeInvalidArgs,
					"ops[%d].legend[%q] is not a colour: %v", index, key, err)
			}
			byColour[hex] = append(byColour[hex], [2]int{x0 + x, y0 + y})
		}
	}

	// Stable order so the same grid always compiles to the same ops.
	colours := make([]string, 0, len(byColour))
	for hex := range byColour {
		colours = append(colours, hex)
	}
	sort.Strings(colours)

	for _, hex := range colours {
		out = append(out, map[string]any{
			"kind": "pixels", "color": hex, "points": byColour[hex], "target": target,
		})
	}
	return out, nil
}

func toInt(v any) int {
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	case int64:
		return int(n)
	}
	return 0
}

// ---------------------------------------------------------------------------
// palette
// ---------------------------------------------------------------------------

func (s *Service) registerPalette(srv *server.MCPServer) {
	tool := mcp.NewTool("palette",
		mcp.WithDescription(
			"Colour work. Ops: get, set, ramp, analyze, adopt_from_art, extract, snap.\n"+
				"`ramp` builds a shading ramp from a base colour with a proper hue shift: shadows drift "+
				"cool, highlights warm. It verifies the result before returning: lightness must rise "+
				"steadily and neighbouring steps must be far enough apart to read as different colours.\n"+
				"`analyze` reports the colours actually in the art against the document palette.\n"+
				"`adopt_from_art` sets the palette to the colours in use, which is the fix for a document "+
				"whose palette and art have quietly drifted apart. `snap` is the other direction: it moves "+
				"every pixel onto its nearest palette entry, so a resized or painted-over sprite goes back "+
				"to a fixed palette without the palette growing to fit the mistakes.\n"+
				"`snap` defaults to every layer and frame; `layer` and `frame` narrow it. It reports how "+
				"many pixels moved and which colours moved where, so a large snap is visible before it is "+
				"accepted."),
		mcp.WithString("op", mcp.Required(),
			mcp.Enum("get", "set", "ramp", "analyze", "adopt_from_art", "extract", "snap")),
		mcp.WithString("base", mcp.Description("For ramp: the base colour, #rrggbb.")),
		mcp.WithString("preset", mcp.Description("For set: a named palette instead of a colour list.")),
		mcp.WithNumber("steps", mcp.Description("For ramp and extract: how many colours.")),
		mcp.WithArray("colors", mcp.Description("For set: the palette as hex strings."), stringItems()),
		mcp.WithString("layer", mcp.Description("For snap: only this layer. Omit for every layer.")),
		mcp.WithNumber("frame", mcp.Description("For snap: only this frame. Omit for every frame.")),
		mcp.WithString("sprite", mcp.Description(spriteNote)),
	)

	srv.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		opName := op(req, "analyze")
		args := spriteArgs(req)

		switch opName {
		case "get":
			var out map[string]any
			if err := s.call(ctx, "palette.get", args, &out); err != nil {
				return fail(err), nil
			}
			return ok(out, fmt.Sprintf("%d palette colours", countOf(out["colors"]))), nil

		case "set":
			colours := req.GetStringSlice("colors", nil)
			if preset := strings.TrimSpace(req.GetString("preset", "")); preset != "" {
				chosen, okPreset := craft.PresetNamed(preset)
				if !okPreset {
					return fail(protocol.New(protocol.CodeInvalidArgs,
						"no palette preset named %q", preset).
						WithRemediation("use one of: %s, or pass `colors`", strings.Join(craft.PresetNames(), ", "))), nil
				}
				colours = make([]string, 0, len(chosen.Colours))
				for _, c := range chosen.Colours {
					colours = append(colours, c.Hex())
				}
			}
			if len(colours) == 0 {
				return fail(protocol.New(protocol.CodeInvalidArgs, "palette op=set needs `colors` or `preset`")), nil
			}
			for _, hex := range colours {
				if _, err := craft.ParseHex(hex); err != nil {
					return fail(protocol.New(protocol.CodeInvalidArgs, "%v", err)), nil
				}
			}
			args["colors"] = colours
			var out map[string]any
			if err := s.call(ctx, "palette.set", args, &out); err != nil {
				return fail(err), nil
			}
			return ok(out, fmt.Sprintf("palette set to %d colours", len(colours))), nil

		case "ramp":
			base, err := craft.ParseHex(req.GetString("base", ""))
			if err != nil {
				return fail(protocol.New(protocol.CodeInvalidArgs,
					"palette op=ramp needs `base` as #rrggbb: %v", err)), nil
			}
			steps := req.GetInt("steps", 5)
			ramp, err := craft.GenerateRamp(base, craft.RampOptions{Steps: steps})
			if err != nil {
				return fail(protocol.New(protocol.CodeInvalidArgs, "could not build a ramp: %v", err)), nil
			}
			hexes := make([]string, 0, len(ramp.Colors))
			for _, c := range ramp.Colors {
				hexes = append(hexes, c.Hex())
			}
			summary := fmt.Sprintf("%d-step ramp from %s: %s (base at index %d)",
				len(hexes), base.Hex(), strings.Join(hexes, " "), ramp.Base)
			return ok(map[string]any{
				"colors": hexes, "baseIndex": ramp.Base, "summary": summary,
			}, summary), nil

		case "analyze", "adopt_from_art", "extract":
			d, err := s.dump(ctx, req)
			if err != nil {
				return fail(err), nil
			}
			inUse := map[craft.RGB]int{}
			for f := 1; f <= d.FrameCount(); f++ {
				for c, n := range doc.Colors(d.Composite(f)) {
					inUse[c] += n
				}
			}
			palette := d.PaletteRGB()
			ordered := doc.SortedColors(inUse)

			report := map[string]any{
				"colorsInUse": len(inUse),
				"paletteSize": len(palette),
			}

			offPalette := make([]map[string]any, 0)
			for _, c := range ordered {
				idx, dist := craft.Nearest(c, palette)
				if idx >= 0 && palette[idx] == c {
					continue
				}
				offPalette = append(offPalette, map[string]any{
					"color": c.Hex(), "pixels": inUse[c], "nearest": nearestHex(palette, idx), "deltaE": round2(dist),
				})
			}
			report["offPalette"] = offPalette

			if opName == "analyze" {
				summary := fmt.Sprintf("%d colours in use across %d palette entries, %d off-palette",
					len(inUse), len(palette), len(offPalette))
				report["summary"] = summary
				return ok(report, summary), nil
			}

			// adopt_from_art takes every colour in use; extract takes the most
			// used ones.
			limit := len(ordered)
			if opName == "extract" {
				limit = req.GetInt("steps", 16)
				if limit < 2 {
					limit = 16
				}
				if limit > len(ordered) {
					limit = len(ordered)
				}
			}
			if limit == 0 {
				return fail(protocol.New(protocol.CodeInvalidArgs,
					"there are no opaque pixels to build a palette from")), nil
			}

			hexes := make([]string, 0, limit)
			for _, c := range ordered[:limit] {
				hexes = append(hexes, c.Hex())
			}
			args["colors"] = hexes
			var setOut map[string]any
			if err := s.call(ctx, "palette.set", args, &setOut); err != nil {
				return fail(err), nil
			}
			summary := fmt.Sprintf("palette replaced with %d colour(s) from the art", len(hexes))
			return ok(map[string]any{
				"colors": hexes, "summary": summary, "offPalette": offPalette,
			}, summary), nil

		case "snap":
			args["layer"] = strings.TrimSpace(req.GetString("layer", ""))
			if v, exists := req.GetArguments()["frame"]; exists {
				args["frame"] = v
			}
			var out map[string]any
			if err := s.call(ctx, "palette.snap", args, &out); err != nil {
				return fail(err), nil
			}
			changedPixels := toInt(out["pixelsChanged"])
			summary := fmt.Sprintf("%d of %d opaque pixel(s) moved onto the palette, %d colour pair(s)",
				changedPixels, toInt(out["pixelsVisited"]), toInt(out["snappedColours"]))
			out["summary"] = summary
			return ok(out, summary), nil
		}

		return fail(unknownOp("palette", opName,
			"get", "set", "ramp", "analyze", "adopt_from_art", "extract", "snap")), nil
	})
}

// dump reads the current document into memory, or the one the request names.
func (s *Service) dump(ctx context.Context, req mcp.CallToolRequest) (*doc.Dump, error) {
	l, err := s.linkFor()
	if err != nil {
		return nil, err
	}
	// The selector goes through untouched: it may be a path, an id or a name, and
	// the document side decides which.
	sprite := strings.TrimSpace(req.GetString("sprite", ""))
	d, err := l.Dump(ctx, &doc.Filter{Sprite: sprite})
	if err != nil {
		return nil, err
	}
	if d.Sprite.Width == 0 || d.Sprite.Height == 0 {
		return nil, protocol.New(protocol.CodeInternal,
			"Aseprite returned a document with no canvas size")
	}
	return d, nil
}

func countOf(v any) int {
	if list, ok := v.([]any); ok {
		return len(list)
	}
	if list, ok := v.([]string); ok {
		return len(list)
	}
	return 0
}

func nearestHex(palette []craft.RGB, idx int) string {
	if idx < 0 || idx >= len(palette) {
		return ""
	}
	return palette[idx].Hex()
}

func round2(v float64) float64 {
	return float64(int(v*100+0.5)) / 100
}
