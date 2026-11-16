package tools

import (
	"encoding/json"
	"sort"

	"github.com/d3uceY/spriteware/internal/analyze"
	"github.com/d3uceY/spriteware/internal/skill"
)

// checkOwners maps a validate check to the subject chapters that own its rule.
//
// Validate measures pixels, not prose: a check either ran and found something,
// or it did not. The receipt needs the other half of that — which routed
// chapter a measured check belongs to — so "read the rules" and "read nothing"
// do not look the same in a tool result.
var checkOwners = map[string][]string{
	analyze.CheckPalette:    {"01-palette-and-color", "21-limited-and-platform-palettes"},
	analyze.CheckAlpha:      {"12-anti-aliasing"},
	analyze.CheckStrays:     {"11-clusters-and-noise"},
	analyze.CheckOrphans:    {"11-clusters-and-noise"},
	analyze.CheckGaps:       {"03-silhouette-and-form"},
	analyze.CheckBounds:     {"91-composition-and-scenes"},
	analyze.CheckOutline:    {"04-outlines-and-edges"},
	analyze.CheckBanding:    {"02-shading-and-light", "24-lighting-scenarios"},
	analyze.CheckLayers:     {"06-layers-and-rigging"},
	analyze.CheckAnimation:  {"05-animation", "40-timing-and-spacing"},
	analyze.CheckReadiness:  {"07-review-checklist"},
	analyze.CheckVolume:     {"42-walk-and-run", "45-secondary-motion"},
	analyze.CheckUI:         {"83-ui-and-icons"},
	analyze.CheckExpect:     {"07-review-checklist"},
	analyze.CheckCompliance: {"07-review-checklist"},
}

// reportPayload converts an analyze.Report into the map shape validate returns,
// because the receipt is added to it. The JSON shape is unchanged.
func reportPayload(report *analyze.Report) map[string]any {
	if report == nil {
		return map[string]any{}
	}
	body, err := json.Marshal(report)
	if err != nil {
		return map[string]any{"summary": report.Summary}
	}
	var out map[string]any
	if err := json.Unmarshal(body, &out); err != nil {
		return map[string]any{"summary": report.Summary}
	}
	return out
}

// attachGuideReceipt closes the loop a guide opens. Validate runs measured,
// chapter-backed checks; the receipt says which routed chapters those checks
// cover, which are still prose the agent has to apply itself, and which chapter
// to read when a failing check was outside the route.
func (s *Service) attachGuideReceipt(payload map[string]any, report *analyze.Report) {
	if payload == nil || report == nil {
		return
	}
	if _, isError := payload["error"]; isError {
		return
	}
	g := s.currentRoute()
	if g == nil || g.Workflow == nil || len(g.Rules) == 0 {
		return
	}

	ran := make(map[string]bool, len(report.Ran))
	for _, check := range report.Ran {
		ran[check] = true
	}

	failed := make(map[string]int)
	severity := make(map[string]string)
	for _, finding := range report.Findings {
		if !ran[finding.Check] || finding.Severity == analyze.Note {
			continue
		}
		failed[finding.Check]++
		if severity[finding.Check] != string(analyze.Error) {
			severity[finding.Check] = string(finding.Severity)
		}
	}

	routed := make(map[string]bool, len(g.Rules))
	checked := make([]string, 0, len(g.Rules))
	pending := make([]string, 0, len(g.Rules))
	for _, chapter := range g.Rules {
		routed[chapter.ID] = true
		if chapterMeasured(chapter.ID, ran) {
			checked = append(checked, chapter.ID)
			continue
		}
		pending = append(pending, chapter.ID)
	}

	payload["guideReceipt"] = map[string]any{
		"workflow": g.WorkflowURI(),
		"checked":  checked,
		"pending":  pending,
		"failed":   failureList(failed, severity),
		"readNext": readNextChapters(routed, failed),
		"note": "checked chapters had a validate check that measures them; a failure still needs a fix. " +
			"pending chapters are prose, or their check was not requested in this run, so apply them " +
			"yourself before the next draw. readNext names a chapter that owns a failing check when the " +
			"route did not carry it.",
	}
}

// chapterMeasured reports whether one ran check is owned by the chapter.
func chapterMeasured(id string, ran map[string]bool) bool {
	for check, owners := range checkOwners {
		if !ran[check] {
			continue
		}
		for _, owner := range owners {
			if owner == id {
				return true
			}
		}
	}
	return false
}

// failureList reports the checks that produced an error or warning, sorted by
// check so two runs of the same art print the same receipt.
func failureList(failed map[string]int, severity map[string]string) []map[string]any {
	names := make([]string, 0, len(failed))
	for name := range failed {
		names = append(names, name)
	}
	sort.Strings(names)
	out := make([]map[string]any, 0, len(names))
	for _, name := range names {
		out = append(out, map[string]any{
			"check":    name,
			"severity": severity[name],
			"findings": failed[name],
		})
	}
	return out
}

// readNextChapters answers the route's blind spot with evidence: a check
// failed, and the chapter that defines the check was not in the list the agent
// was given. That names the missing chapter from the artifact instead of asking
// the agent to notice that its route is a sample.
func readNextChapters(routed map[string]bool, failed map[string]int) []map[string]any {
	if len(failed) == 0 {
		return []map[string]any{}
	}
	lib, err := skill.Default()
	if err != nil {
		return []map[string]any{}
	}

	checks := make([]string, 0, len(failed))
	for check := range failed {
		checks = append(checks, check)
	}
	sort.Strings(checks)

	seen := make(map[string]bool, len(checks))
	out := make([]map[string]any, 0, 4)
	for _, check := range checks {
		for _, owner := range checkOwners[check] {
			if routed[owner] || seen[owner] {
				continue
			}
			seen[owner] = true
			entry := map[string]any{
				"rule":    "rules://" + owner,
				"check":   check,
				"because": "this check failed and the chapter that owns it was not in the route",
			}
			if chapter, ok := lib.Chapter(owner); ok {
				entry["title"] = chapter.Title
				if lines := chapter.Checks(); len(lines) > 0 {
					entry["read"] = truncateText(lines[0], 140)
				}
			}
			out = append(out, entry)
			if len(out) >= 4 {
				return out
			}
		}
	}
	return out
}
