package eval

import (
	"image"
	"image/color"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/d3uceY/spriteware/internal/doc"
)

// --- fixtures ---------------------------------------------------------------

func newDump(w, h int, layers ...string) *doc.Dump {
	d := &doc.Dump{
		Sprite:  doc.Sprite{Width: w, Height: h, ColorMode: "rgb"},
		Palette: []string{"#000000", "#c2c3c7"},
		Frames:  []doc.Frame{{DurationMS: 100}},
	}
	for _, name := range layers {
		d.Layers = append(d.Layers, doc.Layer{Name: name, Visible: true, Opacity: 255})
	}
	return d
}

// paint sets a solid rectangle on one layer and frame at sprite coordinates,
// creating the cel the first time and replacing it on a repaint. Frames are
// 1-based, as everywhere else in the repo.
func paint(d *doc.Dump, layer, frame, x, y, w, h int, hex string) {
	for len(d.Frames) < frame {
		d.Frames = append(d.Frames, doc.Frame{DurationMS: 100})
	}
	l := &d.Layers[layer]
	cel := image.NewNRGBA(image.Rect(0, 0, w, h))
	r, g, b := parseHex(hex)
	for py := 0; py < h; py++ {
		for px := 0; px < w; px++ {
			cel.SetNRGBA(px, py, color.NRGBA{R: r, G: g, B: b, A: 255})
		}
	}
	next := doc.Cel{Frame: frame, X: x, Y: y, Width: w, Height: h, Pixels: cel}
	for i := range l.Cels {
		if l.Cels[i].Frame == frame {
			l.Cels[i] = next
			return
		}
	}
	l.Cels = append(l.Cels, next)
}

func parseHex(hex string) (uint8, uint8, uint8) {
	hex = strings.TrimPrefix(hex, "#")
	var v [3]uint8
	for i := 0; i < 3; i++ {
		var n uint8
		for _, c := range hex[i*2 : i*2+2] {
			n *= 16
			switch {
			case c >= '0' && c <= '9':
				n += uint8(c - '0')
			case c >= 'a' && c <= 'f':
				n += uint8(c-'a') + 10
			case c >= 'A' && c <= 'F':
				n += uint8(c-'A') + 10
			}
		}
		v[i] = n
	}
	return v[0], v[1], v[2]
}

func benchmark() *Benchmark {
	return &Benchmark{
		Slug:     "ring-knight",
		Title:    "Ring knight",
		Summary:  "A knight in a box.",
		Revision: 1,
		Setup:    Setup{Canvas: Canvas{Width: 16, Height: 16, ColorMode: "rgb"}, Palette: "pico8"},
		Steps:    []Step{{Title: "The knight", Text: "Draw it."}, {Title: "The breath", Text: "Animate it."}},
		Criteria: []Criterion{
			{ID: "canvas-and-layers", Step: 1, Text: "size and layers", Check: &Check{Kind: CheckCanvas, Width: 16, Height: 16}},
			{ID: "layer-names", Step: 1, Text: "layer names", Check: &Check{Kind: CheckLayers, Names: []string{"shadow", "body"}}},
			{ID: "palette-pure", Step: 1, Text: "palette", Check: &Check{Kind: CheckValidate, Check: "palette", FailOn: "warning"}},
			{ID: "one-silhouette", Step: 1, Text: "one shape", Check: &Check{Kind: CheckSilhouette}},
			{ID: "inside-box", Step: 1, Text: "in the box", Check: &Check{Kind: CheckBounds, Y0: 2, Y1: 13}},
			{ID: "reads-at-1x", Step: 1, Text: "reads", Human: true},
			{ID: "idle-tagged", Step: 2, Text: "tag", Check: &Check{Kind: CheckTag, Name: "idle", Direction: "forward", AllFrames: true}},
			{ID: "idle-timing", Step: 2, Text: "timing", Check: &Check{Kind: CheckTiming, Vary: true}},
			{ID: "idle-motion", Step: 2, Text: "motion", Human: true},
		},
	}
}

