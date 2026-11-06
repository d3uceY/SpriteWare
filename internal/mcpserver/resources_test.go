package mcpserver

import (
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

// Building must not panic: mcp-go treats a duplicate URI as a caller bug, and a
// startup panic is unrecoverable.
func TestBuildingTheServerPublishesEveryResource(t *testing.T) {
	if srv := New(testService(t), "test"); srv == nil {
		t.Fatal("no server")
	}
}
