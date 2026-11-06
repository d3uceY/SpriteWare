package mcpserver

import (
	"strings"

	"github.com/d3uceY/spriteware/internal/craft"
)

// Palettes are served as a document so an agent can read what "pico8" means
// before asking for it.
func paletteDoc() string {
	var sb strings.Builder
	sb.WriteString("# Palettes\n\n")
	sb.WriteString("Set one with `palette op=set preset=<name>`, or pass an explicit\n")
	sb.WriteString("`colors` list to override any of these.\n\n")

	for _, p := range craft.Presets {
		sb.WriteString("## " + p.Name + "\n\n")
		sb.WriteString(p.Note + "\n\n")
		sb.WriteString("| # | Colour |\n|---|---|\n")
		for i, c := range p.Colours {
			sb.WriteString("| " + itoa(i) + " | `" + c.Hex() + "` |\n")
		}
		sb.WriteString("\n")
	}
	return sb.String()
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	digits := []byte{}
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}
