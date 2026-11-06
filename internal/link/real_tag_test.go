package link

import (
	"context"
	"testing"
)

// Rename breaks when the name and the range end share one field: the tag would
// silently move.
func TestIntegrationTagUpdateRenamesAndRetimes(t *testing.T) {
	h := realHeadless(t)
	newDoc(t, h, 6, 6)
	for i := 0; i < 3; i++ {
		if err := h.Call(context.Background(), "frame.add", map[string]any{"count": 1}, &map[string]any{}); err != nil {
			t.Fatalf("frame.add: %v", err)
		}
	}
	if err := h.Call(context.Background(), "tag.create", map[string]any{
		"name": "walk", "from": 1, "to": 2,
	}, &map[string]any{}); err != nil {
		t.Fatalf("tag.create: %v", err)
	}

	if err := h.Call(context.Background(), "tag.update", map[string]any{
		"name": "walk", "rename": "run", "to": 4,
	}, &map[string]any{}); err != nil {
		t.Fatalf("tag.update: %v", err)
	}

	d := mustDump(t, h)
	if len(d.Tags) != 1 {
		t.Fatalf("%d tags after an update, want 1", len(d.Tags))
	}
	tag := d.Tags[0]
	if tag.Name != "run" {
		t.Errorf("name = %q, want run", tag.Name)
	}
	if tag.From != 1 || tag.To != 4 {
		t.Errorf("range = %d..%d, want 1..4", tag.From, tag.To)
	}
}

func TestIntegrationTagUpdateRefusesAnUnknownTag(t *testing.T) {
	h := realHeadless(t)
	newDoc(t, h, 4, 4)

	err := h.Call(context.Background(), "tag.update", map[string]any{"name": "nope", "to": 2}, &map[string]any{})
	if err == nil {
		t.Fatal("updating a tag that does not exist was accepted")
	}
}

// A tag that would start after it ends is a mistake, not a range.
func TestIntegrationTagUpdateRefusesAnInvertedRange(t *testing.T) {
	h := realHeadless(t)
	newDoc(t, h, 4, 4)
	for i := 0; i < 3; i++ {
		if err := h.Call(context.Background(), "frame.add", map[string]any{"count": 1}, &map[string]any{}); err != nil {
			t.Fatalf("frame.add: %v", err)
		}
	}
	if err := h.Call(context.Background(), "tag.create", map[string]any{
		"name": "walk", "from": 2, "to": 4,
	}, &map[string]any{}); err != nil {
		t.Fatalf("tag.create: %v", err)
	}

	err := h.Call(context.Background(), "tag.update", map[string]any{"name": "walk", "from": 4, "to": nil}, &map[string]any{})
	if err != nil {
		// Setting from to the current end is still valid.
		t.Fatalf("a valid retime was refused: %v", err)
	}
	err = h.Call(context.Background(), "tag.update", map[string]any{"name": "walk", "from": 4, "to": 1}, &map[string]any{})
	if err == nil {
		t.Fatal("an inverted range was accepted")
	}
}
