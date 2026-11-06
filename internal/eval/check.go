package eval

import (
	"fmt"
	"image"
	"strings"

	"github.com/d3uceY/spriteware/internal/analyze"
	"github.com/d3uceY/spriteware/internal/doc"
)

// Result is one criterion answered. A human criterion comes back with
// Human set and Passed false until somebody fills it in: the harness must not
// guess on a question it cannot answer.
type Result struct {
	Criterion string `yaml:"criterion" json:"criterion"`
	Step      int    `yaml:"step" json:"step"`
	Passed    bool   `yaml:"pass" json:"pass"`
	Human     bool   `yaml:"human,omitempty" json:"human,omitempty"`
	Note      string `yaml:"note,omitempty" json:"note,omitempty"`
}

// Evaluate answers every automatic criterion against a dump and leaves the
// human ones open. The dump is the whole input: no Aseprite, no PNG on disk.
func Evaluate(b *Benchmark, d *doc.Dump) []Result {
	out := make([]Result, 0, len(b.Criteria))
	for _, c := range b.Criteria {
		r := Result{Criterion: c.ID, Step: c.Step}
		if c.Human {
			r.Human = true
			r.Note = "for a human to judge"
			out = append(out, r)
			continue
		}
		r.Passed, r.Note = answer(*c.Check, d)
		out = append(out, r)
	}
	return out
}

func answer(c Check, d *doc.Dump) (bool, string) {
	switch c.Kind {
	case CheckValidate:
		return answerValidate(c, d)
	case CheckSilhouette:
		return answerSilhouette(d)
	case CheckCanvas:
		return answerCanvas(c, d)
	case CheckLayers:
		return answerLayers(c, d)
	case CheckPalette:
		got := len(d.Palette)
		return got == c.Size, fmt.Sprintf("the palette holds %d colour(s), wanted %d", got, c.Size)
	case CheckFrames:
		got := d.FrameCount()
		if c.Min > 0 && got < c.Min {
			return false, fmt.Sprintf("%d frame(s), at least %d wanted", got, c.Min)
		}
		if c.Max > 0 && got > c.Max {
			return false, fmt.Sprintf("%d frame(s), at most %d wanted", got, c.Max)
		}
		return true, fmt.Sprintf("%d frame(s)", got)
	case CheckTag:
		return answerTag(c, d)
	case CheckTiming:
		if len(d.Frames) == 0 {
			return false, "the sprite has no frames"
		}
		seen := map[int]bool{}
		for _, f := range d.Frames {
			seen[f.DurationMS] = true
		}
		if len(seen) < 2 {
			return false, fmt.Sprintf("every frame lasts %d ms, so the timing never varies", d.Frames[0].DurationMS)
		}
		return true, fmt.Sprintf("%d distinct frame length(s)", len(seen))
	case CheckBounds:
		return answerBounds(c, d)
	}
	return false, "unimplemented check kind " + c.Kind
}

func answerValidate(c Check, d *doc.Dump) (bool, string) {
	threshold, _ := FailOnThreshold(c.FailOn)
	report := analyze.Run(d, analyze.Options{Checks: []string{c.Check}})
	var failing []analyze.Finding
	for _, f := range report.Findings {
		if f.Severity.Order() <= threshold.Order() {
			failing = append(failing, f)
		}
	}
	if len(failing) == 0 {
		if n := len(report.Findings); n > 0 {
			return true, fmt.Sprintf("%s has nothing at or above %s, %d lower finding(s) left to read", c.Check, threshold, n)
		}
		return true, c.Check + " clean"
	}
	return false, summarise(failing)
}

// summarise keeps the note short enough to read in a table but specific enough
// to act on: the first two messages and a count of the rest.
func summarise(findings []analyze.Finding) string {
	parts := make([]string, 0, 3)
	for i, f := range findings {
		if i == 2 {
			parts = append(parts, fmt.Sprintf("and %d more", len(findings)-2))
			break
		}
		parts = append(parts, f.Message)
	}
	return strings.Join(parts, "; ")
}

