// Command spriteware is the command-line half of the toolkit: the eval harness
// that runs practice tasks and records what came of them.
//
// It is deliberately separate from the MCP server. The server gives a model
// hands and eyes; this measures what the hands produced, and it writes plain
// files a person can read, diff and review.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/d3uceY/spriteware/internal/analyze"
	"github.com/d3uceY/spriteware/internal/config"
	"github.com/d3uceY/spriteware/internal/doc"
	"github.com/d3uceY/spriteware/internal/eval"
	"github.com/d3uceY/spriteware/internal/link"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "eval":
		err = evalCmd(os.Args[2:])
	case "help", "-h", "--help":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n", os.Args[1])
		usage()
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "error: "+err.Error())
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprint(os.Stdout, `spriteware - the eval harness

  spriteware eval list                     the benchmarks and how much each asks
  spriteware eval new  --benchmark <slug>  start a run folder for one benchmark
  spriteware eval add  --role <role> <run> <file>   list one artefact in the run
  spriteware eval brief <run>              print the step texts to hand over
  spriteware eval check <run>              score the automatic criteria and write the review form
  spriteware eval score <run>              read the filled-in review back and print the result
  spriteware eval table                    every run, with the spread of verdicts

Run every command from anywhere inside the repository; eval/ is found by
walking up from the working directory.

Scoring is: open the run's review.yaml, fill in the verdict, the five scores,
the defect ticks and the human criteria, save it, then run eval score <run>.
It reads the form back, refuses a score that is not on the scale, and prints
what the harness and the person each decided.
`)
}

func evalCmd(args []string) error {
	if len(args) == 0 {
		usage()
		return errors.New("eval needs one of: list, new, brief, check, score, table")
	}
	switch args[0] {
	case "list":
		return evalList(args[1:])
	case "new":
		return evalNew(args[1:])
	case "add":
		return evalAdd(args[1:])
	case "brief":
		return evalBrief(args[1:])
	case "check":
		return evalCheck(args[1:])
	case "score":
		return evalScore(args[1:])
	case "table":
		return evalTable(args[1:])
	}
	return fmt.Errorf("unknown eval command %q", args[0])
}

func repoRoot(flagValue string) (string, error) {
	if flagValue != "" {
		return filepath.Abs(flagValue)
	}
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	return eval.Root(wd)
}