// goodDump is a sprite that passes every automatic criterion in benchmark().
func goodDump() *doc.Dump {
	d := newDump(16, 16, "shadow", "body")
	paint(d, 0, 1, 4, 12, 8, 1, "#000000") // shadow, touching the body
	paint(d, 1, 1, 5, 2, 6, 10, "#c2c3c7")
	paint(d, 1, 2, 5, 3, 6, 10, "#c2c3c7")
	d.Frames[1].DurationMS = 260
	d.Tags = []doc.Tag{{Name: "idle", From: 1, To: 2, Direction: "forward"}}
	return d
}

func results(t *testing.T, d *doc.Dump) map[string]Result {
	t.Helper()
	out := map[string]Result{}
	for _, r := range Evaluate(benchmark(), d) {
		out[r.Criterion] = r
	}
	return out
}

// --- loading and validation -------------------------------------------------

func TestTheShippedBenchmarksAreValid(t *testing.T) {
	root, err := Root(".")
	if err != nil {
		t.Fatal(err)
	}
	all, err := Benchmarks(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) == 0 {
		t.Fatal("no benchmarks shipped, so there is nothing to run")
	}
	for _, b := range all {
		if b.Automatic() == 0 {
			t.Errorf("%s has no automatic criterion, so the harness would score nothing", b.Slug)
		}
		if b.Human() == 0 {
			t.Errorf("%s has no human criterion, so nothing about its quality would be judged", b.Slug)
		}
	}
}

func TestACriterionMustBeCheckedOrHuman(t *testing.T) {
	b := benchmark()
	b.Criteria[0].Check = nil
	if err := b.Validate(); err == nil {
		t.Fatal("a criterion with neither a check nor human: true was accepted")
	}

	b = benchmark()
	b.Criteria[5].Check = &Check{Kind: CheckSilhouette}
	b.Criteria[5].Human = true
	if err := b.Validate(); err == nil {
		t.Fatal("a criterion that is both checked and human was accepted")
	}
}

func TestACriterionCannotPointAtAMissingStep(t *testing.T) {
	b := benchmark()
	b.Criteria[0].Step = 3
	err := b.Validate()
	if err == nil || !strings.Contains(err.Error(), "does not exist") {
		t.Fatalf("want a missing-step error, got %v", err)
	}
}

func TestAnUnknownCheckKindIsRejected(t *testing.T) {
	b := benchmark()
	b.Criteria[0].Check = &Check{Kind: "vibes"}
	err := b.Validate()
	if err == nil || !strings.Contains(err.Error(), "is not one of") {
		t.Fatalf("want an unknown-kind error, got %v", err)
	}
}

func TestAnUnknownValidateCheckIsRejected(t *testing.T) {
	b := benchmark()
	b.Criteria[2].Check = &Check{Kind: CheckValidate, Check: "vibes"}
	if err := b.Validate(); err == nil {
		t.Fatal("a validate criterion naming a check the analyzer does not have was accepted")
	}
}

// --- answering criteria -----------------------------------------------------

func TestAGoodSpritePassesEveryAutomaticCriterion(t *testing.T) {
	got := results(t, goodDump())
	for _, c := range benchmark().Criteria {
		if c.Human {
			if !got[c.ID].Human {
				t.Errorf("%s should be left for a human", c.ID)
			}
			continue
		}
		if !got[c.ID].Passed {
			t.Errorf("%s failed on a good sprite: %s", c.ID, got[c.ID].Note)
		}
	}
}

func TestOffPaletteArtFailsThePaletteCriterion(t *testing.T) {
	d := goodDump()
	paint(d, 1, 1, 5, 2, 6, 10, "#ff00ff")
	if got := results(t, d); got["palette-pure"].Passed {
		t.Fatal("magenta is not on the palette but the check passed")
	}
}

func TestTwoPiecesFailTheSilhouetteCriterion(t *testing.T) {
	d := goodDump()
	paint(d, 1, 1, 1, 1, 2, 2, "#c2c3c7") // an island, away from the body
	if got := results(t, d); got["one-silhouette"].Passed {
		t.Fatalf("two separate pieces passed the silhouette check: %s", got["one-silhouette"].Note)
	}
}

