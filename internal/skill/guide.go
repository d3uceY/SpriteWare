package skill

import (
	"regexp"
	"sort"
	"strings"
)

// Guide is one routed task: the workflow, the chapters worth reading, and why
// the router chose them. It is the in-task counterpart to reading the whole
// skill tree: the URIs are cheap to return, and the caller pays for the bodies
// only when it reads them.
type Guide struct {
	Task     string
	Workflow *Workflow
	// Score is the IDF-weighted overlap for the chosen workflow. It is only
	// comparable within the call that produced it.
	Score float64
	// Matched lists the task tokens that contributed.
	Matched []string
	// Routes is the ranked shortlist, best first.
	Routes []Route
	// Rules are the workflow's own chapters first, then the best search fills.
	Rules []*Chapter
	// RuleSources records why each chapter is present: "workflow" or "search".
	RuleSources []string
	// References are the lookup notes the workflow links.
	References []*Reference
}

// Guide routes a task and returns the workflow plus the chapters to read.
// ok is false when the task shares no vocabulary with the library at all, so a
// caller can say "no route" instead of attaching an irrelevant one.
//
// maxRules bounds the chapter list and defaults to 8, the upper end of the
// reading budget the entry document sets.
func (l *Library) Guide(task string, maxRules int) (*Guide, bool) {
	task = strings.TrimSpace(task)
	if task == "" {
		return nil, false
	}
	routes := l.Route(task, 3)
	if len(routes) == 0 {
		return nil, false
	}
	best := routes[0]
	if best.Score < 0.5 || len(best.Matched) == 0 {
		return nil, false
	}
	if maxRules <= 0 {
		maxRules = 8
	}

	g := &Guide{Task: task, Workflow: best.Workflow, Score: best.Score, Matched: best.Matched, Routes: routes}

	seen := make(map[string]bool)
	add := func(chapter *Chapter, source string) bool {
		if chapter == nil || seen[chapter.ID] || len(g.Rules) >= maxRules {
			return false
		}
		seen[chapter.ID] = true
		g.Rules = append(g.Rules, chapter)
		g.RuleSources = append(g.RuleSources, source)
		return true
	}

	// The workflow's own chapters are all worth reading, but not equally: a
	// walk task wants "Walk and run" before "Idle and breathing". Order them by
	// how many of the task's words the chapter's own title carries, then by the
	// search rank, then by declaration order. A body mentioning "walk" once is
	// not the same signal as a chapter about walking. This ordering also
	// decides which chapters survive an inline budget.
	hits := l.Search(task, 0)
	rank := make(map[string]int, len(hits))
	for i, hit := range hits {
		rank[hit.Chapter.ID] = i
	}
	taskTokens := uniqueTokens(task)
	titleHits := make(map[string]int)
	for _, id := range best.Workflow.Rules {
		chapter, ok := l.Chapter(id)
		if !ok {
			continue
		}
		title := strings.ToLower(chapter.Title + " " + chapter.ID)
		for _, token := range taskTokens {
			if strings.Contains(title, token) {
				titleHits[id]++
			}
		}
	}

	own := make([]*Chapter, 0, len(best.Workflow.Rules))
	for _, id := range best.Workflow.Rules {
		if chapter, ok := l.Chapter(id); ok {
			own = append(own, chapter)
		}
	}
	sort.SliceStable(own, func(i, j int) bool {
		if titleHits[own[i].ID] != titleHits[own[j].ID] {
			return titleHits[own[i].ID] > titleHits[own[j].ID]
		}
		ri, iRanked := rank[own[i].ID]
		rj, jRanked := rank[own[j].ID]
		switch {
		case iRanked && jRanked:
			return ri < rj
		case iRanked:
			return true
		case jRanked:
			return false
		}
		return false
	})
	for _, chapter := range own {
		add(chapter, "workflow")
	}

	for _, hit := range hits {
		if len(g.Rules) >= maxRules {
			break
		}
		add(hit.Chapter, "search")
	}
	for _, name := range best.Workflow.References {
		if ref, ok := l.Reference(name); ok {
			g.References = append(g.References, ref)
		}
	}
	return g, true
}

