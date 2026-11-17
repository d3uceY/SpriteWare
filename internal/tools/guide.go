package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/d3uceY/spriteware/internal/protocol"
	"github.com/d3uceY/spriteware/internal/skill"
)

// The guide tool is how the skill reaches an agent *during* a task, not only
// when someone installs the folder or reads a resource. The server cannot know
// the user's brief from a draw payload, so it does the two things it can:
//
//  1. expose an explicit router the connect instructions tell the agent to call
//     before specialized work;
//  2. attach a compact route to the first draw and new document of a session,
//     when the label or name they carried is a clear match.
//
// Attaching is gated on Guide.Confident: a single shared word scores the same
// as a clear brief, and noise teaches agents to ignore the field.

// inlineBudget caps how much markdown one inline guide returns. The chapters
// average 15 KB and the largest is 29 KB, so a 24 KB cap could not hold the one
// chapter the route exists for; 48 KB fits the workflow plus two chapters, which
// is the reading budget the entry document sets. Each chapter is whole; the
// rest come back as rules:// URIs the caller can read itself.
//
// A var rather than a const so a test can lower it instead of building 48 KB.
var inlineBudget = 48 << 10

// guideDigestRules is how many routed chapters get their Review lines printed
// in the payload itself. The rest stay a URI: three checks are read, eight are
// skipped, so the default ships the three most likely to be run before the next
// draw.
const guideDigestRules = 3

// guideCheckChars caps one printed check. The full chapter is always one
// rules:// read away; the digest exists to be acted on, not to be complete.
const guideCheckChars = 180

// guideGateChars keeps a gate to one readable line. A gate has no shorter form:
// it is already the whole check.
const guideGateChars = 220

func (s *Service) registerGuide(srv *server.MCPServer) {
	tool := mcp.NewTool("guide",
		mcp.WithDescription(
			"Route a task to the skill's workflow and the chapters to read before drawing.\n"+
				"Call it at the start of any task that needs craft — a subject (hands, a horse, water), a workflow "+
				"(rig, animate, tileset), a colour or palette decision, an export — even when the user did not ask "+
				"for guidance. A decision you cannot cite is a guess: the chapters carry the sizes, the timing "+
				"tables and the failure modes.\n"+
				"Pass the task in the user's own words. It returns skill:// and rules:// resource URIs, the "+
				"Review lines of the top three chapters, the always-on gates, the tokens that matched and a "+
				"confidence level; when nothing matches it returns skill://spriteware and rules://index "+
				"instead of a guess.\n"+
				"Matching is lexical — words are compared, not meaning. A low or empty match means the task's "+
				"words did not overlap the library, not that the library has nothing on the subject.\n"+
				"Treat the route as a starting point, not a reading limit. A session moves: a sprite that starts "+
				"as a drawing becomes a rig, then an animation, then an export. When the work reaches a subject "+
				"this route did not name, read rules://index or any skill:// / rules:// URI directly; every "+
				"chapter stays readable for the whole session.\n"+
				"Set inline=true only when the client cannot read resources. The text is capped by a size budget "+
				"and `omitted` lists the chapters that were left out, so they can be read next."),
		mcp.WithString("task", mcp.Required(),
			mcp.Description("The task in the user's words: what is being drawn, for what, and what it must end up as.")),
		mcp.WithString("brief", mcp.Description("Alias for task.")),
		mcp.WithString("query", mcp.Description("Alias for task.")),
		mcp.WithString("instruction", mcp.Description("Alias for task.")),
		mcp.WithNumber("limit", mcp.Description("Maximum chapters to return (default 6, cap 12).")),
		mcp.WithBoolean("inline", mcp.Description(
			"Return the workflow and chapter text as markdown instead of URIs only. Use it only when the "+
				"client cannot read MCP resources; the text stops at the size budget and `omitted` names the "+
				"chapters to read next.")),
	)

	srv.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return s.handleGuide(req), nil
	})
}

