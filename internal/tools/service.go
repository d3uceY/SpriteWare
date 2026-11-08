// Package tools is the agent-facing surface: grouped tools with an op enum,
// and a batch array wherever a caller would otherwise loop.
//
// Two rules run through every handler:
//
//  1. Failures are tool results, never protocol errors. The model has to see
//     the message to recover.
//  2. Every result carries data. A summary is an extra field, never a
//     replacement for the structure.
package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/d3uceY/spriteware/internal/config"
	"github.com/d3uceY/spriteware/internal/link"
	"github.com/d3uceY/spriteware/internal/protocol"
)

// Service holds everything a handler needs.
type Service struct {
	cfg   *config.Store
	links *link.Selector

	mu sync.Mutex
	// scratchRoot is where dump directories are created. Empty means the OS temp
	// directory.
	scratchRoot string
	// bridge reports the app's bridge state; it lives in the app, so doctor asks
	// rather than carrying a field that was always zero.
	bridge func() (link.BridgeState, bool)
}

// SetBridgeReporter wires doctor up to the component that owns the bridge.
func (s *Service) SetBridgeReporter(report func() (link.BridgeState, bool)) {
	s.mu.Lock()
	s.bridge = report
	s.mu.Unlock()
}

// bridgeState returns what the bridge knows, if one is running.
func (s *Service) bridgeState() (link.BridgeState, bool) {
	s.mu.Lock()
	report := s.bridge
	s.mu.Unlock()
	if report == nil {
		return link.BridgeState{}, false
	}
	return report()
}

// New builds a Service.
func New(cfg *config.Store, links *link.Selector) *Service {
	return &Service{cfg: cfg, links: links}
}

// Register adds every tool to the MCP server.
func Register(srv *server.MCPServer, svc *Service) {
	svc.registerDoctor(srv)
	svc.registerSprite(srv)
	svc.registerLayer(srv)
	svc.registerCel(srv)
	svc.registerFrame(srv)
	svc.registerTag(srv)
	svc.registerSlice(srv)
	svc.registerDraw(srv)
	svc.registerSelect(srv)
	svc.registerTransform(srv)
	svc.registerLook(srv)
	svc.registerValidate(srv)
	svc.registerPalette(srv)
	svc.registerExport(srv)
}

// ToolInfo is one entry of the published surface: the name the model calls and a
// short line for the Tools screen.
type ToolInfo struct {
	Name  string `json:"name"`
	Blurb string `json:"blurb"`
}

// Catalog is the surface in registration order.
var Catalog = []ToolInfo{
	{"doctor", "Reports the connection and says what to fix."},
	{"sprite", "Open, create, save, close and inspect documents."},
	{"layer", "Create, rename, reorder, group and merge layers."},
	{"cel", "The pixels one layer holds on one frame."},
	{"frame", "Add, duplicate, retime and reset animation frames."},
	{"tag", "Name the frame ranges a game engine can address."},
	{"slice", "Named rectangles: 9-patch regions and bounding boxes."},
	{"draw", "Pixels, shapes and readable character grids."},
	{"select", "Clip the next edits to one rectangle."},
	{"transform", "Flip, rotate, scale, translate and crop whole cels."},
	{"look", "See the work: preview, ascii, filmstrip, diff, onion, compare."},
	{"validate", "Lint for palette drift, stray pixels and broken rigs."},
	{"palette", "Ramps, analysis, snapping to the palette, adopting the art's colours."},
	{"export", "PNG, frame sequences, SVG, GIF, packed sheets and tilesets."},
}

// ToolNames is the same list, name only.
var ToolNames []string

func init() {
	for _, t := range Catalog {
		ToolNames = append(ToolNames, t.Name)
	}
}

// maxTools is the hard ceiling from the design notes.
const maxTools = 24

// ---------------------------------------------------------------------------
// Result helpers
// ---------------------------------------------------------------------------

// ok returns a structured result with a human-readable fallback.
func ok(structured any, summary string) *mcp.CallToolResult {
	return mcp.NewToolResultStructured(structured, summary)
}

