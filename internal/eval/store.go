package eval

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// Folder names inside the repository. The data lives beside the code because a
// benchmark whose runs are somewhere else is a benchmark nobody can check.
const (
	BenchmarkFile = "benchmark.yaml"
	RunFile       = "run.yaml"
	ReviewFile    = "review.yaml"
	// TechniqueFile holds the one thing a blind review must not see. It sits in
	// the run folder so it cannot be separated from the run it describes.
	TechniqueFile = ".technique"
)

// Root finds the folder holding eval/benchmarks, starting at dir and walking up.
// A run executed from anywhere in the repository then lands in the same place.
func Root(dir string) (string, error) {
	start, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	for candidate := start; ; candidate = filepath.Dir(candidate) {
		if isDir(filepath.Join(candidate, "eval", "benchmarks")) {
			return candidate, nil
		}
		parent := filepath.Dir(candidate)
		if parent == candidate {
			return "", fmt.Errorf("no eval/benchmarks folder above %s", start)
		}
	}
}

func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// LoadBenchmark reads one benchmark and validates it. The slug comes from the
// folder name, so a benchmark cannot claim to be a task it is not filed under.
func LoadBenchmark(dir string) (*Benchmark, error) {
	body, err := os.ReadFile(filepath.Join(dir, BenchmarkFile))
	if err != nil {
		return nil, err
	}
	var b Benchmark
	if err := yaml.Unmarshal(body, &b); err != nil {
		return nil, fmt.Errorf("%s: %w", filepath.Join(dir, BenchmarkFile), err)
	}
	b.Slug = filepath.Base(dir)
	if err := b.Validate(); err != nil {
		return nil, err
	}
	return &b, nil
}

// Benchmarks lists every benchmark under root, sorted by slug.
func Benchmarks(root string) ([]*Benchmark, error) {
	entries, err := os.ReadDir(filepath.Join(root, "eval", "benchmarks"))
	if err != nil {
		return nil, err
	}
	out := make([]*Benchmark, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		b, err := LoadBenchmark(filepath.Join(root, "eval", "benchmarks", entry.Name()))
		if err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Slug < out[j].Slug })
	return out, nil
}

// BenchmarkBySlug returns one benchmark by slug.
func BenchmarkBySlug(root, slug string) (*Benchmark, error) {
	return LoadBenchmark(filepath.Join(root, "eval", "benchmarks", slug))
}

// FileRole says why a file is in a run folder, so a reader knows which image is
// the cover without opening it.
const (
	RoleSource    = "source"
	RoleCover     = "cover"
	RolePreview   = "preview"
	RoleFilmstrip = "filmstrip"
	RoleAnimation = "animation"
	RoleSheet     = "sheet"
	RoleValidate  = "validate"
	RoleAscii     = "ascii"
	RoleOther     = "other"
)

// Roles is the closed list, in the order the help text reads. A role nobody
// defined is a file a reader cannot place, so the CLI refuses one.
var Roles = []string{
	RoleSource, RoleCover, RolePreview, RoleFilmstrip,
	RoleAnimation, RoleSheet, RoleValidate, RoleAscii, RoleOther,
}

// RoleValid reports whether a role is one the record knows.
func RoleValid(role string) bool {
	for _, known := range Roles {
		if role == known {
			return true
		}
	}
	return false
}

// File is one artefact of a run.
type File struct {
	Path string `yaml:"path" json:"path"`
	Role string `yaml:"role" json:"role"`
}

// Cost is what the attempt took. Only minutes is recorded: tool calls and token
// spend belong to the harness's session log, and a field nothing fills is a
// field a reader learns to ignore.
type Cost struct {
	Minutes float64 `yaml:"minutes,omitempty" json:"minutes,omitempty"`
}

// ValidateReport is the model's own checker output for one step, kept so a
// reviewer sees the shape without the raw dump.
type ValidateReport struct {
	Step     int  `yaml:"step" json:"step"`
	Passed   bool `yaml:"passed" json:"passed"`
	Score    int  `yaml:"score" json:"score"`
	Errors   int  `yaml:"errors" json:"errors"`
	Warnings int  `yaml:"warnings" json:"warnings"`
}

