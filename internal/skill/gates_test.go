package skill

import (
	"strings"
	"testing"
)

// The gates are the part of the entry document that must arrive on every route.
// If the section is renamed or an item loses its chapter link, the guide quietly
// stops carrying it, so the shape is pinned here.
func TestGatesParseFromTheEntryDocument(t *testing.T) {
	lib, err := Default()
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	gates := lib.Gates()
	if len(gates) != 6 {
		t.Fatalf("%d always-on gates, want 6", len(gates))
	}
	for i, gate := range gates {
		if strings.TrimSpace(gate.Check) == "" {
			t.Errorf("gate %d has no check text", i+1)
			continue
		}
		if !strings.HasSuffix(gate.Check, ".") {
			t.Errorf("gate %d is not a sentence: %q", i+1, gate.Check)
		}
		if strings.Contains(gate.Check, "](") || strings.Contains(gate.Check, "**") {
			t.Errorf("gate %d still carries markdown: %q", i+1, gate.Check)
		}
		if len(gate.Rules) == 0 {
			t.Errorf("gate %d names no chapter: %q", i+1, gate.Check)
		}
		for _, id := range gate.Rules {
			if _, ok := lib.Chapter(id); !ok {
				t.Errorf("gate %d names chapter %q, which is not in the library", i+1, id)
			}
		}
	}

	if !strings.Contains(gates[0].Check, "silhouette") {
		t.Errorf("the first gate is not the silhouette gate: %q", gates[0].Check)
	}
	if got := gates[0].Rules; len(got) != 2 {
		t.Errorf("the silhouette gate names %v, want two chapters", got)
	}
}

// A chapter's Review section is its acceptance test. Every chapter that ships
// one has to parse into at least one check, or the guide would claim a chapter
// has nothing to act on when it does.
func TestEveryReviewSectionYieldsChecks(t *testing.T) {
	lib, err := Default()
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	withReview := 0
	for _, id := range lib.ChapterIDs() {
		chapter, _ := lib.Chapter(id)
		if !strings.Contains(chapter.Body, "## Review") {
			continue
		}
		withReview++

		checks := chapter.Checks()
		if len(checks) == 0 {
			t.Errorf("%s has a Review section but no parsed checks", id)
			continue
		}
		for _, check := range checks {
			if strings.TrimSpace(check) == "" {
				t.Errorf("%s has an empty check", id)
			}
			if strings.HasPrefix(check, "-") || strings.HasPrefix(check, "*") {
				t.Errorf("%s kept a bullet marker: %q", id, check)
			}
			if strings.Contains(check, "\n") {
				t.Errorf("%s has a check that was never joined: %q", id, check)
			}
			if strings.Contains(check, "](") {
				t.Errorf("%s has a check that was never flattened: %q", id, check)
			}
		}
	}

	if withReview < 50 {
		t.Errorf("only %d chapters were found with a Review section; the library ships more", withReview)
	}
}

// The guide prints this chapter first for a walk task, so its checks have to be
// the walk criteria rather than an empty list.
func TestKnownChapterChecksAreTheReviewLines(t *testing.T) {
	lib, err := Default()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	chapter, ok := lib.Chapter("42-walk-and-run")
	if !ok {
		t.Fatal("42-walk-and-run is not in the library")
	}

	checks := chapter.Checks()
	if len(checks) < 5 {
		t.Fatalf("%d checks, want the whole Review list", len(checks))
	}
	if !strings.Contains(checks[0], "walker") {
		t.Errorf("first check = %q, want the flat-frame question", checks[0])
	}
	if !strings.Contains(strings.Join(checks, " "), "pass frame") {
		t.Error("the pass-frame check is missing")
	}
}
