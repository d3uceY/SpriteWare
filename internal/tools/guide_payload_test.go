package tools

import (
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/server"

	"github.com/d3uceY/spriteware/internal/skill"
)

// A route that only names chapters is still a reading list. The payload has to
// carry the chapters' own Review lines, or a model that never opens a resource
// gets nothing it can act on.
func TestGuideCarriesExecutableChecksAndGates(t *testing.T) {
	srv := server.NewMCPServer("test", "0")
	Register(srv, newTestService(t))

	payload := guideFor(t, srv, "animate a walk cycle for a knight")
	if origin, _ := payload["origin"].(string); origin != "explicit" {
		t.Errorf("origin = %q, want explicit: the agent asked for this route", payload["origin"])
	}
	checks := mapsIn(payload["checks"])
	if len(checks) == 0 {
		t.Fatal("the route carries no checks")
	}
	if len(checks) > guideDigestRules {
		t.Errorf("%d checks, the digest cap is %d", len(checks), guideDigestRules)
	}

	if more, _ := payload["moreChecks"].(float64); int(more) <= 0 {
		t.Errorf("moreChecks = %v; the route withheld Review lines", payload["moreChecks"])
	}

	for _, entry := range checks {
		rule, _ := entry["rule"].(string)
		if rule == "" {
			t.Error("a check entry has no rule id")
		}
		lines := stringsIn(entry["checks"])
		if len(lines) == 0 {
			t.Errorf("%s printed no checks", rule)
		}
		if len(lines) > 2 {
			t.Errorf("%s printed %d checks, the cap is 2", rule, len(lines))
		}
		for _, line := range lines {
			if strings.TrimSpace(line) == "" {
				t.Errorf("%s printed an empty check", rule)
			}
			if len([]rune(line)) > guideCheckChars+1 {
				t.Errorf("%s printed %d runes, the cap is %d", rule, len([]rune(line)), guideCheckChars)
			}
		}
	}

	gates := mapsIn(payload["gates"])
	if len(gates) == 0 {
		t.Fatal("the route carries no always-on gates")
	}
	for _, gate := range gates {
		check, _ := gate["check"].(string)
		if strings.TrimSpace(check) == "" {
			t.Error("a gate has no check text")
		}
		if !strings.HasSuffix(check, ".") {
			t.Errorf("gate is not a sentence: %q", check)
		}
		if len(stringsIn(gate["rules"])) == 0 {
			t.Errorf("gate names no chapter: %q", check)
		}
	}

	if _, exists := payload["markdown"]; exists {
		t.Error("the default route inlined the chapters")
	}
}

// The digest skips a routed chapter with no Review section rather than spending
// one of its three slots on an empty entry. The core chapters are the ones
// without one.
func TestGuideChecksSkipChaptersWithoutReview(t *testing.T) {
	srv := server.NewMCPServer("test", "0")
	Register(srv, newTestService(t))

	payload := guideFor(t, srv, "draw a horse sprite")
	routed := map[string]bool{}
	for _, entry := range mapsIn(payload["rules"]) {
		id, _ := entry["id"].(string)
		routed[id] = true
	}
	if !routed["00-core-principles"] {
		t.Skip("the draw route no longer carries a chapter without a Review section")
	}
	for _, entry := range mapsIn(payload["checks"]) {
		if entry["rule"] == "00-core-principles" {
			t.Error("a chapter without Review took a digest slot")
		}
	}
}

// A gate whose chapters the route already carries is noise. An animation route
// always carries 05-animation and 40-timing-and-spacing, so the animation gate
// must not be printed again.
func TestGuideGatesOmitFullyRoutedGates(t *testing.T) {
	srv := server.NewMCPServer("test", "0")
	Register(srv, newTestService(t))

	payload := guideFor(t, srv, "animate a walk cycle for a knight")

	routed := map[string]bool{}
	for _, entry := range mapsIn(payload["rules"]) {
		id, _ := entry["id"].(string)
		routed[id] = true
	}
	for _, gate := range mapsIn(payload["gates"]) {
		ids := stringsIn(gate["rules"])
		covered := len(ids) > 0
		for _, id := range ids {
			if !routed[id] {
				covered = false
				break
			}
		}
		if covered {
			t.Errorf("gate %q repeats chapters the route already carries (%v)", gate["check"], ids)
		}
	}
}

func guideFor(t *testing.T, srv *server.MCPServer, task string) map[string]any {
	t.Helper()
	result, ok := toolResult(callTool(t, srv, "guide", map[string]any{"task": task}))
	if !ok {
		t.Fatal("the guide call did not return a tool result")
	}
	if result.IsError {
		t.Fatalf("guide(%q) returned an error: %s", task, textOf(result))
	}
	return structuredOf(t, result)
}

// mapsIn normalizes the two shapes a JSON array takes in-process: []any from a
// decoded request, []map[string]any when a test builds the payload by hand.
func mapsIn(value any) []map[string]any {
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

func stringsIn(value any) []string {
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

// The gate set is library data, not a fixture: a renamed chapter must fail here
// rather than disappear from every route.
func TestGuideGateChaptersAreInTheLibrary(t *testing.T) {
	lib, err := skill.Default()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	for _, gate := range lib.Gates() {
		for _, id := range gate.Rules {
			if _, ok := lib.Chapter(id); !ok {
				t.Errorf("gate %q names missing chapter %q", gate.Check, id)
			}
		}
	}
}
