package tools

import (
	"context"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// ---------------------------------------------------------------------------
// select
// ---------------------------------------------------------------------------

// The selection is document state, not a tool mode: once set, every `draw` is
// clipped by it until it is cleared. That is Aseprite's own behaviour, and it is
// what makes "shade only this arm" one sentence instead of a per-pixel list.
func (s *Service) registerSelect(srv *server.MCPServer) {
	commands := map[string]string{
		"set":  "select.set",
		"all":  "select.all",
		"none": "select.none",
		"info": "select.info",
	}

	tool := mcp.NewTool("select",
		mcp.WithDescription(
			"Limit the next edits to one rectangle. Ops: set, all, none, info.\n"+
				coordinateNote+"\n"+
				"A selection is document state: once set, it clips every `draw` op until you clear it with "+
				"`none`. `info` reports the rectangle and how many pixels it covers, so a caller can check "+
				"before drawing. Drawing outside the canvas or with no selection is unaffected.\n"+
				"Pass `ignoreSelection: true` on a draw call to edit the whole cel anyway, and remember to "+
				"clear the selection when the local edit is done: a leftover selection silently clips the "+
				"next unrelated draw."),
		mcp.WithString("op", mcp.Required(), mcp.Enum("set", "all", "none", "info")),
		mcp.WithNumber("x", mcp.Description("For set: left edge, 0-based.")),
		mcp.WithNumber("y", mcp.Description("For set: top edge, 0-based.")),
		mcp.WithNumber("width", mcp.Description("For set: width in pixels.")),
		mcp.WithNumber("height", mcp.Description("For set: height in pixels.")),
		mcp.WithString("sprite", mcp.Description("Target sprite by absolute path.")),
	)

	srv.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		opName := op(req, "info")
		args := spriteArgs(req)
		for _, key := range []string{"x", "y", "width", "height"} {
			if v, exists := req.GetArguments()[key]; exists {
				args[key] = v
			}
		}

		var out map[string]any
		if err := s.call(ctx, commands[opName], args, &out); err != nil {
			return fail(err), nil
		}
		summary := selectionSummary(opName, out)
		out["summary"] = summary
		return ok(out, summary), nil
	})
}

func selectionSummary(opName string, out map[string]any) string {
	empty, _ := out["empty"].(bool)
	if empty {
		return "no selection: draws cover the whole cel"
	}
	return fmt.Sprintf("%s: %dx%d at (%d,%d), %d pixel(s)", opName,
		toInt(out["width"]), toInt(out["height"]),
		toInt(out["x"]), toInt(out["y"]), toInt(out["pixels"]))
}

// ---------------------------------------------------------------------------
// slice
// ---------------------------------------------------------------------------

// A slice names a rectangle: the 9-patch region of a UI panel, or a sprite's
// bounding box. Exporters carry it into the atlas, so an importer does not have
// to guess where a panel's border ends.
func (s *Service) registerSlice(srv *server.MCPServer) {
	commands := map[string]string{
		"list":   "slice.list",
		"create": "slice.create",
		"update": "slice.update",
		"delete": "slice.delete",
	}

	tool := mcp.NewTool("slice",
		mcp.WithDescription(
			"Named rectangles: 9-patch regions and bounding boxes. Ops: list, create, update, delete.\n"+
				coordinateNote+"\n"+
				"`center` is the stretchable middle of a 9-patch: a corner stays fixed, the centre tiles. "+
				"`export` op=spritesheet carries every slice, with its centre, into the atlas JSON, so a UI "+
				"importer reads the panel's borders instead of hard-coding them.\n"+
				"Slices are per document, not per frame; the first rectangle is the one reported."),
		mcp.WithString("op", mcp.Required(), mcp.Enum("list", "create", "update", "delete")),
		mcp.WithString("name", mcp.Description("Slice name. create refuses a duplicate; update and delete need an existing one.")),
		mcp.WithString("rename", mcp.Description("For update: the slice's new name.")),
		mcp.WithNumber("x", mcp.Description("Left edge, 0-based.")),
		mcp.WithNumber("y", mcp.Description("Top edge, 0-based.")),
		mcp.WithNumber("width", mcp.Description("Width in pixels.")),
		mcp.WithNumber("height", mcp.Description("Height in pixels.")),
		mcp.WithObject("center", mcp.Description("Optional 9-patch centre: {x, y, width, height} inside the slice."),
			mcp.Properties(map[string]any{
				"x":      map[string]any{"type": "integer"},
				"y":      map[string]any{"type": "integer"},
				"width":  map[string]any{"type": "integer"},
				"height": map[string]any{"type": "integer"},
			}),
		),
		mcp.WithString("sprite", mcp.Description("Target sprite by absolute path.")),
	)

	srv.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return s.dispatch(ctx, "slice", req, commands)
	})
}