// Ref names the benchmark a run followed.
type Ref struct {
	Slug     string `yaml:"slug" json:"slug"`
	Revision int    `yaml:"revision" json:"revision"`
}

// Run is one attempt: the prompts verbatim, the files it produced, and how the
// criteria came out. The technique is deliberately absent: it lives in
// TechniqueFile so a score can be given without knowing it.
type Run struct {
	ID        string           `yaml:"-" json:"id"`
	Title     string           `yaml:"title" json:"title"`
	Date      string           `yaml:"date" json:"date"`
	Agent     string           `yaml:"agent" json:"agent"`
	Model     string           `yaml:"model,omitempty" json:"model,omitempty"`
	Benchmark Ref              `yaml:"benchmark" json:"benchmark"`
	Steps     []Step           `yaml:"steps" json:"steps"`
	Files     []File           `yaml:"files" json:"files"`
	Criteria  []Result         `yaml:"criteria,omitempty" json:"criteria,omitempty"`
	Validate  []ValidateReport `yaml:"validate,omitempty" json:"validate,omitempty"`
	Cost      Cost             `yaml:"cost,omitempty" json:"cost,omitempty"`
}

// Scores are the craft axes, 0 to 4. Motion is a pointer because a still picture
// has no motion to score, and 0 would be a judgement rather than an absence.
type Scores struct {
	Read     int  `yaml:"read" json:"read"`
	Form     int  `yaml:"form" json:"form"`
	Motion   *int `yaml:"motion,omitempty" json:"motion,omitempty"`
	Cohesion int  `yaml:"cohesion" json:"cohesion"`
	Appeal   int  `yaml:"appeal" json:"appeal"`
}

// Review is the human half: the verdict, the craft scores, the answers to the
// criteria no tool can answer, and what is wrong.
type Review struct {
	// Run is the id the form belongs to. It is written into the file so a form
	// copied between folders cannot be scored against the wrong attempt, and it
	// is reloaded rather than dropped, or saving a score would erase the header.
	Run       string   `yaml:"run,omitempty" json:"run,omitempty"`
	Verdict   string   `yaml:"verdict" json:"verdict"`
	Scores    Scores   `yaml:"scores" json:"scores"`
	Defects   []string `yaml:"defects,omitempty" json:"defects,omitempty"`
	TopDefect string   `yaml:"top_defect,omitempty" json:"top_defect,omitempty"`
	Note      string   `yaml:"note,omitempty" json:"note,omitempty"`
	Reviewer  string   `yaml:"reviewer,omitempty" json:"reviewer,omitempty"`
	// Criteria holds the answers to the benchmark's human criteria. The
	// automatic ones are answered by the harness and live in the run.
	Criteria []Result `yaml:"criteria,omitempty" json:"criteria,omitempty"`
}

// ValidateScores checks the half of a review that has nothing to do with a
// verdict: the axes and the defect ticks. A form is checked with this while the
// reviewer is still deciding, so a typo in a tick is reported before the verdict
// is written rather than after.
func (r *Review) ValidateScores() error {
	for name, got := range map[string]int{
		"read": r.Scores.Read, "form": r.Scores.Form,
		"cohesion": r.Scores.Cohesion, "appeal": r.Scores.Appeal,
	} {
		if got < 0 || got > 4 {
			return fmt.Errorf("%s score %d is outside 0-4", name, got)
		}
	}
	if r.Scores.Motion != nil && (*r.Scores.Motion < 0 || *r.Scores.Motion > 4) {
		return fmt.Errorf("motion score %d is outside 0-4", *r.Scores.Motion)
	}
	known := map[string]bool{}
	for _, d := range Defects {
		known[d] = true
	}
	seen := map[string]bool{}
	for _, d := range r.Defects {
		if !known[d] {
			return fmt.Errorf("defect %q is not on the tick list; use one of: %s", d, strings.Join(Defects, ", "))
		}
		if seen[d] {
			return fmt.Errorf("defect %q is ticked twice", d)
		}
		seen[d] = true
	}
	return nil
}

