package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// fakeBinary is the name the walker accepts on this platform.
func fakeBinary() string { return binaryNames()[0] }

// plant writes an empty file at dir/name, creating dir.
func plant(t *testing.T, dir, name string) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, nil, 0o755); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	return path
}

func TestWalkForAsepriteFindsNestedBinary(t *testing.T) {
	root := t.TempDir()
	want := plant(t, filepath.Join(root, "SomeVendor", "AsepritePortable"), fakeBinary())

	got, visited := WalkForAseprite([]string{root}, 5*time.Second)
	if got != want {
		t.Fatalf("found %q, want %q (visited %v)", got, want, visited)
	}
	if len(visited) == 0 || visited[0] != root {
		t.Fatalf("visited should start at the root, got %v", visited)
	}
}

func TestWalkForAsepritePrefersFolderNamedAseprite(t *testing.T) {
	root := t.TempDir()
	want := plant(t, filepath.Join(root, "Aseprite"), fakeBinary())
	// Alphabetically earlier and equally close, so only the name bias can skip
	// it.
	plant(t, filepath.Join(root, "AAA", "Tool", "bin"), fakeBinary())

	got, _ := WalkForAseprite([]string{root}, 5*time.Second)
	if got != want {
		t.Fatalf("found %q, want the folder named Aseprite (%q)", got, want)
	}
}

func TestWalkForAsepriteStopsAtDepthLimit(t *testing.T) {
	root := t.TempDir()

	// One level deeper than the limit allows, derived from maxDepth so raising
	// the knob cannot make this test vacuous.
	deepest := root
	for i := 0; i <= maxDepth; i++ {
		deepest = filepath.Join(deepest, "d")
	}
	plant(t, deepest, fakeBinary())

	got, visited := WalkForAseprite([]string{root}, 5*time.Second)
	if got != "" {
		t.Fatalf("found %q below the depth limit", got)
	}
	if len(visited) == 0 {
		t.Fatal("expected the walk to report what it looked in")
	}
}

func TestWalkForAsepriteSkipsNoiseFolders(t *testing.T) {
	root := t.TempDir()
	plant(t, filepath.Join(root, "node_modules", "aseprite"), fakeBinary())
	plant(t, filepath.Join(root, ".git", "aseprite"), fakeBinary())

	got, _ := WalkForAseprite([]string{root}, 5*time.Second)
	if got != "" {
		t.Fatalf("walked into a folder it should skip: %q", got)
	}
}

func TestWalkForAsepriteRespectsDeadline(t *testing.T) {
	root := t.TempDir()
	plant(t, filepath.Join(root, "Aseprite"), fakeBinary())

	got, visited := WalkForAseprite([]string{root}, 0)
	if got != "" || len(visited) != 0 {
		t.Fatalf("a zero budget should look nowhere, got %q / %v", got, visited)
	}
}

func TestWalkForAsepriteIgnoresMissingRoots(t *testing.T) {
	root := t.TempDir()
	want := plant(t, filepath.Join(root, "Aseprite"), fakeBinary())

	got, _ := WalkForAseprite([]string{
		filepath.Join(root, "does-not-exist"),
		"",
		root,
	}, 5*time.Second)
	if got != want {
		t.Fatalf("found %q, want %q", got, want)
	}
}

func TestBinaryInLooksBesideCompanionFiles(t *testing.T) {
	// A folder holding Aseprite's data but keeping the binary in bin/.
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "data"), 0o755); err != nil {
		t.Fatal(err)
	}
	want := plant(t, filepath.Join(dir, "bin"), fakeBinary())

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := binaryIn(dir, entries); got != want {
		t.Fatalf("binaryIn found %q, want %q", got, want)
	}
}

func TestBinaryInNeedsCompanionsToSearchSubfolders(t *testing.T) {
	// Same shape, but nothing identifies it as Aseprite, so the walker must not
	// guess.
	dir := t.TempDir()
	plant(t, filepath.Join(dir, "bin"), fakeBinary())

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := binaryIn(dir, entries); got != "" {
		t.Fatalf("binaryIn guessed %q from a folder with no Aseprite files", got)
	}
}

func TestHasCompanions(t *testing.T) {
	withIni := t.TempDir()
	if err := os.WriteFile(filepath.Join(withIni, "aseprite.ini"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if !hasCompanions(withIni) {
		t.Fatal("aseprite.ini should identify an install")
	}

	withData := t.TempDir()
	if err := os.MkdirAll(filepath.Join(withData, "data"), 0o755); err != nil {
		t.Fatal(err)
	}
	if !hasCompanions(withData) {
		t.Fatal("a data folder should identify an install")
	}

	empty := t.TempDir()
	if hasCompanions(empty) {
		t.Fatal("an empty folder is not an install")
	}
}

func TestSearchRootsAreAbsolute(t *testing.T) {
	roots := SearchRoots()
	if len(roots) == 0 {
		t.Skip("no install roots on this machine")
	}
	for _, root := range roots {
		if !filepath.IsAbs(root) {
			t.Errorf("search root %q is not absolute", root)
		}
	}
}

func TestWalkBudgetIsBounded(t *testing.T) {
	budget := WalkBudget()
	if budget <= 0 || budget > 30*time.Second {
		t.Fatalf("budget %v would make the UI feel hung", budget)
	}
}

func TestSkipDirsCoverTheUsualTraps(t *testing.T) {
	for _, name := range []string{"node_modules", "windows", "temp"} {
		if !skipDirs[name] {
			t.Errorf("%q should be skipped", name)
		}
	}
	if skipDirs["program files"] {
		t.Error("Program Files is exactly where Aseprite lives")
	}
	if strings.Contains(strings.Join(binaryNames(), ","), "..") {
		t.Error("unexpected binary name")
	}
}
