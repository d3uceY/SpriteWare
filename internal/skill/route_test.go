package skill

import (
	"strings"
	"testing"
)

// The library is the whole skill tree in memory: an entry document, ten
// workflows, the subject chapters and the lookup notes. A count that drifts is
// a file that silently stopped being routed to.
func TestLoadReadsTheWholeLibrary(t *testing.T) {
	lib, err := Default()
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	if lib.Entry == nil {
		t.Fatal("no entry document")
	}
	if lib.Entry.Name != Name {
		t.Errorf("entry name = %q, want %q", lib.Entry.Name, Name)
	}
	if got := len(lib.WorkflowNames()); got != 10 {
		t.Errorf("%d workflows, want 10: %v", got, lib.WorkflowNames())
	}
	if got := len(lib.ChapterIDs()); got != 62 {
		t.Errorf("%d subject chapters, want 62", got)
	}
	if got := len(lib.ReferenceNames()); got != 6 {
		t.Errorf("%d references, want 6: %v", got, lib.ReferenceNames())
	}
}

// The routing contract is data, so a bad edit ships silently unless a test
// reads it: a workflow must name chapters that exist, and its triggers must be
// usable words.
func TestTheRoutingContractHolds(t *testing.T) {
	lib, err := Default()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if err := lib.Validate(); err != nil {
		t.Fatalf("validate: %v", err)
	}
}

func TestRoutePicksExpectedWorkflow(t *testing.T) {
	lib, err := Default()
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	cases := []struct {
		task string
		want string
	}{
		{"animate a walk cycle for a knight", "animate"},
		{"draw a horse sprite", "draw"},
		{"add shading to the cloak", "shade"},
		{"pick a gameboy palette", "palette"},
		{"rig this character into layers", "rig"},
		{"build a dungeon tileset with walls", "tileset"},
		{"fix the arm, it is one pixel too long", "fix"},
		{"review this sprite before I ship it", "review"},
		{"export a spritesheet for Godot", "export"},
		{"starting a new sprite from nothing", "new"},
	}

	for _, tc := range cases {
		routes := lib.Route(tc.task, 3)
		if len(routes) == 0 {
			t.Errorf("Route(%q) returned nothing", tc.task)
			continue
		}
		names := make([]string, 0, len(routes))
		found := false
		for _, route := range routes {
			names = append(names, route.Workflow.Name)
			if route.Workflow.Name == tc.want {
				found = true
			}
		}
		if !found {
			t.Errorf("Route(%q) = %v, want %q in the top 3", tc.task, names, tc.want)
		}
	}
}

// Triggers exist for the words a description never uses. Both tasks below carry
// no workflow vocabulary at all; only the trigger list can route them.
func TestTriggersRouteWordsTheDescriptionsDoNot(t *testing.T) {
	lib, err := Default()
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	cases := []struct {
		task string
		want string
	}{
		{"pack this for godot", "export"},
		{"my wangset has seams", "tileset"},
		{"it needs to work on a gameboy", "palette"},
	}
	for _, tc := range cases {
		routes := lib.Route(tc.task, 3)
		if len(routes) == 0 || routes[0].Workflow.Name != tc.want {
			got := "<none>"
			if len(routes) > 0 {
				got = routes[0].Workflow.Name
			}
			t.Errorf("Route(%q) = %q, want %q", tc.task, got, tc.want)
		}
	}
}

func TestRouteIsDeterministic(t *testing.T) {
	lib, err := Default()
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	first := lib.Route("minimal idle animation for a slime", 5)
	for i := 0; i < 5; i++ {
		next := lib.Route("minimal idle animation for a slime", 5)
		for j := range first {
			if first[j].Workflow.Name != next[j].Workflow.Name {
				t.Fatalf("route order changed between calls at %d", j)
			}
		}
	}
}

func TestSearchFindsChaptersBySubject(t *testing.T) {
	lib, err := Default()
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	cases := []struct {
		query string
		want  string
	}{
		{"hands and feet", "34-hands-and-feet"},
		{"dither", "13-dithering-and-texture"},
		{"isometric", "71-isometric"},
	}
	for _, tc := range cases {
		hits := lib.Search(tc.query, 3)
		if len(hits) == 0 {
			t.Errorf("Search(%q) returned nothing", tc.query)
			continue
		}
		if hits[0].Chapter.ID != tc.want {
			t.Errorf("Search(%q) top hit = %q, want %q", tc.query, hits[0].Chapter.ID, tc.want)
		}
	}
}

