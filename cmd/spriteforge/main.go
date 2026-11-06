// Command spriteforge draws SpriteWare's own artwork the way an assistant would:
// an MCP client of SpriteWare itself.
//
// It launches cmd/spriteware-mcp, creates a document, draws each sprite with the
// `draw` grid op, saves the .aseprite and exports the PNG. Nothing reaches into
// the internals: if a step fails, it fails for an assistant too.
//
//	A frame is one character grid. Multi-frame sprites lay the frames out left
//	to right in one grid, which is what frontend/src/sprite.js expects.
//
//	go run ./cmd/spriteforge
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
)

// art is one sprite: character grids left to right, plus what each character
// means. '.' is transparent. Exactly the shape the `draw` grid op takes.
type art struct {
	name   string
	frames [][]string
	legend map[string]string
}

// The art is the only colour besides the green accent, so it takes the loud end:
// teal, coral, violet, amber.
const (
	tealDeep  = "#0E5C6B"
	teal      = "#2EC4B6"
	amber     = "#FFD166"
	plum      = "#6D2E46"
	coral     = "#FF6B81"
	indigo    = "#2B2A63"
	violet    = "#8B7CF6"
	cyan      = "#4CC9F0"
	ink       = "#22313F"
	white     = "#FFFFFF"
	cheekPink = "#FF6B81"
)

var (
	serverPalette = map[string]string{"K": tealDeep, "B": teal, "A": amber}
	brushPalette  = map[string]string{"K": plum, "B": coral, "A": amber}
	toolsPalette  = map[string]string{"K": indigo, "B": violet, "A": cyan}
	mascotPalette = map[string]string{"K": ink, "B": teal, "W": white, "D": ink, "S": cheekPink}
	wordPalette   = map[string]string{"K": "#1F1F1F"}
	cardPalette   = map[string]string{"K": "#DCDCDC", "W": white}
)

// ---------------------------------------------------------------------------
// Type
//
// Display text is drawn, not set: a 5x7 face with one pixel between glyphs. Only
// the letters the app writes are here, so an unknown character is an error.
// ---------------------------------------------------------------------------

var font = map[rune][]string{
	'A': {".###.", "#...#", "#...#", "#####", "#...#", "#...#", "#...#"},
	'D': {"####.", "#...#", "#...#", "#...#", "#...#", "#...#", "####."},
	'E': {"#####", "#....", "#....", "####.", "#....", "#....", "#####"},
	'F': {"#####", "#....", "#....", "####.", "#....", "#....", "#...."},
	'I': {".###.", "..#..", "..#..", "..#..", "..#..", "..#..", ".###."},
	'L': {"#....", "#....", "#....", "#....", "#....", "#....", "#####"},
	'N': {"#...#", "##..#", "#.#.#", "#..##", "#...#", "#...#", "#...#"},
	'O': {".###.", "#...#", "#...#", "#...#", "#...#", "#...#", ".###."},
	'P': {"####.", "#...#", "#...#", "####.", "#....", "#....", "#...."},
	'R': {"####.", "#...#", "#...#", "####.", "#.#..", "#..#.", "#...#"},
	'S': {".####", "#....", "#....", ".###.", "....#", "....#", "####."},
	'T': {"#####", "..#..", "..#..", "..#..", "..#..", "..#..", "..#.."},
	'V': {"#...#", "#...#", "#...#", "#...#", "#...#", ".#.#.", "..#.."},
	'W': {"#...#", "#...#", "#...#", "#.#.#", "#.#.#", "#.#.#", ".#.#."},
	' ': {".....", ".....", ".....", ".....", ".....", ".....", "....."},
}

// word lays a string out on the 5x7 face, one pixel of gap between glyphs.
func word(s string) ([]string, error) {
	rows := make([]string, 7)
	for i, r := range s {
		glyph, ok := font[r]
		if !ok {
			return nil, fmt.Errorf("no glyph for %q in %q", r, s)
		}
		for y := range rows {
			if i > 0 {
				rows[y] += "."
			}
			rows[y] += strings.ReplaceAll(glyph[y], "#", "K")
		}
	}
	return rows, nil
}

// The card frame: a 24x24 image the CSS slices 8px in from each edge, so one
// asset draws a card of any size.
var cardRows = func() []string {
	rows := make([]string, 24)
	edge := "." + strings.Repeat("K", 22) + "."
	rows[0], rows[23] = edge, edge
	for y := 1; y <= 22; y++ {
		rows[y] = "K" + strings.Repeat("W", 22) + "K"
	}
	return rows
}()

// The mascot blinks. Two frames and a timing rule beat eleven repeated ones: eyes
// open 2s, shut 90ms.
var mascotFrames = [][]string{mascotOpen, mascotBlink}