func evalList(args []string) error {
	fs := flag.NewFlagSet("eval list", flag.ContinueOnError)
	root := fs.String("root", "", "repository root (default: found by walking up)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	dir, err := repoRoot(*root)
	if err != nil {
		return err
	}
	all, err := eval.Benchmarks(dir)
	if err != nil {
		return err
	}
	if len(all) == 0 {
		return fmt.Errorf("no benchmarks under %s", filepath.Join(dir, "eval", "benchmarks"))
	}
	for _, b := range all {
		fmt.Printf("%-14s revision %d  %d step(s)  %d automatic / %d human criterion(s)\n",
			b.Slug, b.Revision, len(b.Steps), b.Automatic(), b.Human())
		fmt.Printf("               %s\n", b.Title)
		fmt.Printf("               %s\n", oneLine(b.Summary))
		if len(b.Tags) > 0 {
			fmt.Printf("               tags: %s\n", strings.Join(b.Tags, ", "))
		}
		fmt.Println()
	}
	return nil
}

func evalNew(args []string) error {
	fs := flag.NewFlagSet("eval new", flag.ContinueOnError)
	root := fs.String("root", "", "repository root (default: found by walking up)")
	slug := fs.String("benchmark", "", "benchmark slug, as listed by eval list")
	technique := fs.String("technique", "", "the technique label, kept hidden from the reviewer")
	agent := fs.String("agent", "unknown", "the harness that drove the run")
	model := fs.String("model", "", "the model that drew it")
	date := fs.String("date", time.Now().Format("2006-01-02"), "run date, YYYY-MM-DD")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *slug == "" {
		return errors.New("eval new needs --benchmark, as listed by eval list")
	}
	dir, err := repoRoot(*root)
	if err != nil {
		return err
	}
	b, err := eval.BenchmarkBySlug(dir, *slug)
	if err != nil {
		return err
	}
	run, runDir, err := eval.NewRun(dir, b, *date, *technique, *agent, *model)
	if err != nil {
		return err
	}
	fmt.Printf("run %s\n%s\n\n", run.ID, runDir)
	fmt.Printf("setup: canvas %dx%d, palette %s\n", b.Setup.Canvas.Width, b.Setup.Canvas.Height, orNone(b.Setup.Palette))
	for _, rule := range b.Setup.Rules {
		fmt.Printf("  - %s\n", rule)
	}
	fmt.Printf("\nHand over step 1 verbatim, then check the run with:\n  spriteware eval check %s\n", run.ID)
	return nil
}

func evalAdd(args []string) error {
	fs := flag.NewFlagSet("eval add", flag.ContinueOnError)
	root := fs.String("root", "", "repository root (default: found by walking up)")
	role := fs.String("role", eval.RoleOther, "source, cover, preview, filmstrip, animation, sheet, validate, ascii or other")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 2 {
		return errors.New("eval add needs a run id and a file; put --role before them, as Go's flag parser stops at the first plain argument")
	}
	if !eval.RoleValid(*role) {
		return fmt.Errorf("role %q is not one of: %s", *role, strings.Join(eval.Roles, ", "))
	}
	dir, err := repoRoot(*root)
	if err != nil {
		return err
	}
	runDir, err := runDir(dir, fs.Arg(0))
	if err != nil {
		return err
	}
	run, err := eval.LoadRun(runDir)
	if err != nil {
		return err
	}
	name := filepath.Base(fs.Arg(1))
	if _, err := os.Stat(filepath.Join(runDir, name)); err != nil {
		return fmt.Errorf("%s is not in the run folder; keep a run's files beside its %s", name, eval.RunFile)
	}
	replaced := false
	for i := range run.Files {
		if run.Files[i].Path == name {
			run.Files[i].Role = *role
			replaced = true
		}
	}
	if !replaced {
		run.Files = append(run.Files, eval.File{Path: name, Role: *role})
	}
	if err := eval.SaveRun(runDir, run); err != nil {
		return err
	}
	fmt.Printf("%s: %s as %s\n", run.ID, name, *role)
	return nil
}

func evalBrief(args []string) error {
	fs := flag.NewFlagSet("eval brief", flag.ContinueOnError)
	root := fs.String("root", "", "repository root (default: found by walking up)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return errors.New("eval brief needs one run id")
	}
	dir, err := repoRoot(*root)
	if err != nil {
		return err
	}
	runDir, err := runDir(dir, fs.Arg(0))
	if err != nil {
		return err
	}
	run, err := eval.LoadRun(runDir)
	if err != nil {
		return err
	}
	for i, step := range run.Steps {
		fmt.Printf("--- step %d: %s ---\n%s\n\n", i+1, step.Title, step.Text)
	}
	return nil
}

func evalCheck(args []string) error {
	fs := flag.NewFlagSet("eval check", flag.ContinueOnError)
	root := fs.String("root", "", "repository root (default: found by walking up)")
	exe := fs.String("exe", "", "Aseprite executable (default: from the config, then discovered)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return errors.New("eval check needs one run id")
	}
	dir, err := repoRoot(*root)
	if err != nil {
		return err
	}
	runDir, err := runDir(dir, fs.Arg(0))
	if err != nil {
		return err
	}
	run, err := eval.LoadRun(runDir)
	if err != nil {
		return err
	}
	b, err := eval.BenchmarkBySlug(dir, run.Benchmark.Slug)
	if err != nil {
		return err
	}
	if err := eval.VerifyBenchmark(run, b); err != nil {
		return err
	}
	source, ok := sourceFile(run)
	if !ok {
		return fmt.Errorf("%s is not in run.yaml with role %q; save the .aseprite and list it first", eval.RunFile, eval.RoleSource)
	}
	dump, err := dumpDocument(filepath.Join(runDir, source), *exe, runDir)
	if err != nil {
		return err
	}
	run.Criteria = eval.Evaluate(b, dump)
	report := analyze.Run(dump, analyze.Options{})
	run.Validate = []eval.ValidateReport{{
		Step:     len(run.Steps),
		Passed:   report.Passed,
		Score:    report.Score,
		Errors:   countSeverity(report, analyze.Error),
		Warnings: countSeverity(report, analyze.Warning),
	}}
	if err := eval.SaveRun(runDir, run); err != nil {
		return err
	}
	form, err := eval.WriteReviewForm(runDir, run, b)
	if err != nil {
		return err
	}

	passed := 0
	for _, r := range run.Criteria {
		mark := "pass"
		switch {
		case r.Human:
			mark = "human"
		case !r.Passed:
			mark = "FAIL"
		}
		if r.Passed {
			passed++
		}
		fmt.Printf("%-5s %-22s %s\n", mark, r.Criterion, r.Note)
	}
	fmt.Printf("\n%d of %d automatic criterion(s) pass; validate: %s (score %d)\n",
		passed, b.Automatic(), boolWord(report.Passed, "clean", "findings"), report.Score)
	fmt.Printf("fill in %s\n", form)
	return nil
}

func evalScore(args []string) error {
	fs := flag.NewFlagSet("eval score", flag.ContinueOnError)
	root := fs.String("root", "", "repository root (default: found by walking up)")
	verdict := fs.String("verdict", "", "ship, fixable or redraw")
	read := fs.Int("read", 0, "read score, 0-4")
	form := fs.Int("form", 0, "form score, 0-4")
	motion := fs.Int("motion", -1, "motion score, 0-4; omit for a still")
	cohesion := fs.Int("cohesion", 0, "cohesion score, 0-4")
	appeal := fs.Int("appeal", 0, "appeal score, 0-4")
	reviewer := fs.String("reviewer", "", "who scored it")
	top := fs.String("top", "", "one line: what most reduces the score")
	note := fs.String("note", "", "a sentence on what held up and what did not")
	minutes := fs.Float64("minutes", 0, "wall time the attempt took, in minutes")
	var defects stringList
	fs.Var(&defects, "defect", "a defect from the tick list; repeat for several")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return errors.New("eval score needs one run id")
	}
	dir, err := repoRoot(*root)
	if err != nil {
		return err
	}
	runDir, err := runDir(dir, fs.Arg(0))
	if err != nil {
		return err
	}
	run, err := eval.LoadRun(runDir)
	if err != nil {
		return err
	}
	b, err := eval.BenchmarkBySlug(dir, run.Benchmark.Slug)
	if err != nil {
		return err
	}
	review, err := eval.LoadReview(runDir)
	if err != nil {
		return err
	}
	// Every flag is optional: the usual way to score is to edit review.yaml by
	// hand and run this with no flags, so it reads the form back and says what
	// is still blank. The flags are for scripting a score without an editor.
	set := map[string]bool{}
	fs.Visit(func(f *flag.Flag) { set[f.Name] = true })
	if set["verdict"] {
		review.Verdict = strings.TrimSpace(*verdict)
	}
	if set["read"] {
		review.Scores.Read = *read
	}
	if set["form"] {
		review.Scores.Form = *form
	}
	if set["motion"] {
		review.Scores.Motion = motion
	}
	if set["cohesion"] {
		review.Scores.Cohesion = *cohesion
	}
	if set["appeal"] {
		review.Scores.Appeal = *appeal
	}
	if set["defect"] {
		review.Defects = defects
	}
	if set["top"] {
		review.TopDefect = *top
	}
	if set["note"] {
		review.Note = *note
	}
	if set["reviewer"] {
		review.Reviewer = *reviewer
	}
	if set["minutes"] {
		run.Cost.Minutes = *minutes
		if err := eval.SaveRun(runDir, run); err != nil {
			return err
		}
	}
	if err := eval.VerifyReview(b, review); err != nil {
		return err
	}
	// The scores and ticks are checked before the verdict, so a half-filled form
	// reports its typos now rather than after the reviewer has walked away.
	if err := review.ValidateScores(); err != nil {
		return err
	}
	// An unscored form is the normal state right after eval check. Showing the
	// results is still useful then, so this is a report, not an error.
	if review.Verdict == "" {
		printRun(run, b, review, runDir)
		fmt.Printf("\nnot scored yet: fill in %s, then run this again\n", filepath.Join(runDir, eval.ReviewFile))
		return nil
	}
	if err := eval.SaveReview(runDir, review); err != nil {
		return err
	}
	printRun(run, b, review, runDir)
	return nil
}

