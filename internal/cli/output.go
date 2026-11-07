package cli

import (
	"encoding/json"
	"io"
)

// jsonOut is the global --json switch. Commands with two shapes read it; the
// rest ignore it.
var jsonOut bool

// printJSON writes v indented, because the terminal output of a CLI is read by a
// person as often as it is piped.
func printJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}
