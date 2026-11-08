package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/d3uceY/spriteware/internal/protocol"
)

// batchItems describes one entry of an `ops` array: an op name plus that op's
// arguments. An array schema must declare `items`: VS Code and OpenAI reject the
// whole tool otherwise.
func batchItems() mcp.PropertyOption {
	return mcp.Items(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"op": map[string]any{"type": "string"},
		},
		"required": []string{"op"},
	})
}

// integerItems and stringItems are the same rule for the flat list arguments.
func integerItems() mcp.PropertyOption { return mcp.Items(map[string]any{"type": "integer"}) }

func stringItems() mcp.PropertyOption { return mcp.Items(map[string]any{"type": "string"}) }

// side means a batch of twelve reports which one failed instead of discarding
// the eleven that worked.
type opResult struct {
	Op    string `json:"op"`
	OK    bool   `json:"ok"`
	Data  any    `json:"data,omitempty"`
	Code  string `json:"code,omitempty"`
	Error string `json:"error,omitempty"`
}

// dispatch runs one op, or a batch of them, against the Lua command table.
//
// `commands` maps the tool's op name to the Lua command; every other argument is
// passed through unchanged.
func (s *Service) dispatch(
	ctx context.Context,
	tool string,
	req mcp.CallToolRequest,
	commands map[string]string,
) (*mcp.CallToolResult, error) {
	ops, batched := extractOps(req)
	if len(ops) == 0 {
		return fail(protocol.New(protocol.CodeInvalidArgs,
			"%s needs an op", tool).WithDetail("allowed", keysOf(commands))), nil
	}

	results := make([]opResult, 0, len(ops))
	for _, entry := range ops {
		opName, _ := entry["op"].(string)
		opName = strings.TrimSpace(opName)

		cmd, known := commands[opName]
		if !known {
			results = append(results, opResult{
				Op:    opName,
				Code:  protocol.CodeInvalidArgs,
				Error: fmt.Sprintf("%s has no op %q", tool, opName),
			})
			continue
		}

		args := make(map[string]any, len(entry))
		for k, v := range entry {
			if k != "op" {
				args[k] = v
			}
		}
		if v, exists := req.GetArguments()["sprite"]; exists {
			if _, alreadyThere := args["sprite"]; !alreadyThere {
				args["sprite"] = v
			}
		}

		var data any
		err := s.call(ctx, cmd, args, &data)
		if err != nil {
			e := protocol.AsError(err)
			results = append(results, opResult{Op: opName, Code: e.Code, Error: e.Message})
			if !batched {
				return fail(err), nil
			}
			continue
		}
		results = append(results, opResult{Op: opName, OK: true, Data: data})
		if !batched {
			return ok(results[0], summaryOf(opName, results[0])), nil
		}
	}

	failures := 0
	for _, r := range results {
		if !r.OK {
			failures++
		}
	}
	summary := fmt.Sprintf("%d op(s), %d failed", len(results), failures)
	if failures == 0 && len(results) == 1 {
		summary = summaryOf(results[0].Op, results[0])
	}
	return ok(map[string]any{"results": results, "failed": failures, "summary": summary}, summary), nil
}

// summaryOf is the one-line text beside a structured result. A bare "done" tells
// a caller that reads only text nothing, so a named result says which name and
// how many things it holds.
func summaryOf(opName string, r opResult) string {
	if !r.OK {
		return opName + " failed: " + r.Error
	}
	data, isMap := r.Data.(map[string]any)
	if !isMap {
		return opName + " done"
	}
	name, _ := data["name"].(string)
	if name == "" {
		return opName + " done"
	}
	if n := countOf(data["layers"]); n > 0 {
		return fmt.Sprintf("%s %s: %d layer(s)", opName, name, n)
	}
	return opName + " " + name
}

// extractOps reads either the flat argument map or an `ops` array. A batch
// replaces the twelve calls the reference project needed to move four layers
// across three frames.
func extractOps(req mcp.CallToolRequest) ([]map[string]any, bool) {
	args := req.GetArguments()

	if raw, exists := args["ops"]; exists {
		list, ok := raw.([]any)
		if !ok {
			return nil, true
		}
		out := make([]map[string]any, 0, len(list))
		for _, item := range list {
			if m, isMap := item.(map[string]any); isMap {
				out = append(out, m)
			}
		}
		return out, true
	}

	if _, exists := args["op"]; !exists {
		return nil, false
	}
	return []map[string]any{args}, false
}

