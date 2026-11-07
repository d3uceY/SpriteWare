package onpath

import (
	"os"
	"path/filepath"
	"testing"
)

// A PATH entry that points at a scratch folder outlives the folder, so Add
// refuses before it writes anything.
func TestInScratch(t *testing.T) {
	if !inScratch(filepath.Join(os.TempDir(), "go-build123", "b001")) {
		t.Error("a folder under the temp directory must be refused")
	}

	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if inScratch(wd) {
		t.Errorf("%s must not be treated as a scratch folder", wd)
	}
}
