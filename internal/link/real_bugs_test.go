package link

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/d3uceY/spriteware/internal/doc"
)

// These tests are the reproducers for dev-docs/mcp-improvements.md. Each one
// fails on the behaviour the document describes, so the fix has a check that
// can go red again.

// Saving a document, opening another, then dumping by path must read the path,
// not whatever happens to be active.
func TestIntegrationDumpReadsTheNamedSprite(t *testing.T) {
	h := realHeadless(t)
	newDoc(t, h, 6, 4)

	path := filepath.Join(t.TempDir(), "first.aseprite")
	call(t, h, "doc.saveAs", map[string]any{"path": path}, &map[string]any{})

	// A second document becomes the active one.
	newDoc(t, h, 3, 3)

	d, err := h.Dump(context.Background(), &doc.Filter{Sprite: path})
	if err != nil {
		t.Fatalf("dump by path: %v", err)
	}
	if d.Sprite.Width != 6 || d.Sprite.Height != 4 {
		t.Fatalf("dump read the %dx%d document, want the named 6x4 one",
			d.Sprite.Width, d.Sprite.Height)
	}
}

// Opening a file that is already open activates it instead of loading a second
// copy, and says which happened.
func TestIntegrationOpenActivatesAnAlreadyOpenDocument(t *testing.T) {
	h := realHeadless(t)
	newDoc(t, h, 6, 4)

	path := filepath.Join(t.TempDir(), "first.aseprite")
	call(t, h, "doc.saveAs", map[string]any{"path": path}, &map[string]any{})

	// A second document becomes the active one.
	newDoc(t, h, 3, 3)

	var opened map[string]any
	call(t, h, "doc.open", map[string]any{"path": path}, &opened)

	if already, _ := opened["alreadyOpen"].(bool); !already {
		t.Fatalf("opening an open file did not report alreadyOpen: %+v", opened)
	}

	var listed []any
	call(t, h, "doc.list", nil, &listed)
	if len(listed) != 2 {
		t.Fatalf("doc.list reports %d documents, want 2: opening an open file made a copy", len(listed))
	}

	d := mustDump(t, h)
	if d.Sprite.Width != 6 || d.Sprite.Height != 4 {
		t.Fatalf("after open the active document is %dx%d, want 6x4",
			d.Sprite.Width, d.Sprite.Height)
	}
}

// replace with a rectangle recolours that rectangle and nothing else.
func TestIntegrationReplaceHonoursItsRegion(t *testing.T) {
	h := realHeadless(t)
	newDoc(t, h, 4, 4)

	draw(t, h, solid(0, 0, 4, 2, "#ff0000"))
	draw(t, h, solid(0, 2, 4, 2, "#ff0000"))

	draw(t, h, map[string]any{
		"kind": "replace", "from": "#ff0000", "to": "#00ff00",
		"x": 0, "y": 0, "width": 4, "height": 2,
	})

	d := mustDump(t, h)
	if _, g, _, _ := pixelOf(t, d, 1, 0, 0); g != 255 {
		t.Fatal("replace skipped a pixel inside its region")
	}
	if r, g, _, _ := pixelOf(t, d, 1, 0, 2); r != 255 || g != 0 {
		t.Fatal("replace changed a pixel outside its region")
	}
}

// A new document can start with the frames an animation needs, and answers to
// the name it was given while it is unsaved.
func TestIntegrationNewDocumentStartsWithFrames(t *testing.T) {
	h := realHeadless(t)

	var out map[string]any
	call(t, h, "doc.new", map[string]any{
		"width": 4, "height": 4, "frames": 3, "name": "walk",
	}, &out)

	d := mustDump(t, h)
	if d.FrameCount() != 3 {
		t.Fatalf("the document has %d frame(s), want 3", d.FrameCount())
	}

	sprite, _ := out["sprite"].(map[string]any)
	if name, _ := sprite["name"].(string); name != "walk" {
		t.Fatalf("the unsaved document is named %q, want walk", name)
	}
}

