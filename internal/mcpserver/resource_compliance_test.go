package mcpserver

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

// Resource-compliance eval. It answers one question: if an agent follows the
// protocol (call guide before composing, then read what it names), does the
// route exist, point at the right workflow, and carry enough to act on — or does
// it come back empty and teach the agent that skipping is free?
//
// What this measures: routing, payload cost, the executable checks, and
// resource resolution through the real server. What it cannot measure: whether
// a model obeys. Obedience is a host-orchestration property; the only lever
// here is making the route cheap, correct and impossible to miss.

// guidePayloadBudget is the most a route may cost. A route that ships three
// chapter digests and six gates must stay far below the cost of the chapters
// themselves, or the digest recreates the reading problem it removed.
const guidePayloadBudget = 6000

type complianceCase struct {
	prompt string
	// want lists acceptable workflows, best first. More than one means the
	// brief is genuinely ambiguous and either owner is defensible.
	want []string
}

// pixelPrompts are the tasks the skill exists for: one per workflow, then the
// trigger paths and two ambiguous briefs. A regression in routing should name
// the prompt, not fail a count.
var pixelPrompts = []complianceCase{
	{"animate a walk cycle for a knight", []string{"animate"}},
	{"draw a slime enemy sprite", []string{"draw"}},
	{"add shading to the cloak", []string{"shade"}},
	{"pick a gameboy palette for my platformer", []string{"palette"}},
	{"rig this character into separate layers", []string{"rig"}},
	{"build a dungeon tileset with walls", []string{"tileset"}},
	{"fix the arm, it is one pixel too long", []string{"fix"}},
	{"review this sprite before I ship it", []string{"review"}},
	{"export a spritesheet for Godot", []string{"export"}},
	{"start a new 32 by 32 sprite", []string{"new"}},
	{"make an attack animation with a sword", []string{"animate"}},
	{"my walk cycle slides, fix the feet", []string{"fix", "animate"}},
	{"my wangset has seams", []string{"tileset"}},
	{"pack this for godot", []string{"export"}},
	{"pixel art of a treasure chest", []string{"draw", "shade"}},
	{"four 16x16 hud icons for a dark shop screen", []string{"draw"}},
	{"four hud icons and a png sheet for the shop", []string{"draw"}},
}

// nonPixelPrompts are the negative control. A lexical router will sometimes
// score one shared word; it must not call that a confident route.
var nonPixelPrompts = []string{
	"compute the fibonacci sequence in rust",
	"rename these files in the folder",
	"what is the capital of France",
}

func TestGuideComplianceEval(t *testing.T) {
	srv := New(testService(t), "test")

	routed, failed := 0, 0
	largest := 0
	for _, tc := range pixelPrompts {
		data := callGuide(t, srv, map[string]any{"task": tc.prompt})
		encoded, _ := json.Marshal(data)
		if len(encoded) > largest {
			largest = len(encoded)
		}

		problems := []string{}
		if data["matched"] != true {
			problems = append(problems, "no route")
		}
		if data["confidence"] != "high" {
			problems = append(problems, "low confidence")
		}
		skillData, _ := data["skill"].(map[string]any)
		name, _ := skillData["name"].(string)
		allowed := false
		for _, want := range tc.want {
			if name == want {
				allowed = true
			}
		}
		if !allowed {
			problems = append(problems, fmt.Sprintf("workflow %q, want %v", name, tc.want))
		}

		rules := asMaps(data["rules"])
		checks := asMaps(data["checks"])
		if len(checks) == 0 {
			problems = append(problems, "no checks digest")
		}
		if len(checks) > 3 {
			problems = append(problems, fmt.Sprintf("%d checks, cap is 3", len(checks)))
		}
		routedIDs := map[string]bool{}
		for _, entry := range rules {
			if id, _ := entry["id"].(string); id != "" {
				routedIDs[id] = true
			}
		}
		for _, entry := range checks {
			rule, _ := entry["rule"].(string)
			if rule == "" {
				problems = append(problems, "a check names no rule")
				continue
			}
			if !routedIDs[rule] {
				problems = append(problems, "check names "+rule+", which is not in the route")
			}
			if len(stringList(entry["checks"])) == 0 {
				problems = append(problems, "check entry "+rule+" is empty")
			}
		}
		if more, _ := data["moreChecks"].(float64); int(more) < 0 {
			problems = append(problems, "moreChecks is negative")
		}
		if gates := asMaps(data["gates"]); len(gates) == 0 {
			problems = append(problems, "no always-on gates")
		}
		if len(encoded) > guidePayloadBudget {
			problems = append(problems, fmt.Sprintf("payload %d bytes over the %d budget", len(encoded), guidePayloadBudget))
		}
		if _, exists := data["markdown"]; exists {
			problems = append(problems, "default route inlined the chapters")
		}

		if len(problems) > 0 {
			failed++
			t.Errorf("%-58q -> %s", tc.prompt, strings.Join(problems, "; "))
			continue
		}
		routed++
	}

	t.Logf("pixel prompts routed: %d/%d, largest payload %d bytes (budget %d)",
		routed, len(pixelPrompts), largest, guidePayloadBudget)
	if largest > guidePayloadBudget {
		t.Errorf("largest payload %d exceeds the budget", largest)
	}
	if routed != len(pixelPrompts) {
		t.Errorf("%d pixel prompts did not get a usable route", len(pixelPrompts)-routed)
	}
}

