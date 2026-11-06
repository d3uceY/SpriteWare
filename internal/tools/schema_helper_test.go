package tools

import (
	"encoding/json"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

// schemaJSON returns the schema a host actually receives.
//
// mcp-go keeps a hand-written schema in RawInputSchema and marshals nothing for
// it, so encoding InputSchema sees `{"type":"object"}`. draw and validate are
// raw, which made the array-items test vacuous for the two largest tools.
func schemaJSON(t *testing.T, raw json.RawMessage, fallback mcp.ToolInputSchema) json.RawMessage {
	t.Helper()
	if len(raw) > 0 {
		return raw
	}
	encoded, err := json.Marshal(fallback)
	if err != nil {
		t.Fatalf("schema does not encode: %v", err)
	}
	return encoded
}
