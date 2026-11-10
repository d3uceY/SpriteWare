package tools

import (
	"path/filepath"
	"strings"
	"testing"
)

// A client that reads only text must still see the data.
func TestOkCarriesTheDataInItsText(t *testing.T) {
	res := ok(map[string]any{"findings": []map[string]any{{"at": map[string]int{"x": 3, "y": 4}}}}, "1 warning")
	text := textOf(*res)
	if !strings.HasPrefix(text, "1 warning\n") || !strings.Contains(text, `"x":3`) {
		t.Fatalf("text lacks the findings: %q", text)
	}
	if res.StructuredContent == nil {
		t.Fatal("structured content was dropped")
	}
}

func TestFramesTargetTreatsAFolderAsAFolder(t *testing.T) {
	dir := t.TempDir()
	cases := []struct {
		path, wantDir, wantBase string
	}{
		{dir, dir, "frame"},
		{filepath.Join(dir, "_frames"), filepath.Join(dir, "_frames"), "frame"},
		{filepath.Join(dir, "hero.png"), dir, "hero"},
	}
	for _, c := range cases {
		gotDir, gotBase := framesTarget(c.path)
		if gotDir != c.wantDir || gotBase != c.wantBase {
			t.Errorf("framesTarget(%q) = %q, %q; want %q, %q", c.path, gotDir, gotBase, c.wantDir, c.wantBase)
		}
	}
}
