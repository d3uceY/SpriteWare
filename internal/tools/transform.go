package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/d3uceY/spriteware/internal/protocol"
)

// transformSchema is hand-written for the same reason as draw: a flat parameter
// set would make "rotate" carry eight fields it never reads.
const transformSchema = `{
  "type": "object",
  "properties": {
    "ops": {
      "type": "array",
      "items": {
        "type": "object",
        "properties": {
          "target": {"type": "object", "properties": {
            "layer": {"type": "string"},
            "frame": {"type": "integer", "minimum": 1}
          }, "additionalProperties": false},
          "kind": {"type": "string", "enum": ["translate", "flip", "rotate", "scale", "crop_to_content"]},
          "dx": {"type": "integer"},
          "dy": {"type": "integer"},
          "direction": {"type": "string", "enum": ["horizontal", "vertical"], "description": "flip only, default horizontal."},
          "angle": {"type": "integer", "description": "rotate only, a multiple of 90. Anything else would resample."},
          "factor": {"type": "integer", "minimum": 1, "description": "scale only, whole number. Nearest neighbour, no smoothing."}
        },
        "required": ["kind"],
        "additionalProperties": false
      }
    },
    "sprite": {"type": "string"}
  },
  "required": ["ops"],
  "additionalProperties": false
}`

func (s *Service) registerTransform(srv *server.MCPServer) {
	tool := mcp.NewTool("transform",
		mcp.WithDescription(
			"Rearrange whole cels. Ops: translate, flip, rotate, scale, crop_to_content.\n"+
				"One call is one transaction, so the user's undo covers the whole batch.\n"+
				"`rotate` only turns by multiples of 90 degrees and `scale` only enlarges by a whole "+
				"number: anything else would resample, and a resampled pixel is not a pixel any more. "+
				"For shrinking, redraw at the target size or use sprite resize.\n"+
				"`crop_to_content` shrinks the cel to its inked bounds and moves it so the art stays "+
				"visually in place."),
		mcp.WithRawInputSchema([]byte(transformSchema)),
	)

	srv.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		raw := req.GetArguments()["ops"]
		list, isList := raw.([]any)
		if !isList || len(list) == 0 {
			return fail(protocol.New(protocol.CodeInvalidArgs, "transform needs a non-empty `ops` array")), nil
		}

		args := map[string]any{"ops": list}
		if sp := req.GetString("sprite", ""); sp != "" {
			args["sprite"] = sp
		}

		var out map[string]any
		if err := s.call(ctx, "transform.apply", args, &out); err != nil {
			return fail(err), nil
		}
		summary := fmt.Sprintf("transformed %d cel(s)", len(list))
		if changed, ok := out["pixelsChanged"].(float64); ok && changed > 0 {
			summary = fmt.Sprintf("transformed %d cel(s), %d pixel(s) moved", len(list), int(changed))
		}
		out["summary"] = summary
		return ok(out, summary), nil
	})
}

// jsonString proves the raw schema parses at startup, not at the first call.
func init() {
	var probe map[string]any
	if err := json.Unmarshal([]byte(transformSchema), &probe); err != nil {
		panic("transform schema is not valid JSON: " + err.Error())
	}
}