// Drawing into one frame of a multi-frame document leaves the others alone, and
// the reply names the document it drew into.
func TestIntegrationDrawTargetsOneFrame(t *testing.T) {
	h := realHeadless(t)
	call(t, h, "doc.new", map[string]any{"width": 4, "height": 4, "frames": 2, "name": "shot"}, &map[string]any{})

	var out map[string]any
	call(t, h, "draw.apply", map[string]any{
		"ops": []any{map[string]any{
			"kind": "rect", "x": 0, "y": 0, "width": 2, "height": 2, "color": "#ff0000",
			"target": map[string]any{"frame": 2},
		}},
	}, &out)

	target, _ := out["target"].(map[string]any)
	if target == nil {
		t.Fatal("the reply does not name the document it acted on")
	}
	if id, _ := target["id"].(string); id != "shot" {
		t.Fatalf("the reply names %q, want the document it drew into", id)
	}

	d := mustDump(t, h)
	if _, _, _, a := pixelOf(t, d, 1, 0, 0); a != 0 {
		t.Fatal("frame 1 was painted; the target frame was ignored")
	}
	if r, _, _, a := pixelOf(t, d, 2, 0, 0); r != 255 || a != 255 {
		t.Fatal("frame 2 did not get the block")
	}
}

// remap applies its whole map in one pass: a colour that is both a source and a
// target must not be replaced twice.
func TestIntegrationRemapIsSimultaneous(t *testing.T) {
	h := realHeadless(t)
	newDoc(t, h, 4, 1)

	draw(t, h, map[string]any{"kind": "pixels", "color": "#ff0000", "points": []any{[]any{0, 0}}})
	draw(t, h, map[string]any{"kind": "pixels", "color": "#00ff00", "points": []any{[]any{1, 0}}})

	draw(t, h, map[string]any{
		"kind": "remap",
		"map":  map[string]any{"#ff0000": "#00ff00", "#00ff00": "#0000ff"},
	})

	d := mustDump(t, h)
	if _, g, _, _ := pixelOf(t, d, 1, 0, 0); g != 255 {
		t.Fatalf("red did not become green; it chained to the next source")
	}
	if _, _, b, _ := pixelOf(t, d, 1, 1, 0); b != 255 {
		t.Fatalf("green did not become blue")
	}
}

// frame draws the bevelled box the app's panels share: notched corners, an
// outline, a lit top-left face and a shaded bottom-right one.
func TestIntegrationFrameDrawsABevelledBox(t *testing.T) {
	h := realHeadless(t)
	newDoc(t, h, 6, 6)

	draw(t, h, map[string]any{
		"kind": "frame", "x": 0, "y": 0, "width": 6, "height": 6, "cut": 1,
		"outline": "#000000", "hi": "#ffffff", "lo": "#808080", "body": "#ff0000",
	})

	d := mustDump(t, h)

	if _, _, _, a := pixelOf(t, d, 1, 0, 0); a != 0 {
		t.Fatal("the corner was not notched")
	}
	if r, g, b, a := pixelOf(t, d, 1, 0, 1); r != 0 || g != 0 || b != 0 || a != 255 {
		t.Fatalf("the left edge is (%d,%d,%d,%d), want the outline", r, g, b, a)
	}
	if r, g, b, _ := pixelOf(t, d, 1, 1, 1); r != 255 || g != 255 || b != 255 {
		t.Fatalf("the lit face is (%d,%d,%d), want white", r, g, b)
	}
	if r, g, b, _ := pixelOf(t, d, 1, 4, 4); r != 128 || g != 128 || b != 128 {
		t.Fatalf("the shaded face is (%d,%d,%d), want grey", r, g, b)
	}
	if r, g, b, _ := pixelOf(t, d, 1, 2, 2); r != 255 || g != 0 || b != 0 {
		t.Fatalf("the body is (%d,%d,%d), want red", r, g, b)
	}
}

