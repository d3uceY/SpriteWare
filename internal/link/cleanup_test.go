package link

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// scratchLuaDirs counts the unpacked script folders left in the temp directory.
func scratchLuaDirs(t *testing.T) int {
	t.Helper()
	entries, err := os.ReadDir(os.TempDir())
	if err != nil {
		t.Fatalf("read the temp directory: %v", err)
	}
	n := 0
	for _, e := range entries {
		if e.IsDir() && strings.HasPrefix(e.Name(), "spriteware-lua-") {
			n++
		}
	}
	return n
}

// Every launch used to leave another spriteware-lua-* folder behind.
func TestTheUnpackedLuaIsRemovedWhenTheLinkCloses(t *testing.T) {
	before := scratchLuaDirs(t)

	h, err := NewHeadless(HeadlessOptions{Exe: filepath.Join(t.TempDir(), "no-such-aseprite")})
	if err != nil {
		t.Fatal(err)
	}

	// The call fails, but it is what unpacks the scripts, which is the point.
	err = h.Call(context.Background(), "doc.info", nil, nil)
	if err == nil {
		t.Fatal("starting a missing Aseprite should fail")
	}

	if err := h.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	if after := scratchLuaDirs(t); after > before {
		t.Fatalf("the temp directory grew from %d to %d script folders; the unpacked copy was not removed", before, after)
	}
}

// A launch failure must name the folder it handed to Aseprite, or the user
// cannot tell binary from scripts.
func TestStartupFailureNamesTheScriptFolder(t *testing.T) {
	h, err := NewHeadless(HeadlessOptions{Exe: filepath.Join(t.TempDir(), "no-such-aseprite")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = h.Close() })

	err = h.Call(context.Background(), "doc.info", nil, nil)
	if err == nil {
		t.Fatal("expected a failure")
	}
	if !strings.Contains(err.Error(), "spriteware-lua-") {
		t.Errorf("the error does not name the Lua folder: %v", err)
	}
}

// A caller that supplies its own scripts is editing them and must keep them.
func TestAPointedAtLuaFolderIsNotDeleted(t *testing.T) {
	root := t.TempDir()
	for _, rel := range []string{"extension/spriteware.lua", "headless/runner.lua"} {
		path := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("-- mine\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	h, err := NewHeadless(HeadlessOptions{
		Exe:  filepath.Join(t.TempDir(), "no-such-aseprite"),
		Root: root,
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = h.Call(context.Background(), "doc.info", nil, nil)
	if err := h.Close(); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(filepath.Join(root, "extension", "spriteware.lua")); err != nil {
		t.Fatalf("the caller's own scripts were deleted: %v", err)
	}
}