func TestArtOutsideTheBoxFails(t *testing.T) {
	d := goodDump()
	paint(d, 1, 1, 5, 0, 6, 1, "#c2c3c7") // row 0 is above y0=2
	got := results(t, d)
	if got["inside-box"].Passed {
		t.Fatal("art on row 0 passed a y0=2 bounds check")
	}
	if !strings.Contains(got["inside-box"].Note, "rows") {
		t.Errorf("the note should say where the art is, got %q", got["inside-box"].Note)
	}
}

func TestUniformTimingFails(t *testing.T) {
	d := goodDump()
	d.Frames[1].DurationMS = 100
	if got := results(t, d); got["idle-timing"].Passed {
		t.Fatal("two frames of the same length passed a vary check")
	}
}

func TestAMissingTagFails(t *testing.T) {
	d := goodDump()
	d.Tags = nil
	if got := results(t, d); got["idle-tagged"].Passed {
		t.Fatal("a sprite with no tag passed the tag check")
	}
}

func TestATagThatMissesTheLastFrameFails(t *testing.T) {
	d := goodDump()
	d.Tags[0].To = 1
	if got := results(t, d); got["idle-tagged"].Passed {
		t.Fatal("a tag covering one of two frames passed an allFrames check")
	}
}

func TestWrongLayerNamesFail(t *testing.T) {
	d := goodDump()
	d.Layers[1].Name = "torso"
	if got := results(t, d); got["layer-names"].Passed {
		t.Fatal("a renamed layer passed the exact-name check")
	}
}

// --- the run record ---------------------------------------------------------

func TestARunKeepsItsTechniqueOutOfTheRecord(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "eval", "benchmarks"), 0o755); err != nil {
		t.Fatal(err)
	}
	run, dir, err := NewRun(root, benchmark(), "2026-11-06", "T4 check-before-finishing", "copilot", "a-model")
	if err != nil {
		t.Fatal(err)
	}
	// The folder name is what a reviewer sees, so it must not name the technique.
	if run.ID != "2026-11-06-ring-knight" {
		t.Errorf("run id = %q, want a neutral 2026-11-06-ring-knight", run.ID)
	}
	body, err := os.ReadFile(filepath.Join(dir, RunFile))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), "T4") {
		t.Error("the technique leaked into run.yaml, so a blind review is impossible")
	}
	if got := Technique(dir); got != "T4 check-before-finishing" {
		t.Errorf("technique = %q", got)
	}
	if !strings.Contains(string(body), "Draw it.") {
		t.Error("the step text was not copied into the run")
	}
	loaded, err := LoadRun(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := VerifyBenchmark(loaded, benchmark()); err != nil {
		t.Errorf("a fresh run should match its benchmark: %v", err)
	}
}

