package analyze

import (
	"testing"

	"github.com/d3uceY/spriteware/internal/craft"
	"github.com/d3uceY/spriteware/internal/doc"
)

// uiPanel builds an 8x8 panel with four transparent corners, an outline and a
// body, which is what the ui check is meant to pass.
func uiPanel(t *testing.T) *doc.Dump {
	t.Helper()
	d := newDump(8, 8, "panel")
	img := cel(t, d, 0, 1)

	body := craft.MustHex("#7d929e")
	line := craft.MustHex("#000000")
	fillRect(img, 1, 1, 6, 6, body)
	for px := 1; px < 7; px++ {
		img.SetNRGBA(px, 1, rgba(line))
		img.SetNRGBA(px, 6, rgba(line))
	}
	for py := 1; py < 7; py++ {
		img.SetNRGBA(1, py, rgba(line))
		img.SetNRGBA(6, py, rgba(line))
	}
	return d
}

func TestUICheckPassesAPanel(t *testing.T) {
	d := uiPanel(t)
	report := Run(d, Options{Checks: []string{CheckUI}})

	if got := findingsFor(report, CheckUI); len(got) != 0 {
		t.Fatalf("a clean panel produced %d finding(s): %+v", len(got), got)
	}
}

func TestUICheckRejectsSemiTransparentPixels(t *testing.T) {
	d := uiPanel(t)
	img := cel(t, d, 0, 1)
	img.SetNRGBA(3, 3, rgba(craft.RGB{R: 255, G: 0, B: 0, A: 128}))

	report := Run(d, Options{Checks: []string{CheckUI}})
	got := findingsFor(report, CheckUI)
	if !hasSeverity(got, Error) {
		t.Fatalf("a half-alpha pixel was not an error: %+v", got)
	}
}

func TestUICheckRejectsTooManyColours(t *testing.T) {
	d := newDump(64, 64, "noise")
	img := cel(t, d, 0, 1)
	for i := 0; i < 40; i++ {
		img.SetNRGBA(i, 0, rgba(craft.RGB{R: uint8(i * 6), G: 40, B: 200, A: 255}))
	}

	report := Run(d, Options{Checks: []string{CheckUI}})
	if !hasSeverity(findingsFor(report, CheckUI), Error) {
		t.Fatalf("40 colours passed the ui check: %+v", findingsFor(report, CheckUI))
	}
}

func TestUICheckRejectsASliceOutsideTheCanvas(t *testing.T) {
	d := uiPanel(t)
	d.Slices = []doc.Slice{{Name: "edge", X: 6, Y: 0, Width: 4, Height: 4}}

	report := Run(d, Options{Checks: []string{CheckUI}})
	got := findingsFor(report, CheckUI)
	if !hasSeverity(got, Error) {
		t.Fatalf("a slice sticking out of the canvas passed: %+v", got)
	}
}

func TestUICheckWarnsAboutUnevenMargins(t *testing.T) {
	d := newDump(8, 8, "off")
	img := cel(t, d, 0, 1)
	fillRect(img, 2, 0, 6, 8, craft.MustHex("#7d929e"))

	report := Run(d, Options{Checks: []string{CheckUI}})
	if !hasSeverity(findingsFor(report, CheckUI), Warning) {
		t.Fatalf("art pushed to the right edge passed: %+v", findingsFor(report, CheckUI))
	}
}

func hasSeverity(findings []Finding, want Severity) bool {
	for _, f := range findings {
		if f.Severity == want {
			return true
		}
	}
	return false
}
