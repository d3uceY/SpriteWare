package mcpserver

import (
	"regexp"
	"strings"
	"testing"

	"github.com/d3uceY/spriteware/internal/config"
	"github.com/d3uceY/spriteware/internal/link"
	"github.com/d3uceY/spriteware/internal/skill"
	"github.com/d3uceY/spriteware/internal/tools"
)

func testService(t *testing.T) *tools.Service {
	t.Helper()
	store, err := config.LoadFrom(t.TempDir() + "/config.json")
	if err != nil {
		t.Fatal(err)
	}
	links := link.NewSelector(link.Options{Mode: config.ModeHeadless, BridgePort: 19998})
	t.Cleanup(func() { _ = links.Close() })
	return tools.New(store, links)
}

// Every markdown file has to be reachable as a resource, or nobody reads it.
func TestEverySkillDocumentIsPublished(t *testing.T) {
	published := map[string]bool{}
	for _, r := range Resources() {
		published[r.Path] = true
	}

	for _, rel := range skill.Files() {
		if !strings.HasSuffix(rel, ".md") {
			continue
		}
		if !published[rel] {
			t.Errorf("%s is not published as a resource", rel)
		}
	}
}

func TestResourceURIsUseTheTwoSchemes(t *testing.T) {
	want := []string{
		"skill://spriteware",
		"skill://draw",
		"skill://animate",
		"rules://craft",
		"rules://checks",
		// The subject chapters keep the URIs they were written with, so a
		// cross-reference inside one resolves as published text.
		"rules://index",
		"rules://34-hands-and-feet",
		"rules://92-generated-art-tells",
	}
	got := map[string]bool{}
	for _, r := range Resources() {
		got[r.URI] = true
	}
	for _, uri := range want {
		if !got[uri] {
			t.Errorf("%s is not published", uri)
		}
	}
}

// The entry document must come back byte-identical to the installed file.
func TestReadingAResourceReturnsTheEmbeddedText(t *testing.T) {
	body, ok := ReadResource("skill://spriteware")
	if !ok {
		t.Fatal("skill://spriteware is not readable")
	}
	if body != skill.Text() {
		t.Fatalf("the resource and the installed SKILL.md differ: %d vs %d bytes", len(body), len(skill.Text()))
	}
	if !strings.Contains(body, "name: spriteware") {
		t.Error("the resource is not the entry document")
	}
}

func TestUnknownResourceIsRefused(t *testing.T) {
	if _, ok := ReadResource("skill://nope"); ok {
		t.Fatal("an unknown URI was served")
	}
	if _, ok := ReadResource("https://example.com/x"); ok {
		t.Fatal("a foreign URI was served")
	}
}

// A resource with no description is one an agent has no reason to open.
func TestEveryResourceHasADescription(t *testing.T) {
	for _, r := range Resources() {
		if strings.TrimSpace(r.Description) == "" {
			t.Errorf("%s has no description", r.URI)
		}
		if len([]rune(r.Description)) > 1024 {
			t.Errorf("%s description is %d runes", r.URI, len([]rune(r.Description)))
		}
	}
}

// The instructions list the resources, so an agent learns they exist.
func TestInstructionsIndexTheResources(t *testing.T) {
	index := ResourceIndex()
	for _, uri := range []string{"skill://spriteware", "rules://craft"} {
		if !strings.Contains(index, uri) {
			t.Errorf("the instruction index does not mention %s", uri)
		}
	}
}

// The subject chapters are read on demand, so the instruction block names the
// family once. Listing all 62 would spend the context the split is there to
// save, on every single message.
func TestInstructionsCollapseTheSubjectChapters(t *testing.T) {
	index := ResourceIndex()
	if !strings.Contains(index, "rules://<chapter>") {
		t.Error("the instruction index does not point at the subject chapters")
	}
	if strings.Contains(index, "rules://34-hands-and-feet") {
		t.Error("the instruction index lists a subject chapter individually")
	}
	if n := strings.Count(index, "\n"); n > 30 {
		t.Errorf("the instruction index is %d lines, which is too much to send on every message", n)
	}
}

// A chapter cites its neighbours by rules:// URI, so those have to land on a
// published resource or the reader hits a dead end mid-thought.
func TestEveryCitedRuleResolves(t *testing.T) {
	cited := regexp.MustCompile(`rules://([a-z0-9-]+)`)
	published := map[string]bool{}
	for _, r := range Resources() {
		published[strings.TrimPrefix(r.URI, "rules://")] = true
	}

	for _, rel := range skill.Files() {
		if !strings.HasSuffix(rel, ".md") {
			continue
		}
		body, err := skill.Read(rel)
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range cited.FindAllStringSubmatch(body, -1) {
			if !published[m[1]] {
				t.Errorf("%s cites rules://%s, which is not published", rel, m[1])
			}
		}
	}
}

// Building must not panic: mcp-go treats a duplicate URI as a caller bug, and a
// startup panic is unrecoverable.
func TestBuildingTheServerPublishesEveryResource(t *testing.T) {
	if srv := New(testService(t), "test"); srv == nil {
		t.Fatal("no server")
	}
}