// printRun is the whole answer for one run: what the harness decided, what a
// person decided, and what is still blank.
func printRun(run *eval.Run, b *eval.Benchmark, review *eval.Review, dir string) {
	fmt.Printf("%s  (%s revision %d)\n", run.ID, b.Slug, b.Revision)
	who := run.Agent
	if run.Model != "" {
		who += ", " + run.Model
	}
	fmt.Printf("made by %s", who)
	if review.Reviewer != "" {
		fmt.Printf(", judged by %s", review.Reviewer)
	}
	if run.Cost.Minutes > 0 {
		fmt.Printf(", %.1f min", run.Cost.Minutes)
	}
	fmt.Println()
	fmt.Println()

	fmt.Println("automatic")
	passed := 0
	for _, r := range run.Criteria {
		if r.Human {
			continue
		}
		mark := "fail"
		if r.Passed {
			mark = "pass"
			passed++
		}
		fmt.Printf("  %-5s %-22s %s\n", mark, r.Criterion, r.Note)
	}
	if len(run.Criteria) == 0 {
		fmt.Println("  nothing scored yet: run `spriteware eval check` first")
	}
	if n := len(run.Validate); n > 0 {
		report := run.Validate[n-1]
		fmt.Printf("\nvalidate  score %d, %d error(s), %d warning(s), passed %v\n",
			report.Score, report.Errors, report.Warnings, report.Passed)
	}

	answered := map[string]eval.Result{}
	for _, r := range review.Criteria {
		answered[r.Criterion] = r
	}
	fmt.Println("\nhuman")
	pending := 0
	for _, c := range b.Criteria {
		if !c.Human {
			continue
		}
		r, ok := answered[c.ID]
		switch {
		case ok && r.Passed:
			fmt.Printf("  pass  %-22s %s\n", c.ID, oneLine(r.Note))
		case ok && strings.TrimSpace(r.Note) != "":
			fmt.Printf("  fail  %-22s %s\n", c.ID, oneLine(r.Note))
		default:
			// The form ships with pass: false and an empty note, so that pair
			// means nobody has answered yet, not that the run failed.
			pending++
			fmt.Printf("  blank %-22s %s\n", c.ID, oneLine(c.Text))
		}
	}

	fmt.Printf("\nautomatic %d/%d   verdict %s   craft %d\n",
		passed, b.Automatic(), orNone(review.Verdict), eval.VerdictScore(review.Verdict))
	if review.TopDefect != "" {
		fmt.Printf("biggest problem: %s\n", review.TopDefect)
	}
	if pending > 0 {
		fmt.Printf("\n%d human answer(s) still blank in %s\n", pending, filepath.Join(dir, eval.ReviewFile))
	}
}

