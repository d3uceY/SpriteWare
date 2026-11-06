package eval

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ReviewForm renders the sheet a reviewer fills in. It is written by hand
// rather than marshalled because the comments are the instructions: a reviewer
// who has to go and read the rubric to remember what `form` means will score
// the wrong thing.
//
// The technique is not in it, and must not be: the run folder's hidden
// TechniqueFile is read only by the table.
func ReviewForm(run *Run, b *Benchmark) []byte {
	var out strings.Builder
	out.WriteString("# The review of one run. Fill this in after looking at the pictures,\n")
	out.WriteString("# not while reading how the run was made.\n")
	out.WriteString("run: " + run.ID + "\n")
	out.WriteString("reviewer: \"\"\n\n")

	out.WriteString("# ship    you would use it as it is; nothing you would bother fixing\n")
	out.WriteString("# fixable the idea works, one to three specific things are wrong\n")
	out.WriteString("# redraw  you cannot tell what it is, or the structure is wrong\n")
	out.WriteString("verdict: \"\"\n\n")

	out.WriteString("# 0-4 each. When a run sits between two numbers take the lower one.\n")
	out.WriteString("scores:\n")
	out.WriteString("  read: 0     # can you tell what it is at actual size?\n")
	out.WriteString("  form: 0     # one light source, do materials look different?\n")
	if animated(b) {
		out.WriteString("  motion: 0   # timing, spacing, follow-through\n")
	} else {
		out.WriteString("  # motion is left out: this benchmark draws stills\n")
	}
	out.WriteString("  cohesion: 0 # palette, consistency, clean pixels\n")
	out.WriteString("  appeal: 0   # would you use it?\n\n")

	out.WriteString("# Tick what you see. Every tick maps to an automatic check where one\n")
	out.WriteString("# exists, so a tick the checker did not report is a bug report.\n")
	out.WriteString("# " + strings.Join(Defects, ", ") + "\n")
	out.WriteString("defects: []\n\n")

	out.WriteString("# One line: what most reduces the score?\n")
	out.WriteString("top_defect: \"\"\n")
	out.WriteString("note: \"\"\n\n")

	out.WriteString("# The criteria no tool can answer. Set pass: true or false and say why.\n")
	out.WriteString("criteria:\n")
	for _, c := range b.Criteria {
		if !c.Human {
			continue
		}
		fmt.Fprintf(&out, "  - criterion: %s\n", c.ID)
		fmt.Fprintf(&out, "    # %s\n", oneLine(c.Text))
		out.WriteString("    pass: false\n")
		out.WriteString("    note: \"\"\n")
	}
	return []byte(out.String())
}

// oneLine folds a multi-line criterion so it fits in a YAML comment.
func oneLine(text string) string {
	return strings.Join(strings.Fields(text), " ")
}

// animated reports whether the benchmark asks for frames at all, which decides
// whether there is a motion score to fill in.
func animated(b *Benchmark) bool {
	for _, c := range b.Criteria {
		if c.Check == nil {
			continue
		}
		switch c.Check.Kind {
		case CheckFrames, CheckTag, CheckTiming:
			return true
		}
	}
	return false
}

// WriteReviewForm writes the form unless one is already there. Overwriting a
// filled-in review would silently throw away the only human judgement in the
// repository.
func WriteReviewForm(dir string, run *Run, b *Benchmark) (string, error) {
	path := filepath.Join(dir, ReviewFile)
	if _, err := os.Stat(path); err == nil {
		return path, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	return path, os.WriteFile(path, ReviewForm(run, b), 0o644)
}