// Confident reports whether the route has enough signal to volunteer itself.
//
// Lexical routing gives one rare word the same score as a clear brief, so an
// automatically attached guide needs either two overlapping content words or a
// word that names the workflow or is one of its triggers. An explicit guide
// call may still use a single-word match: the caller sees Matched and decides.
func (g *Guide) Confident() bool {
	if g == nil {
		return false
	}
	content := 0
	for _, matched := range g.Matched {
		if strings.HasSuffix(matched, "(named)") || strings.HasSuffix(matched, "(trigger)") {
			return true
		}
		content++
	}
	return content >= 2
}

// Inline returns the reading material itself: the workflow body, then each
// chapter that fits inside maxBytes. Chapters are never split, so a caller
// always gets whole documents; the ones that did not fit come back in omitted
// and stay readable as rules:// resources. A zero or negative budget means no
// limit, and the workflow body is always included so the result is never empty.
func (g *Guide) Inline(maxBytes int) (markdown string, included, omitted []*Chapter) {
	if g == nil || g.Workflow == nil {
		return "", nil, nil
	}

	var b strings.Builder
	b.WriteString(resourceLinks(g.Workflow.Body))

	// The budget stops at the first chapter that would not fit, so the inline
	// text is always the workflow plus a contiguous run of chapters. Skipping a
	// chapter and taking a later, smaller one would look like the guide had
	// reordered the reading.
	stopped := false
	for _, chapter := range g.Rules {
		if stopped {
			omitted = append(omitted, chapter)
			continue
		}
		block := "\n\n---\n\n" + resourceLinks(chapter.Body)
		if maxBytes > 0 && b.Len()+len(block) > maxBytes {
			stopped = true
			omitted = append(omitted, chapter)
			continue
		}
		b.WriteString(block)
		included = append(included, chapter)
	}
	return b.String(), included, omitted
}

// WorkflowURI is the MCP resource URI for the routed workflow.
func (g *Guide) WorkflowURI() string {
	if g == nil || g.Workflow == nil {
		return ""
	}
	return g.Workflow.URI()
}

// URI is the MCP resource URI for a workflow.
func (w *Workflow) URI() string { return "skill://" + w.Name }

// URI is the MCP resource URI for a subject chapter.
func (c *Chapter) URI() string { return "rules://" + c.ID }

// URI is the MCP resource URI for a lookup note. References are published under
// the rules:// scheme, so a link to one resolves the same way a chapter does.
func (r *Reference) URI() string { return "rules://" + r.Name }

// resourceLinks rewrites the skill's relative markdown links into MCP URIs, so
// an inlined document is navigable in a client that never saw the file tree.
// Only the inline copy is rewritten; served resources stay byte-identical.
var (
	referenceLinkRe = regexp.MustCompile(`\]\((?:\.\./)?references/([A-Za-z0-9._-]+)\.md\)`)
	ruleLinkRe      = regexp.MustCompile(`\]\((?:\.\./)?rules/([A-Za-z0-9._-]+)\.md\)`)
	workflowLinkRe  = regexp.MustCompile(`\]\((?:\.\./)?([a-z0-9-]+)/SKILL\.md\)`)
)

func resourceLinks(markdown string) string {
	markdown = referenceLinkRe.ReplaceAllString(markdown, "]("+rulesScheme+"$1)")
	markdown = ruleLinkRe.ReplaceAllString(markdown, "]("+rulesScheme+"$1)")
	markdown = workflowLinkRe.ReplaceAllString(markdown, "]("+skillScheme+"$1)")
	return strings.TrimLeft(markdown, "\n")
}

// The schemes are spelled here as well as in mcpserver because that package
// imports this one.
const (
	rulesScheme = "rules://"
	skillScheme = "skill://"
)