// Validate is the whole review: a verdict, on top of the scores and ticks.
func (r *Review) Validate() error {
	switch r.Verdict {
	case VerdictShip, VerdictFixable, VerdictRedraw:
	default:
		return fmt.Errorf("verdict %q must be ship, fixable or redraw", r.Verdict)
	}
	return r.ValidateScores()
}

// NewRun starts a run folder for one benchmark. The steps are copied in
// verbatim: a run that paraphrases the brief measured a different task.
func NewRun(root string, b *Benchmark, date, technique, agent, model string) (*Run, string, error) {
	id, dir := freeRunID(root, date, b.Slug)
	run := &Run{
		ID:        id,
		Title:     b.Title,
		Date:      date,
		Agent:     agent,
		Model:     model,
		Benchmark: Ref{Slug: b.Slug, Revision: b.Revision},
		Steps:     append([]Step(nil), b.Steps...),
		Files:     nil,
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, "", err
	}
	if err := os.WriteFile(filepath.Join(dir, TechniqueFile), []byte(technique+"\n"), 0o644); err != nil {
		return nil, "", err
	}
	if err := SaveRun(dir, run); err != nil {
		return nil, "", err
	}
	return run, dir, nil
}

// freeRunID picks `<date>-<slug>`, or `<date>-<slug>-2` when that is taken.
//
// The technique is deliberately not part of it. The folder name is the run's
// identity and appears in the table, so a name that says "T4" tells the reviewer
// what to expect before they look at the picture, and a blind score stops being
// blind. The label lives only in TechniqueFile.
func freeRunID(root, date, slug string) (string, string) {
	base := date + "-" + slug
	for n := 1; ; n++ {
		id := base
		if n > 1 {
			id = fmt.Sprintf("%s-%d", base, n)
		}
		dir := filepath.Join(root, "eval", "runs", id)
		if _, err := os.Stat(dir); errors.Is(err, os.ErrNotExist) {
			return id, dir
		}
	}
}

// SaveRun writes the run record, keeping key order readable for a human.
func SaveRun(dir string, run *Run) error {
	body, err := yaml.Marshal(run)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, RunFile), body, 0o644)
}

// LoadRun reads a run from its folder.
func LoadRun(dir string) (*Run, error) {
	body, err := os.ReadFile(filepath.Join(dir, RunFile))
	if err != nil {
		return nil, err
	}
	var run Run
	if err := yaml.Unmarshal(body, &run); err != nil {
		return nil, fmt.Errorf("%s: %w", filepath.Join(dir, RunFile), err)
	}
	run.ID = filepath.Base(dir)
	return &run, nil
}

// Technique reads the hidden label. Missing is not an error: a run made by hand
// may not have one, and the table just shows a blank.
func Technique(dir string) string {
	body, err := os.ReadFile(filepath.Join(dir, TechniqueFile))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(body))
}

// LoadReview reads the review, or returns an empty one when it has not been
// written yet.
func LoadReview(dir string) (*Review, error) {
	body, err := os.ReadFile(filepath.Join(dir, ReviewFile))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return &Review{}, nil
		}
		return nil, err
	}
	var review Review
	if err := yaml.Unmarshal(body, &review); err != nil {
		return nil, fmt.Errorf("%s: %w", filepath.Join(dir, ReviewFile), err)
	}
	// A form that names a different run was copied, not written here. Scoring it
	// would attach one attempt's judgement to another attempt's pictures.
	id := filepath.Base(dir)
	if review.Run != "" && review.Run != id {
		return nil, fmt.Errorf("%s names run %q but lives in %q", ReviewFile, review.Run, id)
	}
	review.Run = id
	return &review, nil
}

// SaveReview writes the review, refusing one that would not mean anything.
func SaveReview(dir string, review *Review) error {
	if err := review.Validate(); err != nil {
		return err
	}
	body, err := yaml.Marshal(review)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, ReviewFile), body, 0o644)
}

