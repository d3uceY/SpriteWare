// Package mcpserver assembles the MCP server from the tool registry.
//
// The transports live in the caller: the app serves HTTP on localhost, the CLI
// stdio for CI, and both get the same tool surface.
package mcpserver

import (
	"github.com/mark3labs/mcp-go/server"

	"github.com/d3uceY/spriteware/internal/tools"
)

// Name is what clients see as the server name.
const Name = "SpriteWare"

// Instructions is the short craft doctrine, sent on connect.
//
// A client that never reads a resource would get the tools and none of the
// discipline. The full chapters are resources.
const Instructions = `Pixel art is not a small image; every pixel is a decision. Work in this loop:

1. Pick constraints before drawing: canvas size, palette, light direction, outline style, view.
2. Draw in one batch, then LOOK at the result. Never report a sprite finished without looking at it.
3. Prefer "look" op=ascii for exactness and op=preview for whether it reads.
4. Run "validate" before calling anything done, and fix what it reports.

Five rules that catch most of what looks generated:
- few colours, one ramp per material, hue-shifted rather than just darker;
- no semi-transparent pixels (they read as blur at 1x);
- no isolated pixels;
- a silhouette that reads as one shape;
- frame timing that varies — hold contact poses longer than passing ones.

Coordinates are 0-based and sprite-absolute. Frames are 1-based. Layer indexes are 0-based from the bottom.`

// New builds the server with every tool registered and the skill as resources.
func New(svc *tools.Service, version string) *server.MCPServer {
	srv := server.NewMCPServer(
		Name,
		version,
		server.WithToolCapabilities(true),
		server.WithResourceCapabilities(false, false),
		server.WithInstructions(Instructions+ResourceIndex()),
	)
	tools.Register(srv, svc)
	RegisterResources(srv)
	return srv
}

// ToolCount reports how many tools were registered.
func ToolCount() int { return len(tools.ToolNames) }