var sprites = []art{
	{name: "nav-server", frames: [][]string{serverRows}, legend: serverPalette},
	{name: "nav-aseprite", frames: [][]string{brushRows}, legend: brushPalette},
	{name: "nav-tools", frames: [][]string{toolsRows}, legend: toolsPalette},
	{name: "mascot", frames: mascotFrames, legend: mascotPalette},
	wordSprite("word-brand", "SPRITEWARE"),
	wordSprite("word-server", "SERVER"),
	wordSprite("word-aseprite", "ASEPRITE"),
	wordSprite("word-tools", "TOOLS"),
	wordSprite("word-find", "FIND ASEPRITE"),
	{name: "card", frames: [][]string{cardRows}, legend: cardPalette},
}

// wordSprite builds one drawn word. An unknown letter is a programming error, so
// it panics rather than exporting a half-drawn title.
func wordSprite(name, text string) art {
	rows, err := word(text)
	if err != nil {
		panic(err)
	}
	return art{name: name, frames: [][]string{rows}, legend: wordPalette}
}

// A rack of three plates, each with an amber light: a server at 16 px.
var serverRows = []string{
	"................",
	"................",
	"................",
	"..KKKKKKKKKKKK..",
	"..KABBBBBBBBAK..",
	"..KKKKKKKKKKKK..",
	"................",
	"..KKKKKKKKKKKK..",
	"..KABBBBBBBBAK..",
	"..KKKKKKKKKKKK..",
	"................",
	"..KKKKKKKKKKKK..",
	"..KABBBBBBBBAK..",
	"..KKKKKKKKKKKK..",
	"................",
	"................",
}

// A brush held point-down: amber handle, coral bristles.
var brushRows = []string{
	"................",
	"............KK..",
	"...........KAAK.",
	"..........KAAK..",
	".........KKKK...",
	"........KBBK....",
	".......KBBK.....",
	"......KBBK......",
	".....KBBK.......",
	"....KBBK........",
	"...KBBK.........",
	"..KBBK..........",
	"..KKK...........",
	"...K............",
	"................",
	"................",
}

// Four tiles with a cyan window in each.
var toolsRows = []string{
	"................",
	"................",
	"..KKKKK..KKKKK..",
	"..KABAK..KABAK..",
	"..KAAAK..KAAAK..",
	"..KKKKK..KKKKK..",
	"................",
	"................",
	"..KKKKK..KKKKK..",
	"..KABAK..KABAK..",
	"..KAAAK..KAAAK..",
	"..KKKKK..KKKKK..",
	"................",
	"................",
	"................",
	"................",
}

// The mascot: a teal blob with white eyes, coral cheeks and an ink smile.
var mascotOpen = []string{
	"................",
	"................",
	"....KKKKKKKK....",
	"..KKBBBBBBBBKK..",
	".KBBBBBBBBBBBBK.",
	"KBBBBBBBBBBBBBBK",
	"KBBWWWBBBBWWWBBK",
	"KBBWDWBBBBWDWBBK",
	"KBBBBBBBBBBBBBBK",
	"KBSSBBBBBBBBSSBK",
	"KBBBBBKKKKBBBBBK",
	".KBBBBBBBBBBBBK.",
	"..KKBBBBBBBBKK..",
	"....KKKKKKKK....",
	"................",
	"................",
}

// Eyes shut: one ink line where each eye was.
var mascotBlink = []string{
	"................",
	"................",
	"....KKKKKKKK....",
	"..KKBBBBBBBBKK..",
	".KBBBBBBBBBBBBK.",
	"KBBBBBBBBBBBBBBK",
	"KBBBBBBBBBBBBBBK",
	"KBBKKKBBBBKKKBBK",
	"KBBBBBBBBBBBBBBK",
	"KBSSBBBBBBBBSSBK",
	"KBBBBBKKKKBBBBBK",
	".KBBBBBBBBBBBBK.",
	"..KKBBBBBBBBKK..",
	"....KKKKKKKK....",
	"................",
	"................",
}

func main() {
	command := flag.String("mcp", "go", "command that runs the SpriteWare MCP server")
	srcDir := flag.String("src", filepath.Join("assets", "sprites"), "where the .aseprite files are written")
	pngDir := flag.String("png", filepath.Join("frontend", "public", "sprites"), "where the PNGs are written")
	scale := flag.Int("scale", 1, "PNG pixels per art pixel")
	flag.Parse()

	for _, dir := range []string{*srcDir, *pngDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			fail("create %s: %v", dir, err)
		}
	}

	session, err := dial(*command)
	if err != nil {
		fail("%v", err)
	}
	defer session.close()

	for _, a := range sprites {
		if err := session.draw(a, *srcDir, *pngDir, *scale); err != nil {
			fail("%s: %v", a.name, err)
		}
	}
}