// handleGuide routes a task and returns the pointers, or the text when the
// caller asked for it.
func (s *Service) handleGuide(req mcp.CallToolRequest) *mcp.CallToolResult {
	task := firstTask(req)
	if task == "" {
		return fail(protocol.New(protocol.CodeInvalidArgs, "guide needs `task`").
			WithRemediation("pass the task in the user's words, for example \"animate a walk cycle for a knight\""))
	}

	limit := req.GetInt("limit", 6)
	if limit <= 0 {
		limit = 6
	}
	if limit > 12 {
		limit = 12
	}

	lib, err := skill.Default()
	if err != nil {
		return fail(protocol.New(protocol.CodeInternal, "the skill library did not load: %v", err))
	}

	g, matched := lib.Guide(task, limit)
	if !matched {
		payload := map[string]any{
			"matched":    false,
			"task":       task,
			"entry":      "skill://spriteware",
			"rulesIndex": "rules://index",
			"note": "No workflow matched these words. That means the vocabulary did not overlap, not that " +
				"the library is empty: start at skill://spriteware for the workflow table, or rules://index " +
				"for every subject chapter, and read what the work needs from there.",
		}
		return ok(payload, "no workflow matched; start at skill://spriteware or rules://index")
	}

	if g.Confident() {
		s.recordRoute(g)
	}
	payload := guidePayload(g, "explicit")
	if !g.Confident() {
		payload["note"] = "Low confidence: only one word overlapped the library. Check matchedTokens against " +
			"the brief and add the deliverable or the format for a sharper route, read skill://spriteware, " +
			"or read rules://index directly — a weak route is not a limit on what you may read."
		candidates := make([]map[string]any, 0, len(g.Routes))
		for _, route := range g.Routes {
			candidates = append(candidates, map[string]any{
				"skill":   route.Workflow.URI(),
				"name":    route.Workflow.Name,
				"score":   route.Score,
				"matched": route.Matched,
			})
		}
		payload["candidates"] = candidates
	}

	if req.GetBool("inline", false) {
		markdown, included, omitted := g.Inline(inlineBudget)
		payload["markdown"] = markdown
		payload["included"] = chapterURIs(included)
		payload["omitted"] = chapterURIs(omitted)
		if len(omitted) > 0 {
			payload["omittedNote"] = "markdown stopped at the size budget, so the `omitted` chapters were " +
				"not inlined. They are readable now as rules:// resources, one call each, or call guide " +
				"again without inline for the full list."
		}
	}

	confidence := "low"
	if g.Confident() {
		confidence = "high"
	}
	summary := fmt.Sprintf("routed to %s (%s): %d chapter(s) to read", g.WorkflowURI(), confidence, len(g.Rules))
	return ok(payload, summary)
}

// guidePayload is the compact, stable shape both the tool and the automatic
// attachments use. origin says which one produced it: an attached route is not a
// route the agent asked for, and a transcript cannot tell them apart without it.
func guidePayload(g *skill.Guide, origin string) map[string]any {
	if g == nil || g.Workflow == nil {
		return nil
	}

	rules := make([]map[string]any, 0, len(g.Rules))
	for i, chapter := range g.Rules {
		source := "search"
		if i < len(g.RuleSources) {
			source = g.RuleSources[i]
		}
		rules = append(rules, map[string]any{
			"uri":    chapter.URI(),
			"id":     chapter.ID,
			"title":  chapter.Title,
			"band":   chapter.Band,
			"source": source,
		})
	}

	references := make([]map[string]any, 0, len(g.References))
	for _, ref := range g.References {
		references = append(references, map[string]any{
			"uri":   ref.URI(),
			"name":  ref.Name,
			"title": ref.Title,
		})
	}

	confidence := "low"
	if g.Confident() {
		confidence = "high"
	}

	checks := guideChecks(g)
	gates := guideGates(g)
	digested := 0
	for _, entry := range checks {
		if lines, ok := entry["checks"].([]string); ok {
			digested += len(lines)
		}
	}
	return map[string]any{
		"matched":    true,
		"task":       g.Task,
		"origin":     origin,
		"confidence": confidence,
		"skill": map[string]any{
			"uri":         g.WorkflowURI(),
			"name":        g.Workflow.Name,
			"description": g.Workflow.Description,
		},
		"rules":         rules,
		"references":    references,
		"checks":        checks,
		"moreChecks":    guideCheckTotal(g) - digested,
		"gates":         gates,
		"matchedTokens": g.Matched,
		"note": "Run these checks before the next draw or export, not after delivery. Each line is a " +
			"chapter's own Review item; open the rules:// URI when a check needs the reasoning. When you " +
			"report the result, name the chapter you followed. The route is a sample chosen by word " +
			"overlap: gates apply whatever it says, and rules://index lists the chapters it did not name.",
	}
}

// guideChecks prints the Review lines of the first few routed chapters, so the
// default answer is something to do rather than a list of things to read. A
// chapter with no Review section is skipped rather than spending a slot on an
// empty entry; moreChecks reports how many Review lines were left unprinted.
func guideChecks(g *skill.Guide) []map[string]any {
	if g == nil || g.Workflow == nil {
		return []map[string]any{}
	}
	out := make([]map[string]any, 0, guideDigestRules)
	for _, chapter := range g.Rules {
		if len(out) >= guideDigestRules {
			break
		}
		lines := chapter.Checks()
		if len(lines) == 0 {
			continue
		}
		if len(lines) > 2 {
			lines = lines[:2]
		}
		short := make([]string, 0, len(lines))
		for _, line := range lines {
			short = append(short, truncateText(line, guideCheckChars))
		}
		out = append(out, map[string]any{
			"rule":   chapter.ID,
			"title":  chapter.Title,
			"checks": short,
		})
	}
	return out
}

