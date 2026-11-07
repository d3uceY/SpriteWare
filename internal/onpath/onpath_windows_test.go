//go:build windows

package onpath

import "testing"

// The Windows side of this package writes the registry, so the test covers the
// text handling that decides whether an entry is already there.
func TestSplitPathAndContains(t *testing.T) {
	entries := splitPath(`C:\Tools;;C:\Program Files\SpriteWare ;C:\Users\%USERNAME%\bin`)

	if !contains(entries, `c:\tools`) {
		t.Fatalf("case-insensitive match failed: %#v", entries)
	}
	if !contains(entries, `C:\Program Files\SpriteWare`) {
		t.Fatalf("trailing space not trimmed: %#v", entries)
	}
	if contains(entries, `C:\nope`) {
		t.Fatal("matched a folder that is not in the value")
	}
	for _, entry := range entries {
		if entry == "" {
			t.Fatalf("the empty entry between two separators survived: %#v", entries)
		}
	}
}
