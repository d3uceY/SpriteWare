// Package eval is the practice-task harness: it loads a benchmark, records one
// attempt at it, and scores that attempt against the criteria the benchmark
// declares.
//
// Three things stay apart on purpose, because they answer different questions.
// A benchmark says what the task is. A run says what happened. A review says
// whether it was any good. A criterion is either something a tool can answer or
// explicitly marked `human: true`, so "did it follow the brief" never dissolves
// into opinion, and "is it good" never hides inside a pass/fail.
package eval

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/d3uceY/spriteware/internal/analyze"
)

// Check kinds. The vocabulary is closed: a benchmark cannot script a check, it
// can only pick one of these and give it arguments. A check nobody implements
// is a criterion that silently passes, which is worse than no check at all.
const (
	CheckValidate   = "validate"
	CheckSilhouette = "silhouette"
	CheckCanvas     = "canvas"
	CheckLayers     = "layers"
	CheckPalette    = "palette"
	CheckFrames     = "frames"
	CheckTag        = "tag"
	CheckTiming     = "timing"
	CheckBounds     = "bounds"
)

// CheckKinds is the closed list, in the order the error message reads.
var CheckKinds = []string{
	CheckValidate, CheckSilhouette, CheckCanvas, CheckLayers, CheckPalette,
	CheckFrames, CheckTag, CheckTiming, CheckBounds,
}

// Verdicts a reviewer can reach, worst to best.
const (
	VerdictRedraw  = "redraw"
	VerdictFixable = "fixable"
	VerdictShip    = "ship"
)

// VerdictScore turns a verdict into the overall craft number: ship is 4,
// fixable 2, redraw 0. The per-axis scores are kept beside it so two runs with
// the same verdict still show their differences.
func VerdictScore(verdict string) int {
	switch verdict {
	case VerdictShip:
		return 4
	case VerdictFixable:
		return 2
	default:
		return 0
	}
}

// Defects are the tick list from the rubric, plus layer-order: a layer stack
// that hides part of the art is a defect a person can see and no automatic
// check reports, and it is the one that ruins an animation made from a good
// still. Each one names an automatic check where one exists, so a tick the
// checker missed is a bug report.
var Defects = []string{
	"detached", "clipped", "strays", "banding",
	"palette-drift", "antialiasing", "unreadable",
	"mechanical", "off-brief", "flat-detail", "layer-order",
}

var slugPattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

// Canvas is the document a benchmark starts from.
type Canvas struct {
	Width     int    `yaml:"width" json:"width"`
	Height    int    `yaml:"height" json:"height"`
	ColorMode string `yaml:"colorMode" json:"colorMode"`
}

// Setup is everything the operator arranges before the first step: the canvas,
// the palette, and the rules of the session itself.
type Setup struct {
	Canvas  Canvas   `yaml:"canvas" json:"canvas"`
	Palette string   `yaml:"palette" json:"palette"`
	Rules   []string `yaml:"rules" json:"rules,omitempty"`
}

// Step is one handover: the text the model is given, and what to call it.
type Step struct {
	Title string `yaml:"title" json:"title"`
	// Text is handed over byte for byte. A run must carry it verbatim, which is
	// what makes two runs on the same revision comparable at all.
	Text string `yaml:"text" json:"text"`
}

// Check is one automatic criterion. Only the fields its Kind uses are read,
// and Validate rejects the rest, so a typo cannot quietly do nothing.
type Check struct {
	Kind string `yaml:"kind" json:"kind"`
	// validate: which analyze check to run (palette, gaps, strays, ...).
	Check string `yaml:"check,omitempty" json:"check,omitempty"`
	// validate: the worst severity that still passes. Default "error", so a
	// check that only ever warns would pass forever unless the author says what
	// counts. Palette drift is a warning, and a benchmark that cares about it
	// says `failOn: warning`.
	FailOn string `yaml:"failOn,omitempty" json:"failOn,omitempty"`

	// canvas
	Width     int    `yaml:"width,omitempty" json:"width,omitempty"`
	Height    int    `yaml:"height,omitempty" json:"height,omitempty"`
	ColorMode string `yaml:"colorMode,omitempty" json:"colorMode,omitempty"`

	// layers: exact stack, bottom to top, groups excluded.
	Names []string `yaml:"names,omitempty" json:"names,omitempty"`

	// palette: how many entries the document palette holds.
	Size int `yaml:"size,omitempty" json:"size,omitempty"`

	// frames: bounds on the frame count, 0 meaning unbounded.
	Min int `yaml:"min,omitempty" json:"min,omitempty"`
	Max int `yaml:"max,omitempty" json:"max,omitempty"`

	// tag: the tag that must exist, and whether it must cover every frame.
	Name      string `yaml:"name,omitempty" json:"name,omitempty"`
	Direction string `yaml:"direction,omitempty" json:"direction,omitempty"`
	AllFrames bool   `yaml:"allFrames,omitempty" json:"allFrames,omitempty"`

	// timing: frame durations must not all be the same.
	Vary bool `yaml:"vary,omitempty" json:"vary,omitempty"`

	// bounds: every opaque pixel must sit between these rows, inclusive.
	Y0 int `yaml:"y0,omitempty" json:"y0,omitempty"`
	Y1 int `yaml:"y1,omitempty" json:"y1,omitempty"`
}

