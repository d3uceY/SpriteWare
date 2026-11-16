package tools

import (
	"testing"

	"github.com/mark3labs/mcp-go/server"

	"github.com/d3uceY/spriteware/internal/analyze"
	"github.com/d3uceY/spriteware/internal/skill"
)

// The receipt is the answer to "read the rules" and "read nothing" looking the
// same in a tool result.
func TestValidateReceiptReportsCheckedPendingAndReadNext(t *testing.T) {
	svc := newTestService(t)
	route := guideRoute(t, "animate a walk cycle for a knight")
	svc.recordRoute(route)

	report := &analyze.Report{
		Ran: []string{"palette", "strays", "animation", "volume"},
		Findings: []analyze.Finding{
			{Check: "strays", Severity: analyze.Error, Message: "3 isolated pixels"},
			{Check: "palette", Severity: analyze.Warning, Message: "one colour is off-palette"},
			{Check: "volume", Severity: analyze.Note, Message: "a note is not a failure"},
		},
	}
	payload := reportPayload(report)
	svc.attachGuideReceipt(payload, report)

	receipt, _ := payload["guideReceipt"].(map[string]any)
	if receipt == nil {
		t.Fatal("validate came back with no guideReceipt")
	}
	if receipt["workflow"] != "skill://animate" {
		t.Errorf("receipt workflow = %v, want skill://animate", receipt["workflow"])
	}

	checked := stringsIn(receipt["checked"])
	pending := stringsIn(receipt["pending"])
	for _, want := range []string{"05-animation", "40-timing-and-spacing", "42-walk-and-run"} {
		if !contains(checked, want) {
			t.Errorf("%s should be checked; checked = %v", want, checked)
		}
	}
	if !contains(pending, "41-idle-and-breathing") {
		t.Errorf("41-idle-and-breathing should be pending; pending = %v", pending)
	}
	if contains(checked, "41-idle-and-breathing") {
		t.Error("a chapter no validate check covers was reported as checked")
	}

	failed := mapsIn(receipt["failed"])
	if len(failed) != 2 {
		t.Fatalf("%d failing checks, want strays and palette: %v", len(failed), failed)
	}
	byCheck := map[string]map[string]any{}
	for _, entry := range failed {
		name, _ := entry["check"].(string)
		byCheck[name] = entry
	}
	if got := byCheck["strays"]["severity"]; got != string(analyze.Error) {
		t.Errorf("strays severity = %v, want error", got)
	}
	if got := byCheck["palette"]["severity"]; got != string(analyze.Warning) {
		t.Errorf("palette severity = %v, want warning", got)
	}
	if _, exists := byCheck["volume"]; exists {
		t.Error("a note was reported as a failure")
	}

	readNext := mapsIn(receipt["readNext"])
	if len(readNext) == 0 {
		t.Fatal("a failing check outside the route named no chapter to read")
	}
	foundStrays := false
	for _, entry := range readNext {
		if entry["rule"] == "rules://11-clusters-and-noise" {
			foundStrays = true
			if entry["check"] != "strays" {
				t.Errorf("readNext entry names check %v, want strays", entry["check"])
			}
			if entry["title"] == "" || entry["read"] == "" {
				t.Errorf("readNext entry is not actionable: %v", entry)
			}
		}
	}
	if !foundStrays {
		t.Errorf("readNext does not name the chapter that owns strays: %v", readNext)
	}
}

// A clean validate is still a receipt: the route's prose chapters remain the
// agent's job, and saying so is the difference between measured and read.
func TestValidateReceiptSurvivesACleanReport(t *testing.T) {
	svc := newTestService(t)
	svc.recordRoute(guideRoute(t, "animate a walk cycle for a knight"))

	report := &analyze.Report{Ran: []string{"animation"}}
	payload := reportPayload(report)
	svc.attachGuideReceipt(payload, report)

	receipt, _ := payload["guideReceipt"].(map[string]any)
	if receipt == nil {
		t.Fatal("a clean validate lost its receipt")
	}
	if len(stringsIn(receipt["checked"])) == 0 {
		t.Error("no chapter is reported as measured")
	}
	if len(stringsIn(receipt["pending"])) == 0 {
		t.Error("no chapter is reported as prose")
	}
	if got := len(mapsIn(receipt["readNext"])); got != 0 {
		t.Errorf("a clean validate named %d chapters to read next", got)
	}
}

