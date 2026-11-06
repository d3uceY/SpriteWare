package export

import (
	"image"
	"image/color"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func canvas(w, h int) *image.NRGBA { return image.NewNRGBA(image.Rect(0, 0, w, h)) }

func paint(i *image.NRGBA, x, y int, c color.NRGBA) { i.SetNRGBA(x, y, c) }

func TestPNGWritesThePathItReports(t *testing.T) {
	i := canvas(4, 4)
	paint(i, 0, 0, color.NRGBA{R: 255, A: 255})

	path := filepath.Join(t.TempDir(), "out.png")
	written, err := PNG(i, path, 1)
	if err != nil {
		t.Fatalf("PNG: %v", err)
	}
	if written != path {
		t.Errorf("reported %q, wrote %q", written, path)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("the file was not written: %v", err)
	}
	if info.Size() == 0 {
		t.Error("the file is empty")
	}
}

// The reference project echoed the pattern instead of the files, so an agent
// could not tell whether the export worked.
func TestFramesReportsConcreteNames(t *testing.T) {
	frames := []*image.NRGBA{canvas(2, 2), canvas(2, 2), canvas(2, 2)}
	dir := t.TempDir()

	written, err := Frames(frames, dir, "f{frame}", 1)
	if err != nil {
		t.Fatalf("Frames: %v", err)
	}
	if len(written) != 3 {
		t.Fatalf("got %d paths, want 3", len(written))
	}
	for i, path := range written {
		if strings.Contains(path, "{frame}") {
			t.Errorf("path %q still holds the pattern", path)
		}
		want := filepath.Join(dir, "f"+string(rune('0'+i))+".png")
		if path != want {
			t.Errorf("path %d = %q, want %q", i, path, want)
		}
		if _, err := os.Stat(path); err != nil {
			t.Errorf("%s was not written", path)
		}
	}
}

func TestFramesNumbersFromZeroByDefault(t *testing.T) {
	written, err := Frames([]*image.NRGBA{canvas(1, 1), canvas(1, 1)}, t.TempDir(), "frame", 1)
	if err != nil {
		t.Fatalf("Frames: %v", err)
	}
	if filepath.Base(written[0]) != "frame0.png" || filepath.Base(written[1]) != "frame1.png" {
		t.Errorf("names = %v, want frame0.png and frame1.png", written)
	}
}

func TestSVGIsDeterministicAndGroupedPerFrame(t *testing.T) {
	frame := canvas(3, 2)
	paint(frame, 0, 0, color.NRGBA{R: 255, A: 255})
	paint(frame, 1, 0, color.NRGBA{R: 255, A: 255})
	paint(frame, 2, 1, color.NRGBA{B: 255, A: 255})

	path := filepath.Join(t.TempDir(), "art.svg")
	if _, err := SVG([]*image.NRGBA{frame, frame}, path); err != nil {
		t.Fatalf("SVG: %v", err)
	}
	first, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	if _, err := SVG([]*image.NRGBA{frame, frame}, path); err != nil {
		t.Fatalf("SVG again: %v", err)
	}
	second, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(first) != string(second) {
		t.Error("two exports of the same art differ; the output is not deterministic")
	}

	body := string(first)
	for _, want := range []string{`<svg`, `id="frame-1"`, `id="frame-2"`, `#ff0000`, `#0000ff`} {
		if !strings.Contains(body, want) {
			t.Errorf("SVG is missing %q", want)
		}
	}
	// Two adjacent red pixels are one rect, not two.
	if strings.Count(body, `fill="#ff0000"`) != 2 {
		t.Errorf("horizontal runs are not merged: %s", body)
	}
}

func TestSVGRefusesAnEmptyDocument(t *testing.T) {
	if _, err := SVG(nil, filepath.Join(t.TempDir(), "x.svg")); err == nil {
		t.Error("an empty export was accepted")
	}
}
