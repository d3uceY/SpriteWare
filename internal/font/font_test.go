package font

import (
	"strings"
	"testing"
)

// Every glyph must be seven rows of five characters, or the text drifts.
func TestEveryGlyphIsTheDeclaredSize(t *testing.T) {
	if len(rows) < 50 {
		t.Fatalf("the font has %d glyphs; a usable set is at least digits, capitals and punctuation", len(rows))
	}
	for r := range rows {
		glyph, ok := textRows(r)
		if !ok {
			t.Errorf("%q is malformed", r)
			continue
		}
		for i, row := range glyph {
			if len([]rune(row)) != GlyphWidth {
				t.Errorf("%q row %d is %d wide, want %d: %q", r, i, len(row), GlyphWidth, row)
			}
			for _, c := range row {
				if c != '#' && c != '.' {
					t.Errorf("%q row %d has %q, which is neither ink nor background", r, i, c)
				}
			}
		}
	}
}

func TestEveryPrintableASCIICharacterHasAGlyph(t *testing.T) {
	const wanted = "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789 .,:;!?-+/'()\"#%*=<>_&"
	for _, r := range wanted {
		if _, ok := rows[r]; !ok {
			t.Errorf("no glyph for %q", r)
		}
	}
}

func TestInkIsAbsoluteAndAdvancesByTheCellWidth(t *testing.T) {
	one, err := Ink("I", 10, 4)
	if err != nil {
		t.Fatal(err)
	}
	if len(one) == 0 {
		t.Fatal("I produced no ink")
	}
	for _, p := range one {
		if p[0] < 10 || p[0] >= 10+GlyphWidth || p[1] < 4 || p[1] >= 4+GlyphHeight {
			t.Fatalf("point %v is outside the glyph cell", p)
		}
	}

	// Two letters must not overlap: the second starts one advance later.
	two, err := Ink("II", 10, 4)
	if err != nil {
		t.Fatal(err)
	}
	if len(two) != 2*len(one) {
		t.Fatalf("II has %d points, want %d", len(two), 2*len(one))
	}
	for _, p := range two {
		if p[0] >= 10+Advance {
			if p[0] < 10+Advance || p[0] >= 10+Advance+GlyphWidth {
				t.Fatalf("second glyph point %v is misplaced", p)
			}
		}
	}
}

func TestWidthCountsEveryCell(t *testing.T) {
	if w, err := Width("AB"); err != nil || w != 2*Advance {
		t.Fatalf("Width(AB) = %d, %v", w, err)
	}
	if _, err := Width(""); err == nil {
		t.Fatal("empty text should be refused")
	}
}

// A character outside the table must fail loudly: a blank where a letter
// belongs is worse than an error.
func TestUnknownCharactersAreRefused(t *testing.T) {
	if _, err := Ink("héllo", 0, 0); err == nil {
		t.Fatal("a non-ASCII letter should be refused")
	} else if !strings.Contains(err.Error(), "glyph") {
		t.Errorf("the error should name the problem: %v", err)
	}
	if _, err := Ink("a\nb", 0, 0); err == nil {
		t.Fatal("a newline should be refused, not drawn as a glyph")
	}
}

func TestLowercaseIsDrawnAsCapitals(t *testing.T) {
	lower, err := Ink("ab", 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	upper, err := Ink("AB", 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(lower) != len(upper) {
		t.Fatalf("ab and AB produced different ink: %d vs %d", len(lower), len(upper))
	}
	for i := range lower {
		if lower[i] != upper[i] {
			t.Fatalf("point %d differs: %v vs %v", i, lower[i], upper[i])
		}
	}
}
