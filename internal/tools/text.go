package tools

import (
	"github.com/d3uceY/spriteware/internal/craft"
	"github.com/d3uceY/spriteware/internal/font"
	"github.com/d3uceY/spriteware/internal/protocol"
)

// compileText turns one text op into one pixels op.
//
// The font lives in Go so the live extension and headless runner draw the same
// glyphs, and a bad string is refused before any pixel is written.
func compileText(entry map[string]any, index int) (map[string]any, error) {
	text, _ := entry["text"].(string)
	if text == "" {
		return nil, protocol.New(protocol.CodeInvalidArgs, "ops[%d].text is missing", index)
	}
	colour, _ := entry["color"].(string)
	if colour == "" {
		return nil, protocol.New(protocol.CodeInvalidArgs, "ops[%d].color is missing; text needs a colour", index)
	}
	if _, err := craft.ParseHex(colour); err != nil {
		return nil, protocol.New(protocol.CodeInvalidArgs, "ops[%d].color is not a colour: %v", index, err)
	}

	points, err := font.Ink(text, toInt(entry["x"]), toInt(entry["y"]))
	if err != nil {
		return nil, protocol.New(protocol.CodeInvalidArgs, "ops[%d]: %v", index, err)
	}

	target, _ := entry["target"].(map[string]any)
	return map[string]any{
		"kind":   "pixels",
		"color":  colour,
		"points": points,
		"target": target,
	}, nil
}
