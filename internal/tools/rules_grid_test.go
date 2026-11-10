package tools

import (
	"regexp"
	"strings"
	"testing"

	"github.com/d3uceY/spriteware/internal/skill"
)

// The rule chapters carry grid templates written by another project. Shipping
// them is only honest if `draw` op `grid` actually reads them, so every
// template goes through the same compiler a caller's grid goes through.
//
// A failure here means a template an agent is told to copy would be refused at
// the tool, so the fix belongs in the chapter rather than in this test.
func TestRuleGridTemplatesCompile(t *testing.T) {
	legendLine := regexp.MustCompile(`^(\S)\s*=\s*.*?(#[0-9a-fA-F]{3,8})\s*$`)

	blocks := 0
	for _, rel := range skill.Files() {
		if !strings.HasPrefix(rel, "rules/") || !strings.HasSuffix(rel, ".md") {
			continue
		}
		body, err := skill.Read(rel)
		if err != nil {
			t.Fatal(err)
		}

		for _, tmpl := range gridTemplates(body) {
			blocks++
			legend := map[string]any{}
			for _, line := range tmpl.legend {
				m := legendLine.FindStringSubmatch(strings.TrimSpace(line))
				if m == nil {
					t.Errorf("%s: template %q has a legend line this test cannot read: %q", rel, tmpl.name, line)
					continue
				}
				legend[m[1]] = m[2]
			}
			if len(legend) == 0 {
				t.Errorf("%s: template %q declares no colours", rel, tmpl.name)
				continue
			}

			entry := gridEntry(tmpl.rows, legend, "")
			entry["x"], entry["y"] = float64(0), float64(0)
			if _, err := compileGrid(entry, 0); err != nil {
				t.Errorf("%s: template %q is not a grid `draw` accepts: %v", rel, tmpl.name, err)
			}
		}
	}

	if blocks < 50 {
		t.Fatalf("found %d grid templates in the rules; the chapters did not ship in full", blocks)
	}
}

type template struct {
	name   string
	legend []string
	rows   []string
}

// gridTemplates pulls out every ```grid fence: legend lines up to the `---`
// rule, then the rows, exactly as the chapters write them.
func gridTemplates(body string) []template {
	lines := strings.Split(strings.ReplaceAll(body, "\r\n", "\n"), "\n")
	var out []template

	for i := 0; i < len(lines); i++ {
		line := lines[i]
		if !strings.HasPrefix(line, "```grid") {
			continue
		}
		current := template{name: strings.TrimSpace(strings.TrimPrefix(line, "```grid"))}
		inRows := false

		i++
		for ; i < len(lines); i++ {
			if strings.HasPrefix(lines[i], "```") {
				break
			}
			if !inRows {
				if strings.TrimSpace(lines[i]) == "---" {
					inRows = true
					continue
				}
				current.legend = append(current.legend, lines[i])
				continue
			}
			current.rows = append(current.rows, lines[i])
		}
		out = append(out, current)
	}
	return out
}
