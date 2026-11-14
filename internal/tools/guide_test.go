package tools

import (
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// The guide is a tool like any other: if it is registered but not catalogued the
// Tools screen hides it, and if it is catalogued but not registered the screen
// lies.
func TestGuideIsCataloguedAndRegistered(t *testing.T) {
	srv := server.NewMCPServer("test", "0")
	Register(srv, newTestService(t))

	if _, ok := srv.ListTools()["guide"]; !ok {
		t.Fatal("the guide tool is not registered")
	}

	catalogued := false
	for _, info := range Catalog {
		if info.Name == "guide" {
			catalogued = true
			if strings.TrimSpace(info.Blurb) == "" {
				t.Error("the guide has no blurb for the Tools screen")
			}
		}
	}
	if !catalogued {
		t.Error("the guide is missing from the catalogue")
	}
}

func TestGuideCallReturnsAWorkflow(t *testing.T) {
	srv := server.NewMCPServer("test", "0")
	Register(srv, newTestService(t))

	result, ok := toolResult(callTool(t, srv, "guide", map[string]any{
		"task": "animate a walk cycle for a knight",
	}))
	if !ok {
		t.Fatal("the guide call did not return a tool result")
	}
	if result.IsError {
		t.Fatalf("guide returned an error: %s", textOf(result))
	}

	payload := structuredOf(t, result)
	if payload["matched"] != true {
		t.Fatalf("guide did not match an animation task: %v", payload)
	}
	if payload["confidence"] != "high" {
		t.Errorf("confidence = %v, want high", payload["confidence"])
	}
	skillData, _ := payload["skill"].(map[string]any)
	if skillData["uri"] != "skill://animate" {
		t.Errorf("skill = %v, want skill://animate", skillData["uri"])
	}
	rules, _ := payload["rules"].([]any)
	if len(rules) < 3 {
		t.Errorf("%d chapters returned, want at least the workflow's own", len(rules))
	}
}

// The aliases exist because a model that learned another word for "task" should
// not lose the call.
func TestGuideAcceptsTheBriefAlias(t *testing.T) {
	srv := server.NewMCPServer("test", "0")
	Register(srv, newTestService(t))

	result, ok := toolResult(callTool(t, srv, "guide", map[string]any{
		"brief": "pack this for godot",
	}))
	if !ok {
		t.Fatal("no tool result")
	}
	payload := structuredOf(t, result)
	skillData, _ := payload["skill"].(map[string]any)
	if skillData["uri"] != "skill://export" {
		t.Errorf("skill = %v, want skill://export", skillData["uri"])
	}
}

// inline returns the text, and whatever did not fit stays addressable.
func TestGuideInlineReturnsMarkdownAndOmissions(t *testing.T) {
	previous := inlineBudget
	inlineBudget = 12 << 10
	t.Cleanup(func() { inlineBudget = previous })

	srv := server.NewMCPServer("test", "0")
	Register(srv, newTestService(t))

	result, ok := toolResult(callTool(t, srv, "guide", map[string]any{
		"task":   "animate a walk cycle for a knight",
		"inline": true,
	}))
	if !ok {
		t.Fatal("no tool result")
	}
	payload := structuredOf(t, result)

	markdown, _ := payload["markdown"].(string)
	if !strings.Contains(markdown, "# Animate") {
		t.Error("the inline markdown does not start with the workflow body")
	}

	rules, _ := payload["rules"].([]any)
	included, _ := payload["included"].([]any)
	omitted, _ := payload["omitted"].([]any)
	if len(included)+len(omitted) != len(rules) {
		t.Errorf("%d included + %d omitted, want %d chapters", len(included), len(omitted), len(rules))
	}
	if len(omitted) == 0 {
		t.Fatal("a 12 KB budget should not fit every animation chapter")
	}
	if note, _ := payload["omittedNote"].(string); note == "" {
		t.Error("omitted chapters came with no explanation")
	}
}

func TestGuideNoMatchPointsAtTheEntryDocument(t *testing.T) {
	srv := server.NewMCPServer("test", "0")
	Register(srv, newTestService(t))

	result, ok := toolResult(callTool(t, srv, "guide", map[string]any{
		"task": "xyzzy plugh quux",
	}))
	if !ok {
		t.Fatal("no tool result")
	}
	payload := structuredOf(t, result)
	if payload["matched"] != false {
		t.Errorf("an unrelated task matched: %v", payload)
	}
	if payload["entry"] != "skill://spriteware" || payload["rulesIndex"] != "rules://index" {
		t.Errorf("the miss does not point at the entry documents: %v", payload)
	}
}

func TestGuideRequiresATask(t *testing.T) {
	srv := server.NewMCPServer("test", "0")
	Register(srv, newTestService(t))

	result, ok := toolResult(callTool(t, srv, "guide", map[string]any{}))
	if !ok {
		t.Fatal("an empty call did not reach the handler")
	}
	if !result.IsError || !strings.Contains(textOf(result), "guide needs") {
		t.Fatalf("expected a task requirement, got: %s", textOf(result))
	}
}

func TestGuideCapsTheLimit(t *testing.T) {
	srv := server.NewMCPServer("test", "0")
	Register(srv, newTestService(t))

	result, ok := toolResult(callTool(t, srv, "guide", map[string]any{
		"task":  "build a dungeon tileset with walls",
		"limit": 50,
	}))
	if !ok {
		t.Fatal("no tool result")
	}
	rules, _ := structuredOf(t, result)["rules"].([]any)
	if len(rules) > 12 {
		t.Errorf("%d chapters returned, the cap is 12", len(rules))
	}
}

// A draw label is the only prose a draw carries; one rare word is not enough to
// volunteer a route.
func TestAttachGuideIsQuietUnlessConfident(t *testing.T) {
	svc := newTestService(t)

	quiet := map[string]any{"summary": "drew 4 pixels"}
	svc.attachGuide("fibonacci sequence", quiet)
	if _, exists := quiet["guide"]; exists {
		t.Errorf("a weak route was attached: %v", quiet["guide"])
	}

	loud := map[string]any{"summary": "drew 4 pixels"}
	svc.attachGuide("block in a slime", loud)
	if _, exists := loud["guide"]; !exists {
		t.Error("a confident route was not attached")
	}
}

// A session draws many times; the same route ten times is noise.
func TestAttachGuideAnnouncesEachRouteOnce(t *testing.T) {
	svc := newTestService(t)

	first := map[string]any{}
	svc.attachGuide("block in a slime", first)
	if _, exists := first["guide"]; !exists {
		t.Fatal("the first draw should carry the route")
	}

	second := map[string]any{}
	svc.attachGuide("block in a skeleton", second)
	if _, exists := second["guide"]; exists {
		t.Error("the same route was announced twice in one session")
	}

	// A different workflow is new information, so it is attached.
	third := map[string]any{}
	svc.attachGuide("spritesheet atlas for unity", third)
	if _, exists := third["guide"]; !exists {
		t.Error("a different route should be attached")
	}
}

// The description is the only place an agent learns that routing is lexical and
// that the route is not the whole reading list.
func TestGuideDescriptionExplainsLexicalMatching(t *testing.T) {
	srv := server.NewMCPServer("test", "0")
	Register(srv, newTestService(t))

	tool, ok := srv.ListTools()["guide"]
	if !ok {
		t.Fatal("no guide tool")
	}
	description := tool.Tool.Description
	for _, want := range []string{"lexical", "rules://index", "starting point"} {
		if !strings.Contains(description, want) {
			t.Errorf("the guide description does not mention %q", want)
		}
	}
}

// structuredOf unwraps the result's structured payload.
func structuredOf(t *testing.T, result mcp.CallToolResult) map[string]any {
	t.Helper()
	payload, ok := result.StructuredContent.(map[string]any)
	if !ok {
		t.Fatalf("structured content is %T, want an object", result.StructuredContent)
	}
	return payload
}