func evalTable(args []string) error {
	fs := flag.NewFlagSet("eval table", flag.ContinueOnError)
	root := fs.String("root", "", "repository root (default: found by walking up)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	dir, err := repoRoot(*root)
	if err != nil {
		return err
	}
	rows, err := eval.Table(dir)
	if err != nil {
		return err
	}
	if len(rows) == 0 {
		fmt.Println("no runs yet: spriteware eval new --benchmark <slug>")
		return nil
	}
	fmt.Printf("%-34s %-14s %-7s %-9s %-8s %-5s %s\n", "run", "benchmark", "tech", "verdict", "criteria", "craft", "minutes")
	for _, row := range rows {
		fmt.Printf("%-34s %-14s %-7s %-9s %-8s %-5d %s\n",
			row.Run, row.Benchmark, orNone(row.Technique), orNone(row.Verdict),
			fmt.Sprintf("%d/%d", row.Passed, row.Total), row.Craft, minutesOrDash(row.Minutes))
	}
	return nil
}

// minutesOrDash shows a cost nobody recorded as a dash rather than as zero,
// which would claim the attempt was instant.
func minutesOrDash(minutes float64) string {
	if minutes <= 0 {
		return "-"
	}
	return fmt.Sprintf("%.1f", minutes)
}

// runDir accepts either a run id or a path to the folder itself.
func runDir(root, arg string) (string, error) {
	if info, err := os.Stat(arg); err == nil && info.IsDir() {
		return filepath.Abs(arg)
	}
	candidate := filepath.Join(root, "eval", "runs", arg)
	if info, err := os.Stat(candidate); err == nil && info.IsDir() {
		return candidate, nil
	}
	return "", fmt.Errorf("no run folder named %q under eval/runs", arg)
}

func sourceFile(run *eval.Run) (string, bool) {
	for _, file := range run.Files {
		if file.Role == eval.RoleSource {
			return file.Path, true
		}
	}
	return "", false
}

// dumpDocument reads the .aseprite through a background Aseprite, which is the
// same path the tools use: the record is scored from the file, not from a
// screenshot of it.
func dumpDocument(path, exe, dir string) (*doc.Dump, error) {
	if _, err := os.Stat(path); err != nil {
		return nil, err
	}
	if exe == "" {
		store, err := config.Load()
		if err != nil {
			return nil, err
		}
		exe, err = config.FindAseprite(store.Get().AsepritePath)
		if err != nil {
			return nil, err
		}
	}
	h, err := link.NewHeadless(link.HeadlessOptions{Exe: exe, Cwd: dir})
	if err != nil {
		return nil, err
	}
	defer h.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	if err := h.Call(ctx, "doc.open", map[string]any{"path": path}, nil); err != nil {
		return nil, err
	}
	return h.Dump(ctx, nil)
}

func countSeverity(report *analyze.Report, severity analyze.Severity) int {
	n := 0
	for _, f := range report.Findings {
		if f.Severity == severity {
			n++
		}
	}
	return n
}

func oneLine(text string) string { return strings.Join(strings.Fields(text), " ") }

func orNone(value string) string {
	if strings.TrimSpace(value) == "" {
		return "-"
	}
	return value
}

func boolWord(ok bool, whenTrue, whenFalse string) string {
	if ok {
		return whenTrue
	}
	return whenFalse
}

// stringList collects a repeatable flag.
type stringList []string

func (s *stringList) String() string     { return strings.Join(*s, ",") }
func (s *stringList) Set(v string) error { *s = append(*s, v); return nil }