func TestARunWhoseStepTextDriftedIsRejected(t *testing.T) {
	run, dir, err := NewRun(t.TempDir(), benchmark(), "2026-11-06", "T0", "copilot", "")
	if err != nil {
		t.Fatal(err)
	}
	run.Steps[0].Text = "Draw something else."
	if err := SaveRun(dir, run); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadRun(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := VerifyBenchmark(loaded, benchmark()); err == nil {
		t.Fatal("a run that paraphrased the brief was accepted as comparable")
	}
}

func TestASecondRunOnTheSameDayGetsANumber(t *testing.T) {
	root := t.TempDir()
	first, _, err := NewRun(root, benchmark(), "2026-11-06", "T4", "copilot", "")
	if err != nil {
		t.Fatal(err)
	}
	second, _, err := NewRun(root, benchmark(), "2026-11-06", "T8 key poses", "copilot", "")
	if err != nil {
		t.Fatal(err)
	}
	if first.ID != "2026-11-06-ring-knight" || second.ID != "2026-11-06-ring-knight-2" {
		t.Fatalf("ids = %q and %q", first.ID, second.ID)
	}
	for _, id := range []string{first.ID, second.ID} {
		if strings.Contains(id, "T4") || strings.Contains(id, "T8") {
			t.Errorf("run id %q names the technique", id)
		}
	}
}

func TestAReviewRejectsAVerdictNobodyDefined(t *testing.T) {
	r := Review{Verdict: "pretty good"}
	if err := r.Validate(); err == nil {
		t.Fatal("an invented verdict was accepted")
	}
	r = Review{Verdict: VerdictShip, Scores: Scores{Read: 5}}
	if err := r.Validate(); err == nil {
		t.Fatal("a read score of 5 was accepted on a 0-4 scale")
	}
	r = Review{Verdict: VerdictShip, Defects: []string{"smells"}}
	if err := r.Validate(); err == nil {
		t.Fatal("a defect that is not on the tick list was accepted")
	}
}

func TestAnInventedRoleIsNotARole(t *testing.T) {
	if !RoleValid(RoleSource) || !RoleValid(RoleAnimation) {
		t.Fatal("a defined role was rejected")
	}
	if RoleValid("something-i-made-up") {
		t.Fatal("an undefined role was accepted, so a reader has a file they cannot place")
	}
}

func TestAReviewKeepsTheRunItNames(t *testing.T) {
	root := t.TempDir()
	run, dir, err := NewRun(root, benchmark(), "2026-11-06", "T4", "copilot", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := WriteReviewForm(dir, run, benchmark()); err != nil {
		t.Fatal(err)
	}
	review, err := LoadReview(dir)
	if err != nil {
		t.Fatal(err)
	}
	if review.Run != run.ID {
		t.Errorf("the form names run %q, want %q", review.Run, run.ID)
	}
	review.Verdict = VerdictRedraw
	if err := SaveReview(dir, review); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(filepath.Join(dir, ReviewFile))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "run: "+run.ID) {
		t.Error("saving the score dropped the run line from the form")
	}
}

func TestAFormCopiedIntoAnotherRunIsRefused(t *testing.T) {
	root := t.TempDir()
	_, dir, err := NewRun(root, benchmark(), "2026-11-06", "T4", "copilot", "")
	if err != nil {
		t.Fatal(err)
	}
	form := "run: 2026-11-06-ring-knight-9\nverdict: ship\nscores: {read: 4, form: 4, cohesion: 4, appeal: 4}\n"
	if err := os.WriteFile(filepath.Join(dir, ReviewFile), []byte(form), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadReview(dir); err == nil {
		t.Fatal("a form naming a different run was loaded, so one attempt's judgement could score another")
	}
}

func TestATimingCheckOnAnEmptySpriteFailsInsteadOfPanicking(t *testing.T) {
	d := newDump(16, 16, "shadow", "body")
	d.Frames = nil
	if got := results(t, d); got["idle-timing"].Passed {
		t.Fatalf("a sprite with no frames passed a timing check: %s", got["idle-timing"].Note)
	}
}

func TestATickThatIsTickedTwiceIsRejected(t *testing.T) {
	r := Review{Verdict: VerdictFixable, Defects: []string{"banding", "banding"}}
	if err := r.Validate(); err == nil {
		t.Fatal("the same defect ticked twice was accepted, which would weight it twice")
	}
}

func TestAnAnimatedBenchmarkNeedsAMotionScore(t *testing.T) {
	b := benchmark()
	review := &Review{Verdict: VerdictFixable}
	if err := VerifyReview(b, review); err == nil {
		t.Fatal("an animated benchmark accepted a review with no motion score")
	}
	motion := 2
	review.Scores.Motion = &motion
	if err := VerifyReview(b, review); err != nil {
		t.Fatalf("a motion score was rejected: %v", err)
	}

	still := benchmark()
	for i := range still.Criteria {
		if id := still.Criteria[i].ID; id == "idle-tagged" || id == "idle-timing" {
			still.Criteria[i].Check = &Check{Kind: CheckCanvas, Width: 16, Height: 16}
		}
	}
	if err := VerifyReview(still, review); err == nil {
		t.Fatal("a still benchmark accepted a motion score")
	}
}

func TestAReviewOnlyAnswersTheHumanCriteria(t *testing.T) {
	r := &Review{Verdict: VerdictFixable, Criteria: []Result{{Criterion: "palette-pure", Passed: true}}}
	err := VerifyReview(benchmark(), r)
	if err == nil {
		t.Fatal("a review answered an automatic criterion, which the harness owns")
	}
}

func TestTheFormListsOnlyWhatAPersonMustJudge(t *testing.T) {
	run := &Run{ID: "2026-11-06-ring-knight-t4"}
	form := string(ReviewForm(run, benchmark()))
	if !strings.Contains(form, "criterion: reads-at-1x") {
		t.Error("the human criterion is missing from the form")
	}
	if strings.Contains(form, "criterion: palette-pure") {
		t.Error("the form asks a person to answer a criterion the harness answers")
	}
	if !strings.Contains(form, "motion:") {
		t.Error("an animated benchmark should offer a motion score")
	}
}

func TestTheFormLeavesMotionOutForAStill(t *testing.T) {
	b := benchmark()
	b.Criteria = b.Criteria[:6]
	for i := range b.Criteria {
		if b.Criteria[i].Check != nil && b.Criteria[i].Check.Kind == CheckTag {
			b.Criteria[i].Check = &Check{Kind: CheckCanvas, Width: 16, Height: 16}
			b.Criteria[i].ID = "still"
		}
	}
	form := string(ReviewForm(&Run{ID: "x"}, b))
	if strings.Contains(form, "motion: 0") {
		t.Error("a still got a motion score to fill in")
	}
}

func TestTheTableCountsHumanAnswersOnceTheyExist(t *testing.T) {
	root := t.TempDir()
	benchDir := filepath.Join(root, "eval", "benchmarks", "ring-knight")
	if err := os.MkdirAll(benchDir, 0o755); err != nil {
		t.Fatal(err)
	}
	run, dir, err := NewRun(root, benchmark(), "2026-11-06", "T4", "copilot", "")
	if err != nil {
		t.Fatal(err)
	}
	run.Criteria = []Result{{Criterion: "palette-pure", Passed: true}, {Criterion: "one-silhouette"}}
	if err := SaveRun(dir, run); err != nil {
		t.Fatal(err)
	}

	rows, err := Table(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("want one row, got %d", len(rows))
	}
	if rows[0].Passed != 1 || rows[0].Total != 2 || rows[0].Human != 0 {
		t.Errorf("before a review: passed %d of %d, %d human", rows[0].Passed, rows[0].Total, rows[0].Human)
	}
	if rows[0].Verdict != "" || rows[0].Craft != 0 {
		t.Errorf("an unreviewed run claims a verdict %q and craft %d", rows[0].Verdict, rows[0].Craft)
	}

	review := &Review{
		Verdict:  VerdictFixable,
		Criteria: []Result{{Criterion: "reads-at-1x", Passed: true}},
	}
	if err := SaveReview(dir, review); err != nil {
		t.Fatal(err)
	}
	rows, err = Table(root)
	if err != nil {
		t.Fatal(err)
	}
	if rows[0].Passed != 2 || rows[0].Total != 3 || rows[0].Human != 1 {
		t.Errorf("after a review: passed %d of %d, %d human", rows[0].Passed, rows[0].Total, rows[0].Human)
	}
	if rows[0].Craft != 2 {
		t.Errorf("fixable should score 2, got %d", rows[0].Craft)
	}
}

func TestTheTableIsEmptyBeforeAnythingHasRun(t *testing.T) {
	rows, err := Table(t.TempDir())
	if err != nil {
		t.Fatalf("a repository with no runs yet is not an error: %v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("want no rows, got %d", len(rows))
	}
}

func TestVerdictScoreMatchesTheRubric(t *testing.T) {
	if VerdictScore(VerdictShip) != 4 || VerdictScore(VerdictFixable) != 2 || VerdictScore(VerdictRedraw) != 0 {
		t.Fatal("ship/fixable/redraw must map to 4/2/0")
	}
}
