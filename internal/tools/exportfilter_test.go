package tools

import (
	"strings"
	"testing"

	"github.com/d3uceY/spriteware/internal/doc"
	"github.com/d3uceY/spriteware/internal/protocol"
)

// dumpWith builds the smallest document the filters need: frames, tags, layer
// names.
func dumpWith(frames int, tags []doc.Tag, layers []string) *doc.Dump {
	d := &doc.Dump{
		Sprite: doc.Sprite{Width: 4, Height: 4},
		Tags:   tags,
	}
	for i := 0; i < frames; i++ {
		d.Frames = append(d.Frames, doc.Frame{DurationMS: 100})
	}
	for _, name := range layers {
		d.Layers = append(d.Layers, doc.Layer{Name: name})
	}
	return d
}

func TestNoFilterExportsEveryFrame(t *testing.T) {
	d := dumpWith(3, nil, nil)
	frames, _, err := exportFrames(d, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(frames) != 3 || frames[0] != 1 || frames[2] != 3 {
		t.Fatalf("frames = %v, want 1..3", frames)
	}
}

// One tag must export one animation, in document order.
func TestATagKeepsOnlyItsFrames(t *testing.T) {
	d := dumpWith(6, []doc.Tag{{Name: "walk", From: 2, To: 4}, {Name: "idle", From: 5, To: 6}}, nil)

	frames, _, err := exportFrames(d, []string{"walk"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(frames) != 3 || frames[0] != 2 || frames[2] != 4 {
		t.Fatalf("frames = %v, want 2..4", frames)
	}
}

// Two tags that overlap must not export the shared frame twice.
func TestOverlappingTagsDoNotDuplicateFrames(t *testing.T) {
	d := dumpWith(6, []doc.Tag{{Name: "walk", From: 1, To: 4}, {Name: "run", From: 3, To: 6}}, nil)

	frames, _, err := exportFrames(d, []string{"walk", "run"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[int]bool{}
	for _, f := range frames {
		if seen[f] {
			t.Fatalf("frame %d appears twice in %v", f, frames)
		}
		seen[f] = true
	}
	if len(frames) != 6 {
		t.Fatalf("frames = %v, want all six", frames)
	}
}

// A typo must not export the whole document.
func TestAnUnknownTagIsRefusedWithTheRealNames(t *testing.T) {
	d := dumpWith(4, []doc.Tag{{Name: "walk", From: 1, To: 2}}, nil)

	_, _, err := exportFrames(d, []string{"wlak"}, nil)
	if err == nil {
		t.Fatal("a misspelled tag was accepted")
	}
	// The names live in the remediation and details the caller is shown.
	wire := protocol.AsError(err)
	if !strings.Contains(wire.Remediation, "walk") {
		t.Errorf("the remediation does not list the available tags: %+v", wire)
	}
	details, _ := wire.Details["tags"].([]string)
	if len(details) != 1 || details[0] != "walk" {
		t.Errorf("details carry %v, want the tag names", wire.Details["tags"])
	}
}

func TestTagNamesAreMatchedCaseInsensitively(t *testing.T) {
	d := dumpWith(4, []doc.Tag{{Name: "Walk", From: 1, To: 2}}, nil)
	if _, _, err := exportFrames(d, []string{"walk"}, nil); err != nil {
		t.Fatalf("a case difference should not break an export: %v", err)
	}
}

func TestLayersAreResolvedByName(t *testing.T) {
	d := dumpWith(2, nil, []string{"base", "shading", "outline"})

	if _, _, err := exportFrames(d, nil, []string{"base", "outline"}); err != nil {
		t.Fatalf("valid layers were refused: %v", err)
	}

	_, _, err := exportFrames(d, nil, []string{"nope"})
	if err == nil {
		t.Fatal("an unknown layer was accepted")
	}
	if wire := protocol.AsError(err); !strings.Contains(wire.Remediation, "outline") {
		t.Errorf("the remediation does not list the real layers: %+v", wire)
	}
}

// A tag running past the end must not produce a panicking index.
func TestATagPastTheLastFrameIsClamped(t *testing.T) {
	d := dumpWith(3, []doc.Tag{{Name: "walk", From: 1, To: 99}}, nil)

	frames, _, err := exportFrames(d, []string{"walk"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range frames {
		if f < 1 || f > 3 {
			t.Fatalf("frame %d is outside the document", f)
		}
	}
}

// doctor must answer even when nothing works, with one line saying what to do.
func TestDoctorDirectiveAlwaysSaysWhatToDoNext(t *testing.T) {
	cases := []DoctorReport{
		{Connected: true},
		{Remediation: "Set asepritePath in the settings."},
		{},
	}
	for _, r := range cases {
		if got := doctorDirective(r); strings.TrimSpace(got) == "" {
			t.Errorf("no directive for %+v", r)
		}
	}
	if got := doctorDirective(cases[1]); got != cases[1].Remediation {
		t.Errorf("the directive should hand over the remediation, got %q", got)
	}
}