// answerSilhouette asks the analyzer the question the rubric's "one silhouette"
// line actually means. A plain gaps run cannot fail: separate pieces are only a
// warning unless the caller says every frame must be one shape.
func answerSilhouette(d *doc.Dump) (bool, string) {
	report := analyze.Run(d, analyze.Options{
		Checks: []string{analyze.CheckGaps},
		Expect: &analyze.Expect{SingleComponent: true},
	})
	for _, f := range report.Findings {
		if f.Severity == analyze.Error {
			return false, f.Message
		}
	}
	return true, "every frame is one connected shape"
}

func answerCanvas(c Check, d *doc.Dump) (bool, string) {
	got := fmt.Sprintf("%dx%d", d.Sprite.Width, d.Sprite.Height)
	want := fmt.Sprintf("%dx%d", c.Width, c.Height)
	if got != want {
		return false, "the canvas is " + got + ", wanted " + want
	}
	if c.ColorMode != "" && !strings.EqualFold(d.Sprite.ColorMode, c.ColorMode) {
		return false, fmt.Sprintf("the canvas is %s, wanted %s", d.Sprite.ColorMode, c.ColorMode)
	}
	return true, got
}

func answerLayers(c Check, d *doc.Dump) (bool, string) {
	got := make([]string, 0, len(d.Layers))
	for _, layer := range d.Layers {
		if layer.IsGroup {
			continue
		}
		got = append(got, layer.Name)
	}
	if strings.Join(got, "/") == strings.Join(c.Names, "/") {
		return true, strings.Join(got, ", ")
	}
	return false, fmt.Sprintf("layers are %s, wanted %s", strings.Join(got, ", "), strings.Join(c.Names, ", "))
}

func answerTag(c Check, d *doc.Dump) (bool, string) {
	for _, tag := range d.Tags {
		if tag.Name != c.Name {
			continue
		}
		if c.Direction != "" && !strings.EqualFold(tag.Direction, c.Direction) {
			return false, fmt.Sprintf("tag %s runs %s, wanted %s", c.Name, tag.Direction, c.Direction)
		}
		if c.AllFrames && (tag.From != 1 || tag.To != d.FrameCount()) {
			return false, fmt.Sprintf("tag %s covers frames %d-%d of %d", c.Name, tag.From, tag.To, d.FrameCount())
		}
		return true, fmt.Sprintf("tag %s covers frames %d-%d", c.Name, tag.From, tag.To)
	}
	return false, fmt.Sprintf("there is no tag named %s", c.Name)
}

// answerBounds measures where the ink actually is, rather than trusting a
// per-layer or per-frame figure that a stray pixel would slip past.
func answerBounds(c Check, d *doc.Dump) (bool, string) {
	top, bottom, any := 0, 0, false
	for f := 1; f <= d.FrameCount(); f++ {
		minY, maxY, found := inkRows(d.Composite(f))
		if !found {
			continue
		}
		if !any || minY < top {
			top = minY
		}
		if !any || maxY > bottom {
			bottom = maxY
		}
		any = true
	}
	if !any {
		return false, "the sprite has no visible pixels at all"
	}
	if top < c.Y0 || bottom > c.Y1 {
		return false, fmt.Sprintf("art spans rows %d-%d, wanted %d-%d", top, bottom, c.Y0, c.Y1)
	}
	return true, fmt.Sprintf("art spans rows %d-%d", top, bottom)
}

// inkRows returns the first and last row holding an opaque pixel.
func inkRows(img *image.NRGBA) (top, bottom int, ok bool) {
	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			if img.NRGBAAt(x, y).A != 0 {
				if !ok {
					top, ok = y, true
				}
				bottom = y
				break
			}
		}
	}
	return top, bottom, ok
}
