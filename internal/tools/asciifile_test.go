package tools

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

// writePNG puts a small picture on disk: red at (0,0), blue at (1,0), nothing
// elsewhere.
func writePNG(t *testing.T, dir string) string {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, 3, 2))
	img.SetNRGBA(0, 0, color.NRGBA{R: 255, A: 255})
	img.SetNRGBA(1, 0, color.NRGBA{B: 255, A: 255})

	path := filepath.Join(dir, "cell.png")
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		t.Fatalf("encode: %v", err)
	}
	return path
}

// A PNG can be read as a grid without opening a document, and the glyphs are
// ordered by colour so two reads agree.
func TestASCIIFileReadsAPNGWithoutADocument(t *testing.T) {
	path := writePNG(t, t.TempDir())

	payload, text, err := asciiFile(path, image.Rectangle{}, false)
	if err != nil {
		t.Fatalf("asciiFile: %v", err)
	}
	if payload["path"] != path {
		t.Fatalf("the payload does not name the file it read: %v", payload["path"])
	}
	if payload["width"] != 3 || payload["height"] != 2 {
		t.Fatalf("grid is %vx%v, want 3x2", payload["width"], payload["height"])
	}

	rows := payload["rows"].([]string)
	legend := payload["legend"].(map[string]string)
	first := string(rows[0][0])
	second := string(rows[0][1])
	if legend[first] != "#ff0000" || legend[second] != "#0000ff" {
		t.Fatalf("row 0 is %q with legend %v, want red then blue left to right", rows[0], legend)
	}
	if rows[0][2] != '.' {
		t.Fatalf("row 0 column 2 is %q, want the transparent glyph", string(rows[0][2]))
	}

	// One glyph per colour, and blue sorts first so it claims the first glyph.
	if legend["!"] != "#0000ff" {
		t.Fatalf("the legend is not ordered by colour: %v", legend)
	}

	again, _, err := asciiFile(path, image.Rectangle{}, false)
	if err != nil {
		t.Fatalf("asciiFile: %v", err)
	}
	if again["rows"].([]string)[0] != rows[0] {
		t.Fatal("two reads of one file disagree")
	}
	if text == "" {
		t.Fatal("the text form is empty")
	}
}

func TestASCIIFileRegionKeepsTheOrigin(t *testing.T) {
	path := writePNG(t, t.TempDir())

	payload, _, err := asciiFile(path, image.Rect(1, 0, 3, 1), false)
	if err != nil {
		t.Fatalf("asciiFile: %v", err)
	}
	origin := payload["origin"].(map[string]int)
	if origin["x"] != 1 || origin["y"] != 0 {
		t.Fatalf("origin is %v, want (1,0)", origin)
	}
	if payload["width"] != 2 || payload["height"] != 1 {
		t.Fatalf("grid is %vx%v, want 2x1", payload["width"], payload["height"])
	}
}

func TestASCIIFileRejectsAMissingPath(t *testing.T) {
	_, _, err := asciiFile(filepath.Join(t.TempDir(), "gone.png"), image.Rectangle{}, false)
	if err == nil {
		t.Fatal("a missing file was read without an error")
	}
}