// fail turns any error into a tool result the model can read and act on. The
// code comes first so a client can branch on it; remediation must be present.
func fail(err error) *mcp.CallToolResult {
	e := protocol.AsError(err)
	return mcp.NewToolResultError(e.Render())
}

// ---------------------------------------------------------------------------
// Argument helpers
// ---------------------------------------------------------------------------

// op returns the op argument, defaulting when absent.
func op(req mcp.CallToolRequest, def string) string {
	v := strings.TrimSpace(req.GetString("op", def))
	if v == "" {
		return def
	}
	return v
}

// decodeArg pulls one argument out and unmarshals it into target.
//
// mcp-go hands back generic maps; going through JSON keeps one decoding path.
func decodeArg(req mcp.CallToolRequest, key string, target any) error {
	raw, exists := req.GetArguments()[key]
	if !exists || raw == nil {
		return nil
	}
	body, err := json.Marshal(raw)
	if err != nil {
		return protocol.New(protocol.CodeInvalidArgs, "could not read %q: %v", key, err)
	}
	if err := json.Unmarshal(body, target); err != nil {
		return protocol.New(protocol.CodeInvalidArgs, "%q has the wrong shape: %v", key, err)
	}
	return nil
}

// linkFor returns the connection the tools should use, starting it if needed.
//
// No separate "connect" tool: it could become unreachable and take every other
// tool with it, so each tool connects itself and reports failure after trying.
func (s *Service) linkFor() (link.Link, error) { return s.links.Current() }

// call runs one Lua command against the current link.
func (s *Service) call(ctx context.Context, cmd string, args any, out any) error {
	l, err := s.linkFor()
	if err != nil {
		return err
	}
	return l.Call(ctx, cmd, args, out)
}

// ---------------------------------------------------------------------------
// Paths
// ---------------------------------------------------------------------------

// resolveOutput makes a user path absolute, refusing parent-directory traversal
// so nothing lands outside the project.
func resolveOutput(p string) (string, error) {
	p = strings.TrimSpace(p)
	if p == "" {
		return "", protocol.New(protocol.CodeInvalidArgs, "a path is required").
			WithRemediation("pass the destination path, for example C:/projects/hero.png")
	}
	if strings.Contains(filepath.ToSlash(p), "../") || filepath.ToSlash(p) == ".." {
		return "", protocol.New(protocol.CodeInvalidArgs, "paths may not contain '..'").
			WithDetail("path", p)
	}
	abs, err := filepath.Abs(p)
	if err != nil {
		return "", protocol.New(protocol.CodeInvalidArgs, "could not resolve %q: %v", p, err)
	}
	return abs, nil
}

// scratchDir creates a per-request directory for dump files.
func (s *Service) scratchDir() (string, error) {
	s.mu.Lock()
	root := s.scratchRoot
	s.mu.Unlock()
	return os.MkdirTemp(root, "spriteware-dump-*")
}

// ---------------------------------------------------------------------------
// Schema fragments
// ---------------------------------------------------------------------------

// coordinateNote is repeated in every tool that takes pixels: leaving the origin
// unstated is how a caller draws in the wrong place.
const coordinateNote = "Coordinates are 0-based and sprite-absolute: (0,0) is the top-left corner of the canvas, x right, y down. Layer offsets never shift them."

// indexNote states the one numbering convention, in and out.
const indexNote = "Layer indexes are 0-based with 0 at the bottom, and the same value is reported back. Frames are 1-based."

// spriteNote lists every way a caller can name a document.
const spriteNote = "Target sprite by absolute path, by the `id` a reply gave it, or by the name passed to `sprite op=new`. Omit to use the active one."

// unknownOp lists what was expected.
func unknownOp(tool, got string, allowed ...string) error {
	return protocol.New(protocol.CodeInvalidArgs,
		"%s has no op %q", tool, got).
		WithDetail("allowed", allowed).
		WithRemediation("use one of: %s", strings.Join(allowed, ", "))
}
