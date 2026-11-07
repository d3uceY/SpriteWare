//go:build !windows

package onpath

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAddStatusRemove(t *testing.T) {
	dir := t.TempDir()
	original := candidateDirs
	candidateDirs = func() []string { return []string{dir} }
	t.Cleanup(func() { candidateDirs = original })

	target := filepath.Join(t.TempDir(), "spriteware")
	if err := os.WriteFile(target, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	if _, err := Add(target); err != nil {
		t.Fatalf("add: %v", err)
	}
	if st := Status(target); !st.OnPath {
		t.Fatalf("not on PATH after add: %+v", st)
	}
	if _, err := Add(target); err != nil {
		t.Fatalf("second add must be a no-op, got %v", err)
	}
	if err := Remove(target); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if st := Status(target); st.OnPath {
		t.Fatalf("still on PATH after remove: %+v", st)
	}
}