func keysOf(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

// ---------------------------------------------------------------------------
// layer
// ---------------------------------------------------------------------------

func (s *Service) registerLayer(srv *server.MCPServer) {
	commands := map[string]string{
		"list":      "layer.list",
		"create":    "layer.create",
		"rename":    "layer.rename",
		"delete":    "layer.delete",
		"set":       "layer.set",
		"duplicate": "layer.duplicate",
		"reorder":   "layer.reorder",
		"activate":  "layer.activate",
		"group":     "layer.group",
		"ungroup":   "layer.ungroup",
		"merge":     "layer.merge",
	}

	ops := []string{"list", "create", "rename", "delete", "set", "duplicate", "reorder", "activate", "group", "ungroup", "merge"}

	tool := mcp.NewTool("layer",
		mcp.WithDescription(
			"Manage layers. Ops: list, create, rename, delete, set, duplicate, reorder, activate, group, ungroup, merge.\n"+
				"Every op may be repeated in an `ops` array, applied as one undo step: "+
				"layer(ops=[{op:\"set\",name:\"a\",visible:false},{op:\"set\",name:\"b\",opacity:128}]).\n"+
				indexNote+"\n"+
				"A layer name may be a plain name or a \"group/child\" path. A name that matches more "+
				"than one layer fails and lists every match instead of silently picking one.\n"+
				"`group` puts the named layers into one new group, which takes the slot of the lowest "+
				"layer moved. `ungroup` lifts a group's children into its parent and deletes the group; it "+
				"takes the one group to open, as `name` or as the only entry of `layers`. "+
				"`merge` composites the named layers into one, bottom to top, and deletes the originals: "+
				"the result keeps the pixels, not the blend modes."),
		mcp.WithString("op", mcp.Enum(ops...)),
		mcp.WithArray("ops", mcp.Description("Batch: an array of {op, …args} applied in order."), batchItems()),
		mcp.WithString("name", mcp.Description("Target layer, the new name for op=create, or the group's name for op=group.")),
		mcp.WithString("to", mcp.Description("New name for rename, the copy's name for duplicate, or the merged layer's name for merge.")),
		mcp.WithArray("layers", mcp.Description("For group, ungroup and merge: the layer names to act on. Every name must share a parent. ungroup opens exactly one group."), stringItems()),
		mcp.WithBoolean("visible", mcp.Description("For op=set.")),
		mcp.WithNumber("opacity", mcp.Description("For op=set, 0-255.")),
		mcp.WithString("blendMode", mcp.Description("For op=set: normal, multiply, screen, overlay, darken, lighten, color_dodge, color_burn, hard_light, soft_light, difference, exclusion, hue, saturation, color, luminosity, addition, subtract, divide.")),
		mcp.WithNumber("index", mcp.Description("For reorder: 0-based stack position, 0 at the bottom. The result reports where the layer actually ended up.")),
		mcp.WithString("sprite", mcp.Description(spriteNote)),
	)

	srv.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return s.dispatch(ctx, "layer", req, commands)
	})
}

// ---------------------------------------------------------------------------
// cel
// ---------------------------------------------------------------------------

func (s *Service) registerCel(srv *server.MCPServer) {
	commands := map[string]string{
		"list":      "cel.list",
		"create":    "cel.create",
		"clear":     "cel.clear",
		"delete":    "cel.delete",
		"move":      "cel.move",
		"copy":      "cel.copy",
		"tween":     "cel.tween",
		"oscillate": "cel.oscillate",
		"set":       "cel.set",
		"reset":     "cel.reset",
	}

	tool := mcp.NewTool("cel",
		mcp.WithDescription(
			"Manage the pixels one layer holds on one frame. Ops: list, create, clear, delete, move, copy, tween, oscillate, set, reset.\n"+
				"Batches as `ops`.\n"+indexNote+"\n"+
				"`copy` copies one cel onto another layer or frame, which is how a baked sprite is split onto a rig "+
				"(`fromLayer`/`fromFrame` default to the target). It stamps the source over the destination, so "+
				"pixels already on the target survive wherever the source is transparent; run `clear` first to "+
				"replace it. The reply reports the cel's position as `x`/`y`, like every other cel op, and where "+
				"the source's pixels landed as `contentX`/`contentY`. `tween` and `oscillate` fill in missing cels from "+
				"the first frame of a range: `tween` moves or fades from one value to another with an easing curve, "+
				"`oscillate` applies a sine offset for a bob, a float or a sway. `reset` sets a cel's position back "+
				"to the origin. Use it after duplicating a frame: a copy carries the source's offsets, so an idle "+
				"pose's 1 px bob silently becomes part of every animation built on it."),
		mcp.WithString("op",
			mcp.Enum("list", "create", "clear", "delete", "move", "copy", "tween", "oscillate", "set", "reset")),
		mcp.WithArray("ops", mcp.Description("Batch: an array of {op, …args} applied in order."), batchItems()),
		mcp.WithString("layer", mcp.Description("Layer name or group path.")),
		mcp.WithNumber("frame", mcp.Description("1-based frame.")),
		mcp.WithString("fromLayer", mcp.Description("For copy: the layer to copy from. Defaults to `layer`.")),
		mcp.WithNumber("fromFrame", mcp.Description("For copy: the frame to copy from. Defaults to `frame`.")),
		mcp.WithNumber("toFrame", mcp.Description("For tween and oscillate: the last frame of the range.")),
		mcp.WithNumber("fromX", mcp.Description("For tween: the starting x position.")),
		mcp.WithNumber("fromY", mcp.Description("For tween: the starting y position.")),
		mcp.WithNumber("toX", mcp.Description("For tween: the ending x position.")),
		mcp.WithNumber("toY", mcp.Description("For tween: the ending y position.")),
		mcp.WithNumber("opacityFrom", mcp.Description("For tween: the starting cel opacity, 0-255.")),
		mcp.WithNumber("opacityTo", mcp.Description("For tween: the ending cel opacity, 0-255.")),
		mcp.WithString("easing", mcp.Description("For tween: linear | ease_in | ease_out | ease_in_out | smoothstep.")),
		mcp.WithNumber("amplitudeX", mcp.Description("For oscillate: pixels of horizontal sway.")),
		mcp.WithNumber("amplitudeY", mcp.Description("For oscillate: pixels of vertical bob.")),
		mcp.WithNumber("period", mcp.Description("For oscillate: frames per full wave.")),
		mcp.WithNumber("phase", mcp.Description("For oscillate: 0-1 offset into the wave.")),
		mcp.WithNumber("dx", mcp.Description("For move: pixels to add to x.")),
		mcp.WithNumber("dy", mcp.Description("For move: pixels to add to y.")),
		mcp.WithNumber("x", mcp.Description("For set: absolute x.")),
		mcp.WithNumber("y", mcp.Description("For set: absolute y.")),
		mcp.WithNumber("opacity", mcp.Description("For set: 0-255.")),
		mcp.WithString("sprite", mcp.Description(spriteNote)),
	)

	srv.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return s.dispatch(ctx, "cel", req, commands)
	})
}