func TestValidateReceiptNeedsARoute(t *testing.T) {
	svc := newTestService(t)
	payload := reportPayload(&analyze.Report{Ran: []string{"strays"}})
	svc.attachGuideReceipt(payload, &analyze.Report{Ran: []string{"strays"}})

	if _, exists := payload["guideReceipt"]; exists {
		t.Fatal("a session with no route received a receipt")
	}
}

func TestValidateReceiptIgnoresAnErrorPayload(t *testing.T) {
	svc := newTestService(t)
	svc.recordRoute(guideRoute(t, "animate a walk cycle for a knight"))

	payload := map[string]any{"error": "no document open"}
	svc.attachGuideReceipt(payload, &analyze.Report{Ran: []string{"strays"}})
	if _, exists := payload["guideReceipt"]; exists {
		t.Fatal("an error payload was decorated with a receipt")
	}
}

// The mapping is data. A check with no owner, or an owner that is not a
// chapter, silently makes a routed chapter unmeasurable.
func TestEveryValidateCheckHasARealOwnerChapter(t *testing.T) {
	lib, err := skill.Default()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	for _, check := range append(append([]string{}, analyze.AllChecks...), analyze.CheckUI, analyze.CheckExpect, analyze.CheckCompliance) {
		owners, ok := checkOwners[check]
		if !ok || len(owners) == 0 {
			t.Errorf("validate check %q has no owning chapter", check)
			continue
		}
		for _, id := range owners {
			if _, ok := lib.Chapter(id); !ok {
				t.Errorf("check %q names missing chapter %q", check, id)
			}
		}
	}
}

// A session that never routed has no guide and no receipt, so the omission has
// to be visible on the result itself.
func TestAttachGuideNagsWhileTheSessionIsUnrouted(t *testing.T) {
	svc := newTestService(t)

	out := map[string]any{"summary": "drew 4 pixels"}
	svc.attachGuide("", out)
	if _, exists := out["guide"]; exists {
		t.Error("an empty task attached a route")
	}
	if _, exists := out["guideMissing"]; !exists {
		t.Error("an unrouted draw came back without guideMissing")
	}

	weak := map[string]any{}
	svc.attachGuide("fibonacci sequence", weak)
	if _, exists := weak["guideMissing"]; !exists {
		t.Error("a weak match stopped the nag without routing")
	}
}

// The nag is a nudge, not wallpaper: one confident route ends it.
func TestGuideMissingStopsAfterAConfidentRoute(t *testing.T) {
	svc := newTestService(t)

	routed := map[string]any{}
	svc.attachGuide("block in a slime", routed)
	if _, exists := routed["guide"]; !exists {
		t.Fatal("a confident label did not attach a route")
	}
	if _, exists := routed["guideMissing"]; exists {
		t.Error("a routed result also carried guideMissing")
	}

	quiet := map[string]any{}
	svc.attachGuide("", quiet)
	if _, exists := quiet["guideMissing"]; exists {
		t.Error("the session kept nagging after a confident route")
	}
}

// An explicit guide call ends the nag too, even when no composition has carried
// the route yet.
func TestExplicitGuideMarksTheSessionRouted(t *testing.T) {
	svc := newTestService(t)
	srv := server.NewMCPServer("test", "0")
	Register(srv, svc)

	if payload := guideFor(t, srv, "animate a walk cycle for a knight"); payload["matched"] != true {
		t.Fatalf("guide did not route: %v", payload)
	}

	out := map[string]any{"summary": "drew 1 pixel"}
	svc.attachGuide("", out)
	if _, exists := out["guideMissing"]; exists {
		t.Error("a session that called guide was still told it had no route")
	}
}

// A result that already carries a guide must not also be told one is missing.
func TestNoteMissingRouteDoesNotDrownAnAttachedGuide(t *testing.T) {
	svc := newTestService(t)
	out := map[string]any{"guide": map[string]any{"skill": "skill://draw"}}
	svc.noteMissingRoute(out)
	if _, exists := out["guideMissing"]; exists {
		t.Error("a result with a guide was also marked guideMissing")
	}
}

func guideRoute(t *testing.T, task string) *skill.Guide {
	t.Helper()
	lib, err := skill.Default()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	route, ok := lib.Guide(task, 8)
	if !ok || !route.Confident() {
		t.Fatalf("Guide(%q) did not produce a confident route", task)
	}
	return route
}

func contains(list []string, want string) bool {
	for _, item := range list {
		if item == want {
			return true
		}
	}
	return false
}