// ---------------------------------------------------------------------------
// The MCP connection
// ---------------------------------------------------------------------------

type session struct {
	client *client.Client
	ctx    context.Context
}

// dial starts the server and completes the MCP handshake.
func dial(command string) (*session, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	_ = cancel // the process owns the deadline; the link closes on exit

	// The server defaults to stdio and resolves from the repository root.
	c, err := client.NewStdioMCPClient(command, os.Environ(), "run", "./cmd/spriteware-mcp")
	if err != nil {
		return nil, fmt.Errorf("start the MCP server: %w", err)
	}
	if err := c.Start(ctx); err != nil {
		_ = c.Close()
		return nil, fmt.Errorf("start the MCP transport: %w", err)
	}

	init := mcp.InitializeRequest{}
	init.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
	init.Params.ClientInfo = mcp.Implementation{Name: "spriteforge", Version: "0.3.0"}

	hello, err := c.Initialize(ctx, init)
	if err != nil {
		_ = c.Close()
		return nil, fmt.Errorf("handshake: %w", err)
	}
	fmt.Printf("connected to %s %s\n", hello.ServerInfo.Name, hello.ServerInfo.Version)

	return &session{client: c, ctx: ctx}, nil
}

func (s *session) close() { _ = s.client.Close() }

// draw is one sprite end to end, through the public tools only.
func (s *session) draw(a art, srcDir, pngDir string, scale int) error {
	rows, err := sheet(a)
	if err != nil {
		return err
	}
	width, height := len([]rune(rows[0])), len(rows)

	if _, err := s.call("sprite", map[string]any{
		"op": "new", "width": width, "height": height,
	}); err != nil {
		return fmt.Errorf("sprite new: %w", err)
	}

	if _, err := s.call("draw", map[string]any{
		"label": "spriteforge " + a.name,
		"ops": []any{map[string]any{
			"kind":   "grid",
			"rows":   rows,
			"legend": a.legend,
		}},
	}); err != nil {
		return fmt.Errorf("draw: %w", err)
	}

	asePath := filepath.Join(srcDir, a.name+".aseprite")
	if _, err := s.call("sprite", map[string]any{"op": "save_as", "path": asePath}); err != nil {
		return fmt.Errorf("save %s: %w", asePath, err)
	}

	pngPath := filepath.Join(pngDir, a.name+".png")
	written, err := s.call("export", map[string]any{
		"op": "png", "path": pngPath, "scale": scale,
	})
	if err != nil {
		return fmt.Errorf("export %s: %w", pngPath, err)
	}

	frameWord := "frame"
	if len(a.frames) > 1 {
		frameWord = fmt.Sprintf("%d frames", len(a.frames))
	}
	fmt.Printf("%-14s %2dx%-2d %-10s %s\n", a.name, width/len(a.frames), height, frameWord, firstLine(written))
	return nil
}

// call runs one tool and returns its text answer.
func (s *session) call(tool string, args map[string]any) (string, error) {
	req := mcp.CallToolRequest{}
	req.Params.Name = tool
	req.Params.Arguments = args

	result, err := s.client.CallTool(s.ctx, req)
	if err != nil {
		return "", err
	}
	text := textOf(result)
	if result.IsError {
		return "", fmt.Errorf("%s", text)
	}
	return text, nil
}

// textOf flattens whatever content came back into one readable line.
func textOf(result *mcp.CallToolResult) string {
	for _, content := range result.Content {
		switch v := content.(type) {
		case mcp.TextContent:
			return v.Text
		case *mcp.TextContent:
			return v.Text
		}
	}
	raw, err := json.Marshal(result.StructuredContent)
	if err != nil {
		return "ok"
	}
	return string(raw)
}

func firstLine(s string) string {
	for i, r := range s {
		if r == '\n' {
			return s[:i]
		}
	}
	return s
}

// sheet joins frames left to right into the wide grid `draw` wants, refusing a
// ragged drawing.
func sheet(a art) ([]string, error) {
	if len(a.frames) == 0 {
		return nil, fmt.Errorf("no frames")
	}

	height, width := len(a.frames[0]), 0
	for i, frame := range a.frames {
		if len(frame) != height {
			return nil, fmt.Errorf("frame %d has %d rows, frame 0 has %d", i, len(frame), height)
		}
		for y, row := range frame {
			n := len([]rune(row))
			if y == 0 {
				width = n
			}
			if n != width {
				return nil, fmt.Errorf("frame %d row %d is %d characters but row 0 is %d", i, y, n, width)
			}
		}
	}

	rows := make([]string, height)
	for _, frame := range a.frames {
		for y, row := range frame {
			rows[y] += row
		}
	}
	return rows, nil
}

func fail(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "spriteforge: "+format+"\n", args...)
	os.Exit(1)
}