// openBottom is for a tab: the bottom row is missing, and no line is drawn just
// above where it would have been.
func TestIntegrationFrameOpenBottomLeavesNoLine(t *testing.T) {
	h := realHeadless(t)
	newDoc(t, h, 6, 6)

	draw(t, h, map[string]any{
		"kind": "frame", "x": 0, "y": 0, "width": 6, "height": 6, "cut": 1,
		"outline": "#000000", "body": "#ff0000", "openBottom": true,
	})

	d := mustDump(t, h)
	for x := 0; x < 6; x++ {
		if _, _, _, a := pixelOf(t, d, 1, x, 5); a != 0 {
			t.Fatalf("the bottom row is painted at x=%d; openBottom did not open it", x)
		}
	}
	if r, g, b, _ := pixelOf(t, d, 1, 3, 4); r != 255 || g != 0 || b != 0 {
		t.Fatalf("the row above the opening is (%d,%d,%d), want the body: the edge moved up instead of disappearing", r, g, b)
	}
}

// shift copies the cel moved, painting behind the art by default.
func TestIntegrationShiftMakesADropShadow(t *testing.T) {
	h := realHeadless(t)
	newDoc(t, h, 4, 4)

	draw(t, h, map[string]any{"kind": "pixels", "color": "#ff0000", "points": []any{[]any{1, 1}}})

	var out map[string]any
	call(t, h, "draw.apply", map[string]any{
		"ops": []any{map[string]any{"kind": "shift", "dx": 1, "dy": 1, "color": "#000000"}},
	}, &out)
	if changed, _ := out["pixelsChanged"].(float64); changed != 1 {
		t.Fatalf("shift changed %v pixels, want the one shadow pixel", changed)
	}

	d := mustDump(t, h)
	if r, _, _, a := pixelOf(t, d, 1, 1, 1); r != 255 || a != 255 {
		t.Fatal("shift overwrote the art instead of sitting behind it")
	}
	if r, g, b, a := pixelOf(t, d, 1, 2, 2); r != 0 || g != 0 || b != 0 || a != 255 {
		t.Fatalf("the shadow is (%d,%d,%d,%d), want opaque black", r, g, b, a)
	}
}

// Closing every document refuses to discard unsaved work, and a forced close
// leaves nothing open.
func TestIntegrationCloseAllRefusesUnsavedWork(t *testing.T) {
	h := realHeadless(t)
	newDoc(t, h, 2, 2)

	var out map[string]any
	if err := h.Call(context.Background(), "doc.closeAll", nil, &out); err == nil {
		t.Fatal("closeAll discarded an unsaved document without being told to")
	}

	call(t, h, "doc.closeAll", map[string]any{"force": true}, &out)
	if closed, _ := out["closed"].(float64); closed != 1 {
		t.Fatalf("closed %v document(s), want 1", out["closed"])
	}

	var listed []any
	call(t, h, "doc.list", nil, &listed)
	if len(listed) != 0 {
		t.Fatalf("doc.list still reports %d document(s)", len(listed))
	}
}

// pixelsChanged means pixels whose value changed, so redrawing the same art
// reports nothing.
func TestIntegrationPixelsChangedCountsDifferences(t *testing.T) {
	h := realHeadless(t)
	newDoc(t, h, 8, 8)

	block := solid(1, 1, 4, 4, "#ff0000")
	draw(t, h, block)

	var second map[string]any
	call(t, h, "draw.apply", map[string]any{
		"ops": []any{solid(1, 1, 4, 4, "#ff0000")},
	}, &second)

	if changed, _ := second["pixelsChanged"].(float64); changed != 0 {
		t.Fatalf("redrawing identical pixels reported %v changed, want 0", changed)
	}
	if written, _ := second["pixelsWritten"].(float64); written != 16 {
		t.Fatalf("pixelsWritten = %v, want 16: the write happened, the value did not change", written)
	}
}
