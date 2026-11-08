package mcpserver

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// errNoBody means the reply arrived but carried no document.
var errNoBody = errors.New("the server returned no resource contents")

// Reading through the server, not through the package function.
//
// Resources() and ReadResource() are tested above, but nothing proved the
// `resources/read` handler serves them: a client that got an empty reply would
// still pass every test in this package.
func TestEveryResourceReadsBackThroughTheServer(t *testing.T) {
	srv := New(testService(t), "test")

	for _, r := range Resources() {
		body, err := readResourceThroughServer(srv, r.URI)
		if err != nil {
			t.Errorf("%s: %v", r.URI, err)
			continue
		}
		if strings.TrimSpace(body) == "" {
			t.Errorf("%s: served an empty body", r.URI)
			continue
		}
		if r.URI == "skill://spriteware" && !strings.Contains(body, "name: spriteware") {
			t.Errorf("%s: served something that is not the entry document", r.URI)
		}
	}
}

// The four that a client reported as unreadable.
func TestTheReportedResourcesAreServable(t *testing.T) {
	srv := New(testService(t), "test")

	for _, uri := range []string{"skill://new", "skill://animate", "rules://craft", "rules://animation"} {
		body, err := readResourceThroughServer(srv, uri)
		if err != nil {
			t.Errorf("%s: %v", uri, err)
			continue
		}
		if len(body) < 200 {
			t.Errorf("%s: only %d bytes, which is too short to be a document", uri, len(body))
		}
	}
}

// The negative control. Without it, a helper that quietly returned "" for
// everything would still leave the tests above green.
func TestAnUnpublishedResourceIsRefusedThroughTheServer(t *testing.T) {
	srv := New(testService(t), "test")

	if body, err := readResourceThroughServer(srv, "skill://not-a-document"); err == nil {
		t.Fatalf("an unpublished URI was served %d bytes", len(body))
	}
}

func readResourceThroughServer(srv *server.MCPServer, uri string) (string, error) {
	body, err := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "resources/read",
		"params":  map[string]any{"uri": uri},
	})
	if err != nil {
		return "", err
	}

	resp, isResponse := srv.HandleMessage(context.Background(), body).(mcp.JSONRPCResponse)
	if !isResponse {
		return "", errNoBody
	}
	raw, err := json.Marshal(resp.Result)
	if err != nil {
		return "", err
	}
	var result struct {
		Contents []struct {
			Text string `json:"text"`
		} `json:"contents"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return "", err
	}
	if len(result.Contents) == 0 {
		return "", errNoBody
	}
	return result.Contents[0].Text, nil
}