// guideCheckTotal counts every Review line in the route, so moreChecks can say
// how much of the reading the digest left out rather than only how many
// chapters it skipped.
func guideCheckTotal(g *skill.Guide) int {
	total := 0
	for _, chapter := range g.Rules {
		total += len(chapter.Checks())
	}
	return total
}

// guideGates ships the entry document's always-on checks with every route.
//
// The route is a sample of the library chosen by word overlap; these checks are
// the part that applies whatever the sample says. Only gates whose chapters the
// route does not already carry are printed, so an animation task does not
// repeat the animation gate.
func guideGates(g *skill.Guide) []map[string]any {
	if g == nil || g.Workflow == nil {
		return []map[string]any{}
	}
	lib, err := skill.Default()
	if err != nil {
		return []map[string]any{}
	}
	routed := make(map[string]bool, len(g.Rules))
	for _, chapter := range g.Rules {
		routed[chapter.ID] = true
	}
	out := make([]map[string]any, 0, 6)
	for _, gate := range lib.Gates() {
		covered := len(gate.Rules) > 0
		for _, id := range gate.Rules {
			if !routed[id] {
				covered = false
				break
			}
		}
		if covered {
			continue
		}
		out = append(out, map[string]any{
			"check": truncateText(gate.Check, guideGateChars),
			"rules": gate.Rules,
		})
	}
	return out
}

// truncateText shortens on a word boundary so a digest line never ends
// mid-word; the ellipsis signals that the chapter has the full sentence.
func truncateText(s string, max int) string {
	runes := []rune(strings.TrimSpace(s))
	if len(runes) <= max {
		return string(runes)
	}
	cut := runes[:max]
	for i := len(cut) - 1; i > max/2; i-- {
		if cut[i] == ' ' {
			cut = cut[:i]
			break
		}
	}
	return strings.TrimRight(string(cut), " ,;:") + "…"
}

// firstTask reads the task from any of the aliases the schema accepts.
func firstTask(req mcp.CallToolRequest) string {
	for _, key := range []string{"task", "brief", "query", "instruction"} {
		if value := strings.TrimSpace(req.GetString(key, "")); value != "" {
			return value
		}
	}
	return ""
}

// chapterURIs turns chapters into the URIs a caller can read.
func chapterURIs(chapters []*skill.Chapter) []string {
	out := make([]string, 0, len(chapters))
	for _, chapter := range chapters {
		out = append(out, chapter.URI())
	}
	return out
}

// attachGuide adds the route to a successful result when the text the caller
// did send routes with confidence. It never overwrites an existing guide, never
// decorates a failure, and announces a workflow once per session: a draw-heavy
// session should not be told the same route on every call.
func (s *Service) attachGuide(task string, out map[string]any) {
	if out == nil {
		return
	}
	if _, exists := out["guide"]; exists {
		return
	}
	task = strings.TrimSpace(task)
	if task == "" {
		s.noteMissingRoute(out)
		return
	}

	lib, err := skill.Default()
	if err != nil {
		return
	}
	g, ok := lib.Guide(task, 4)
	if !ok || !g.Confident() {
		s.noteMissingRoute(out)
		return
	}

	s.mu.Lock()
	if s.guideAnnounced == nil {
		s.guideAnnounced = make(map[string]bool)
	}
	announced := s.guideAnnounced[g.Workflow.Name]
	s.guideAnnounced[g.Workflow.Name] = true
	s.guideRoute = g
	s.guideRouted = true
	s.mu.Unlock()
	if announced {
		return
	}

	out["guide"] = guidePayload(g, "attached")
}

// recordRoute keeps the last confident route the session was handed. Validate
// reads it back to say which of those chapters its measured checks cover.
func (s *Service) recordRoute(g *skill.Guide) {
	if g == nil || g.Workflow == nil {
		return
	}
	s.mu.Lock()
	s.guideRoute = g
	s.guideRouted = true
	s.mu.Unlock()
}

// currentRoute returns the route validate should report against, if any.
func (s *Service) currentRoute() *skill.Guide {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.guideRoute
}

// noteMissingRoute makes the omission visible in the tool output, which is the
// one channel an agent cannot skim past. It fires only while no confident route
// has been handed out in this session, so it stops as soon as guide is called
// or a draw routes on its label; a hard error would refuse legitimate work.
func (s *Service) noteMissingRoute(out map[string]any) {
	if out == nil {
		return
	}
	if _, exists := out["guide"]; exists {
		return
	}
	s.mu.Lock()
	routed := s.guideRouted
	s.mu.Unlock()
	if routed {
		return
	}
	out["guideMissing"] = "No skill route has been fetched in this session. Call guide(task=<the brief>) " +
		"before the next composition: it returns the workflow, the chapters and the always-on gates that " +
		"this response did not carry."
}