func TestGuideRoutesTaskAndChapters(t *testing.T) {
	lib, err := Default()
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	g, ok := lib.Guide("animate a walk cycle for a knight", 8)
	if !ok {
		t.Fatal("Guide() did not route an animation task")
	}
	if g.Workflow.Name != "animate" {
		t.Errorf("workflow = %q, want animate", g.Workflow.Name)
	}
	if len(g.Rules) < 3 {
		t.Errorf("%d chapters, want at least the workflow's own", len(g.Rules))
	}
	if g.RuleSources[0] != "workflow" {
		t.Errorf("first chapter source = %q, want workflow", g.RuleSources[0])
	}
	if !g.Confident() {
		t.Error("a task naming the workflow should route with confidence")
	}
	if len(g.References) == 0 {
		t.Error("animate links references, so the guide should carry them")
	}
	for _, chapter := range g.Rules {
		if !strings.HasPrefix(chapter.URI(), "rules://") {
			t.Errorf("%s has no rules:// URI", chapter.ID)
		}
	}
}

// The workflow's chapters are all worth reading, but the one the task is about
// has to come first: an inline budget cuts the tail, so a walk task must lead
// with the walk chapter.
func TestGuideOrdersChaptersByTaskRelevance(t *testing.T) {
	lib, err := Default()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	g, ok := lib.Guide("animate a walk cycle for a knight", 6)
	if !ok {
		t.Fatal("no route")
	}
	if g.Rules[0].ID != "42-walk-and-run" {
		t.Errorf("first chapter = %q, want 42-walk-and-run", g.Rules[0].ID)
	}
}

// A single rare word routes, but it is not enough to volunteer the route: the
// caller sees matchedTokens and decides.
func TestGuideFlagsAWeakRoute(t *testing.T) {
	lib, err := Default()
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	g, ok := lib.Guide("fibonacci sequence", 5)
	if !ok {
		t.Fatal("expected a flagged match, not a miss")
	}
	if g.Confident() {
		t.Errorf("one overlapping word should not be confident: %v", g.Matched)
	}
}

func TestGuideRejectsAWordlessTask(t *testing.T) {
	lib, err := Default()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if g, ok := lib.Guide("xyzzy plugh quux", 5); ok {
		t.Fatalf("an unrelated task routed to %s", g.Workflow.Name)
	}
	if _, ok := lib.Guide("  ", 5); ok {
		t.Fatal("an empty task routed")
	}
}

func TestGuideRespectsTheRuleLimit(t *testing.T) {
	lib, err := Default()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	g, ok := lib.Guide("build a dungeon tileset with walls", 3)
	if !ok {
		t.Fatal("no route")
	}
	if len(g.Rules) > 3 {
		t.Errorf("%d chapters returned, limit was 3", len(g.Rules))
	}
}

func TestInlineAlwaysIncludesTheWorkflow(t *testing.T) {
	lib, err := Default()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	g, ok := lib.Guide("animate a walk cycle for a knight", 8)
	if !ok {
		t.Fatal("no route")
	}

	markdown, included, omitted := g.Inline(1)
	if markdown == "" {
		t.Fatal("a budget smaller than the workflow returned nothing")
	}
	if !strings.Contains(markdown, "# Animate") {
		t.Errorf("the workflow body is missing from the inline text")
	}
	if len(included) != 0 {
		t.Errorf("%d chapters were inlined past a 1-byte budget", len(included))
	}
	if len(omitted) != len(g.Rules) {
		t.Errorf("%d chapters omitted, want %d", len(omitted), len(g.Rules))
	}
}

// The budget stops on a chapter boundary: never half a document, and the rest
// stay addressable.
func TestInlineHonoursTheBudget(t *testing.T) {
	lib, err := Default()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	g, ok := lib.Guide("animate a walk cycle for a knight", 8)
	if !ok {
		t.Fatal("no route")
	}

	full, allIncluded, noneOmitted := g.Inline(0)
	if len(allIncluded) != len(g.Rules) || len(noneOmitted) != 0 {
		t.Fatalf("an unlimited inline dropped chapters: %d included, %d omitted", len(allIncluded), len(noneOmitted))
	}

	markdown, included, omitted := g.Inline(len(full) / 2)
	if len(omitted) == 0 {
		t.Fatal("half the full text omitted nothing")
	}
	if len(included)+len(omitted) != len(g.Rules) {
		t.Errorf("%d included + %d omitted, want %d chapters", len(included), len(omitted), len(g.Rules))
	}
	if !strings.HasPrefix(full, markdown) {
		t.Error("the capped text is not a prefix of the full text")
	}

	known := make(map[string]bool)
	for _, chapter := range g.Rules {
		known[chapter.URI()] = true
	}
	for _, chapter := range omitted {
		if !known[chapter.URI()] {
			t.Errorf("omitted chapter %s was never offered", chapter.ID)
		}
	}
}

// A relative link means nothing in a chat message, so the inline copy points at
// the resource instead.
func TestInlineRewritesLinksToResourceURIs(t *testing.T) {
	lib, err := Default()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	g, ok := lib.Guide("animate a walk cycle for a knight", 8)
	if !ok {
		t.Fatal("no route")
	}

	markdown, _, _ := g.Inline(0)
	if !strings.Contains(markdown, "](rules://animation)") {
		t.Error("the references link was not rewritten to a resource URI")
	}
	if strings.Contains(markdown, "../references/") {
		t.Error("a relative reference link survived the rewrite")
	}
}