func (c *Check) validate(where string) error {
	known := false
	for _, kind := range CheckKinds {
		if c.Kind == kind {
			known = true
		}
	}
	if !known {
		return fmt.Errorf("%s: check kind %q is not one of %s", where, c.Kind, strings.Join(CheckKinds, ", "))
	}
	switch c.Kind {
	case CheckValidate:
		for _, name := range analyze.AllChecks {
			if c.Check == name {
				return validateFailOn(where, c.FailOn)
			}
		}
		return fmt.Errorf("%s: validate check %q is not one of %s", where, c.Check, strings.Join(analyze.AllChecks, ", "))
	case CheckCanvas:
		if c.Width <= 0 || c.Height <= 0 {
			return fmt.Errorf("%s: a canvas check needs width and height", where)
		}
	case CheckLayers:
		if len(c.Names) == 0 {
			return fmt.Errorf("%s: a layers check needs names", where)
		}
	case CheckPalette:
		if c.Size <= 0 {
			return fmt.Errorf("%s: a palette check needs size", where)
		}
	case CheckTag:
		if strings.TrimSpace(c.Name) == "" {
			return fmt.Errorf("%s: a tag check needs a name", where)
		}
	case CheckTiming:
		if !c.Vary {
			return fmt.Errorf("%s: the only timing check is vary: true", where)
		}
	case CheckBounds:
		if c.Y1 < c.Y0 {
			return fmt.Errorf("%s: bounds y1 %d is above y0 %d", where, c.Y1, c.Y0)
		}
	}
	return nil
}

// FailOnThreshold turns a failOn name into the severity order it allows, with
// an empty value meaning errors only.
func FailOnThreshold(name string) (analyze.Severity, bool) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "", string(analyze.Error):
		return analyze.Error, true
	case string(analyze.Warning):
		return analyze.Warning, true
	case string(analyze.Note):
		return analyze.Note, true
	}
	return analyze.Error, false
}

func validateFailOn(where, name string) error {
	if _, ok := FailOnThreshold(name); !ok {
		return fmt.Errorf("%s: failOn %q must be error, warning or note", where, name)
	}
	return nil
}

// Criterion is one line of the pass bar, judged either by a tool or by a human.
type Criterion struct {
	ID   string `yaml:"id" json:"id"`
	Step int    `yaml:"step" json:"step"`
	Text string `yaml:"text" json:"text"`
	// Check and Human are exclusive: every criterion must say which one it is.
	Check *Check `yaml:"check,omitempty" json:"check,omitempty"`
	Human bool   `yaml:"human,omitempty" json:"human,omitempty"`
}

// Benchmark is one fixed task: setup, the text of each step, and the pass bar.
type Benchmark struct {
	Slug     string      `yaml:"-" json:"slug"`
	Title    string      `yaml:"title" json:"title"`
	Summary  string      `yaml:"summary" json:"summary"`
	Revision int         `yaml:"revision" json:"revision"`
	Tags     []string    `yaml:"tags" json:"tags,omitempty"`
	Setup    Setup       `yaml:"setup" json:"setup"`
	Steps    []Step      `yaml:"steps" json:"steps"`
	Criteria []Criterion `yaml:"criteria" json:"criteria"`
}

// Human reports how many criteria a person has to judge.
func (b *Benchmark) Human() int {
	n := 0
	for _, c := range b.Criteria {
		if c.Human {
			n++
		}
	}
	return n
}

// Automatic reports how many criteria the harness answers itself.
func (b *Benchmark) Automatic() int { return len(b.Criteria) - b.Human() }

// Validate rejects a benchmark that would score something other than what it
// says it scores. Every rule here is one an author could get wrong in a way
// that only shows up as a criterion that always passes.
func (b *Benchmark) Validate() error {
	if !slugPattern.MatchString(b.Slug) {
		return fmt.Errorf("slug %q must be lowercase words joined by single hyphens", b.Slug)
	}
	if strings.TrimSpace(b.Title) == "" {
		return fmt.Errorf("%s: title is empty", b.Slug)
	}
	if strings.TrimSpace(b.Summary) == "" {
		return fmt.Errorf("%s: summary is empty", b.Slug)
	}
	if b.Revision < 1 {
		return fmt.Errorf("%s: revision must be 1 or more, got %d", b.Slug, b.Revision)
	}
	if b.Setup.Canvas.Width <= 0 || b.Setup.Canvas.Height <= 0 {
		return fmt.Errorf("%s: setup needs a canvas width and height", b.Slug)
	}
	if len(b.Steps) == 0 {
		return fmt.Errorf("%s: no steps", b.Slug)
	}
	for i, step := range b.Steps {
		if strings.TrimSpace(step.Title) == "" {
			return fmt.Errorf("%s: step %d has no title", b.Slug, i+1)
		}
		if strings.TrimSpace(step.Text) == "" {
			return fmt.Errorf("%s: step %d has no text", b.Slug, i+1)
		}
	}
	if len(b.Criteria) == 0 {
		return fmt.Errorf("%s: no criteria, so nothing would be scored", b.Slug)
	}
	seen := map[string]bool{}
	for i, c := range b.Criteria {
		where := fmt.Sprintf("%s: criterion %d", b.Slug, i+1)
		if !slugPattern.MatchString(c.ID) {
			return fmt.Errorf("%s: id %q must be lowercase words joined by single hyphens", where, c.ID)
		}
		if seen[c.ID] {
			return fmt.Errorf("%s: duplicate id %q", where, c.ID)
		}
		seen[c.ID] = true
		if strings.TrimSpace(c.Text) == "" {
			return fmt.Errorf("%s (%s): text is empty", where, c.ID)
		}
		if c.Step < 1 || c.Step > len(b.Steps) {
			return fmt.Errorf("%s (%s): step %d does not exist, the benchmark has %d", where, c.ID, c.Step, len(b.Steps))
		}
		if (c.Check == nil) != c.Human {
			return fmt.Errorf("%s (%s): give it a check or mark it human: true, not both and not neither", where, c.ID)
		}
		if c.Check != nil {
			if err := c.Check.validate(fmt.Sprintf("%s (%s)", where, c.ID)); err != nil {
				return err
			}
		}
	}
	return nil
}