// VerifyBenchmark checks that a run's steps and revision still match the
// benchmark it claims. A run from an older revision is kept but never compared,
// so this returning an error is information, not corruption.
func VerifyBenchmark(run *Run, b *Benchmark) error {
	if run.Benchmark.Slug != b.Slug {
		return fmt.Errorf("this run followed %s, not %s", run.Benchmark.Slug, b.Slug)
	}
	if run.Benchmark.Revision != b.Revision {
		return fmt.Errorf("this run followed revision %d, the benchmark is now revision %d", run.Benchmark.Revision, b.Revision)
	}
	if len(run.Steps) != len(b.Steps) {
		return fmt.Errorf("the run has %d step(s), the benchmark has %d", len(run.Steps), len(b.Steps))
	}
	for i := range b.Steps {
		if run.Steps[i].Text != b.Steps[i].Text {
			return fmt.Errorf("step %d text differs from the benchmark, so this run measured a different task", i+1)
		}
	}
	return nil
}

// VerifyReview checks that a review answers this benchmark's human criteria and
// nothing else. An answer to a criterion that no longer exists is a form filled
// in against a different revision.
func VerifyReview(b *Benchmark, review *Review) error {
	human := map[string]bool{}
	for _, c := range b.Criteria {
		if c.Human {
			human[c.ID] = true
		}
	}
	seen := map[string]bool{}
	for _, r := range review.Criteria {
		if !human[r.Criterion] {
			return fmt.Errorf("%q is not a human criterion of %s", r.Criterion, b.Slug)
		}
		if seen[r.Criterion] {
			return fmt.Errorf("%q is answered twice", r.Criterion)
		}
		seen[r.Criterion] = true
	}
	// Motion is judged only where there is motion to judge. An unscored form is
	// allowed to leave it out; once a verdict is claimed, the axis has to match
	// the benchmark, or "I looked at the loop" and "I looked at frame one" score
	// the same.
	if review.Verdict != "" {
		if animated(b) && review.Scores.Motion == nil {
			return fmt.Errorf("%s animates, so score its motion", b.Slug)
		}
		if !animated(b) && review.Scores.Motion != nil {
			return fmt.Errorf("%s draws stills, so there is no motion to score", b.Slug)
		}
	}
	return nil
}

// Row is one line of the results table.
type Row struct {
	Run       string  `json:"run"`
	Benchmark string  `json:"benchmark"`
	Technique string  `json:"technique,omitempty"`
	Verdict   string  `json:"verdict,omitempty"`
	Craft     int     `json:"craft"`
	Passed    int     `json:"passed"`
	Total     int     `json:"total"`
	Human     int     `json:"human"`
	Minutes   float64 `json:"minutes,omitempty"`
}

// Table reads every run under root, joined with its hidden technique, so the
// spread of verdicts is visible instead of a single average that hides it.
func Table(root string) ([]Row, error) {
	runsDir := filepath.Join(root, "eval", "runs")
	entries, err := os.ReadDir(runsDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	rows := make([]Row, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		dir := filepath.Join(runsDir, entry.Name())
		run, err := LoadRun(dir)
		if err != nil {
			return nil, err
		}
		review, err := LoadReview(dir)
		if err != nil {
			return nil, err
		}
		row := Row{
			Run:       run.ID,
			Benchmark: run.Benchmark.Slug,
			Technique: Technique(dir),
			Verdict:   review.Verdict,
			Craft:     VerdictScore(review.Verdict),
			Minutes:   run.Cost.Minutes,
		}
		for _, r := range run.Criteria {
			// Human criteria sit in the run as unanswered placeholders; their
			// answers live in the review, so counting them here would count them
			// twice the moment somebody filled the form in.
			if r.Human {
				continue
			}
			row.Total++
			if r.Passed {
				row.Passed++
			}
		}
		// Human criteria are only counted once a reviewer has answered them, so
		// an unfilled form reads as less complete rather than as passing.
		for _, r := range review.Criteria {
			row.Total++
			row.Human++
			if r.Passed {
				row.Passed++
			}
		}
		rows = append(rows, row)
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].Run < rows[j].Run })
	return rows, nil
}
