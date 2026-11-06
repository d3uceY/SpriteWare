package craft

import (
	"strings"
	"testing"
)

// A preset is a promise about what "PICO-8" means: the published counts, so a
// typo in a hex value is caught here rather than in the art.
func TestPresetsAreThePalettesTheyClaimToBe(t *testing.T) {
	wantSizes := map[string]int{"pico8": 16, "gameboy": 4, "sweetie16": 16}
	for _, p := range Presets {
		size, known := wantSizes[p.Name]
		if !known {
			t.Errorf("preset %q has no expected size, so the test cannot verify it", p.Name)
			continue
		}
		if len(p.Colours) != size {
			t.Errorf("%s has %d colours, want %d", p.Name, len(p.Colours), size)
		}
		if strings.TrimSpace(p.Note) == "" {
			t.Errorf("%s has no note, so nothing says when to use it", p.Name)
		}
	}
}

func TestPresetColoursAreOpaqueAndDistinct(t *testing.T) {
	for _, p := range Presets {
		seen := map[string]bool{}
		for _, c := range p.Colours {
			if c.A != 255 {
				t.Errorf("%s contains %s with alpha %d", p.Name, c.Hex(), c.A)
			}
			key := c.Hex()
			if seen[key] {
				t.Errorf("%s repeats %s", p.Name, key)
			}
			seen[key] = true
		}
	}
}

func TestPresetLookupIsCaseInsensitive(t *testing.T) {
	for _, name := range []string{"pico8", "PICO8", "Pico8"} {
		if _, ok := PresetNamed(name); !ok {
			t.Errorf("%q was not found", name)
		}
	}
	if _, ok := PresetNamed("no-such-palette"); ok {
		t.Error("an unknown name matched a preset")
	}
}

func TestPresetNamesCoverEveryPreset(t *testing.T) {
	if len(PresetNames()) != len(Presets) {
		t.Fatalf("%d names for %d presets", len(PresetNames()), len(Presets))
	}
}