// ---------------------------------------------------------------------------
// frame
// ---------------------------------------------------------------------------

func (s *Service) registerFrame(srv *server.MCPServer) {
	commands := map[string]string{
		"list":         "frame.list",
		"add":          "frame.add",
		"duplicate":    "frame.duplicate",
		"delete":       "frame.delete",
		"set_duration": "frame.setDuration",
		"activate":     "frame.activate",
		"normalise":    "frame.normalise",
	}

	tool := mcp.NewTool("frame",
		mcp.WithDescription(
			"Manage animation frames. Ops: list, add, duplicate, delete, set_duration, activate, normalise.\n"+
				"Batches as `ops`.\n"+indexNote+"\n"+
				"`duplicate` reports `carriedOffsets` — the per-layer positions the copy inherited. "+
				"`normalise` zeroes every cel on a frame, which is what makes a duplicated frame a truly "+
				"neutral starting point."),
		mcp.WithString("op",
			mcp.Enum("list", "add", "duplicate", "delete", "set_duration", "activate", "normalise")),
		mcp.WithArray("ops", mcp.Description("Batch: an array of {op, …args} applied in order."), batchItems()),
		mcp.WithNumber("frame", mcp.Description("1-based frame.")),
		mcp.WithNumber("count", mcp.Description("For add: how many frames.")),
		mcp.WithNumber("durationMs", mcp.Description("For set_duration, milliseconds.")),
		mcp.WithArray("frames", mcp.Description("For set_duration: which frames. Omit for all."), integerItems()),
		mcp.WithString("sprite", mcp.Description(spriteNote)),
	)

	srv.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return s.dispatch(ctx, "frame", req, commands)
	})
}

// ---------------------------------------------------------------------------
// tag
// ---------------------------------------------------------------------------

func (s *Service) registerTag(srv *server.MCPServer) {
	commands := map[string]string{
		"list":   "tag.list",
		"create": "tag.create",
		"update": "tag.update",
		"delete": "tag.delete",
	}

	tool := mcp.NewTool("tag",
		mcp.WithDescription(
			"Manage animation tags. Ops: list, create, update, delete. Batches as `ops`.\n"+
				"`update` retimes or renames an existing tag: `rename` is separate from `to`, because `to` is "+
				"the range end and a rename that looked like a range would silently move the tag.\n"+
				"Untagged frames are unaddressable by a game engine, so a multi-frame sprite with no tag "+
				"is reported as an error by `validate`."),
		mcp.WithString("op", mcp.Enum("list", "create", "update", "delete")),
		mcp.WithArray("ops", mcp.Description("Batch: an array of {op, …args} applied in order."), batchItems()),
		mcp.WithString("name", mcp.Description("Tag name.")),
		mcp.WithString("rename", mcp.Description("For update: the tag's new name.")),
		mcp.WithNumber("from", mcp.Description("First frame, 1-based.")),
		mcp.WithNumber("to", mcp.Description("Last frame, 1-based, inclusive.")),
		mcp.WithString("direction", mcp.Description("forward | reverse | pingpong")),
		mcp.WithString("sprite", mcp.Description(spriteNote)),
	)

	srv.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return s.dispatch(ctx, "tag", req, commands)
	})
}