func TestGuideComplianceIgnoresNonPixelWork(t *testing.T) {
	srv := New(testService(t), "test")
	for _, prompt := range nonPixelPrompts {
		data := callGuide(t, srv, map[string]any{"task": prompt})
		if data["confidence"] == "high" {
			t.Errorf("%q was routed with high confidence to %v", prompt, data["skill"])
		}
	}
}

// Every URI a route names has to be readable through the server, or the agent
// follows it into nothing. That includes the check rules and the gate rules,
// which are named outside the rules array.
func TestGuideComplianceResolvesEveryNamedURI(t *testing.T) {
	srv := New(testService(t), "test")
	data := callGuide(t, srv, map[string]any{"task": "animate a walk cycle for a knight"})

	uris := []string{"skill://spriteware", "rules://index"}
	skillData, _ := data["skill"].(map[string]any)
	if uri, _ := skillData["uri"].(string); uri != "" {
		uris = append(uris, uri)
	}
	for _, entry := range asMaps(data["rules"]) {
		if uri, _ := entry["uri"].(string); uri != "" {
			uris = append(uris, uri)
		}
	}
	for _, entry := range asMaps(data["references"]) {
		if uri, _ := entry["uri"].(string); uri != "" {
			uris = append(uris, uri)
		}
	}
	for _, entry := range asMaps(data["checks"]) {
		if rule, _ := entry["rule"].(string); rule != "" {
			uris = append(uris, "rules://"+rule)
		}
	}
	for _, gate := range asMaps(data["gates"]) {
		for _, rule := range stringList(gate["rules"]) {
			uris = append(uris, "rules://"+rule)
		}
	}

	seen := map[string]bool{}
	for _, uri := range uris {
		if seen[uri] {
			continue
		}
		seen[uri] = true
		body, err := readResourceThroughServer(srv, uri)
		if err != nil {
			t.Errorf("read %s: %v", uri, err)
			continue
		}
		if strings.TrimSpace(body) == "" {
			t.Errorf("%s resolved to an empty body", uri)
		}
	}
	t.Logf("resolved %d URIs from one route", len(seen))
}

// asMaps normalizes a decoded JSON array: in-process calls hand back
// []map[string]any, while a real client sees []any.
func asMaps(value any) []map[string]any {
	switch typed := value.(type) {
	case []map[string]any:
		return typed
	case []any:
		out := make([]map[string]any, 0, len(typed))
		for _, item := range typed {
			if entry, ok := item.(map[string]any); ok {
				out = append(out, entry)
			}
		}
		return out
	}
	return nil
}

func stringList(value any) []string {
	switch typed := value.(type) {
	case []string:
		return typed
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}
