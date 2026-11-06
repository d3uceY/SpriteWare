// Package font is the bitmap font SpriteWare draws text with.
//
// It is deliberately tiny: 5x7 glyphs, capitals only. A shipped font beats
// hand-typing a grid and miscounting a row.
package font

import (
	"fmt"
	"strings"
)

// Glyph metrics. A cell is 6 wide including the gap, so Width(s) is exact.
const (
	GlyphWidth  = 5
	GlyphHeight = 7
	Advance     = GlyphWidth + 1
)

// Rows maps a rune to seven rows of five characters: '#' is ink, '.' is none.
// Lowercase is drawn as capitals.
var rows = map[rune]string{
	' ':  "...../...../...../...../...../...../.....",
	'A':  ".###./#...#/#...#/#####/#...#/#...#/#...#",
	'B':  "####./#...#/#...#/####./#...#/#...#/####.",
	'C':  ".###./#...#/#..../#..../#..../#...#/.###.",
	'D':  "####./#...#/#...#/#...#/#...#/#...#/####.",
	'E':  "#####/#..../#..../####./#..../#..../#####",
	'F':  "#####/#..../#..../####./#..../#..../#....",
	'G':  ".###./#...#/#..../#.###/#...#/#...#/.###.",
	'H':  "#...#/#...#/#...#/#####/#...#/#...#/#...#",
	'I':  ".###./..#../..#../..#../..#../..#../.###.",
	'J':  "..###/...#./...#./...#./...#./#..#./.##..",
	'K':  "#...#/#..#./#.#../##.../#.#../#..#./#...#",
	'L':  "#..../#..../#..../#..../#..../#..../#####",
	'M':  "#...#/##.##/#.#.#/#...#/#...#/#...#/#...#",
	'N':  "#...#/##..#/#.#.#/#..##/#...#/#...#/#...#",
	'O':  ".###./#...#/#...#/#...#/#...#/#...#/.###.",
	'P':  "####./#...#/#...#/####./#..../#..../#....",
	'Q':  ".###./#...#/#...#/#...#/#.#.#/#..#./.##.#",
	'R':  "####./#...#/#...#/####./#.#../#..#./#...#",
	'S':  ".####/#..../#..../.###./....#/....#/####.",
	'T':  "#####/..#../..#../..#../..#../..#../..#..",
	'U':  "#...#/#...#/#...#/#...#/#...#/#...#/.###.",
	'V':  "#...#/#...#/#...#/#...#/#...#/.#.#./..#..",
	'W':  "#...#/#...#/#...#/#...#/#.#.#/##.##/#...#",
	'X':  "#...#/#...#/.#.#./..#../.#.#./#...#/#...#",
	'Y':  "#...#/#...#/.#.#./..#../..#../..#../..#..",
	'Z':  "#####/....#/...#./..#../.#.../#..../#####",
	'0':  ".###./#...#/#..##/#.#.#/##..#/#...#/.###.",
	'1':  "..#../.##../..#../..#../..#../..#../.###.",
	'2':  ".###./#...#/....#/...#./..#../.#.../#####",
	'3':  "####./....#/....#/.###./....#/....#/####.",
	'4':  "...#./..##./.#.#./#..#./#####/...#./...#.",
	'5':  "#####/#..../####./....#/....#/#...#/.###.",
	'6':  ".###./#..../#..../####./#...#/#...#/.###.",
	'7':  "#####/....#/...#./..#../.#.../.#.../.#...",
	'8':  ".###./#...#/#...#/.###./#...#/#...#/.###.",
	'9':  ".###./#...#/#...#/.####/....#/....#/.###.",
	'.':  "...../...../...../...../...../...../..#..",
	',':  "...../...../...../...../...../..#../.#...",
	':':  "...../..#../..#../...../..#../..#../.....",
	';':  "...../..#../..#../...../..#../..#../.#...",
	'!':  "..#../..#../..#../..#../..#../...../..#..",
	'?':  ".###./#...#/....#/...#./..#../...../..#..",
	'-':  "...../...../...../.###./...../...../.....",
	'+':  "...../..#../..#../.###./..#../..#../.....",
	'/':  "....#/....#/...#./..#../.#.../#..../#....",
	'\'': "..#../..#../...../...../...../...../.....",
	'(':  "...#./..#../.#.../.#.../.#.../..#../...#.",
	')':  ".#.../..#../...#./...#./...#./..#../.#...",
	'"':  ".#.#./.#.#./...../...../...../...../.....",
	'#':  "#.#../#####/#.#../#####/#.#../...../.....",
	'%':  "#...#/...#./..#../.#.../#...#/...../.....",
	'*':  "...../#.#.#/.###./#####/.###./#.#.#/.....",
	'=':  "...../...../.###./...../.###./...../.....",
	'<':  "...#./..#../.#.../#..../.#.../..#../...#.",
	'>':  ".#.../..#../...#./....#/...#./..#../.#...",
	'_':  "...../...../...../...../...../...../#####",
	'&':  ".##../#..#./.##../#.#.#/#..#./#..#./.##.#",
}

// textRows splits the packed form into its seven rows.
func textRows(r rune) ([GlyphHeight]string, bool) {
	packed, ok := rows[r]
	if !ok {
		return [GlyphHeight]string{}, false
	}
	parts := strings.Split(packed, "/")
	if len(parts) != GlyphHeight {
		return [GlyphHeight]string{}, false
	}
	var out [GlyphHeight]string
	copy(out[:], parts)
	for _, row := range parts {
		if len([]rune(row)) != GlyphWidth {
			return [GlyphHeight]string{}, false
		}
	}
	return out, true
}

// Normalise uppercases text and rejects anything the font cannot draw, before
// any pixel is written.
func Normalise(s string) (string, error) {
	if s == "" {
		return "", fmt.Errorf("text is empty")
	}
	upper := strings.ToUpper(s)
	for _, r := range upper {
		if _, ok := rows[r]; !ok {
			return "", fmt.Errorf("no glyph for %q, so %q cannot be drawn; use capitals, digits and . , : ; ! ? - + / ' ( ) \" # %% * = < > _ &", r, s)
		}
	}
	return upper, nil
}

// Width reports the pixels s occupies, including the gap after every glyph, so
// a cursor needs no special cases.
func Width(s string) (int, error) {
	upper, err := Normalise(s)
	if err != nil {
		return 0, err
	}
	return len([]rune(upper)) * Advance, nil
}

// Ink returns the absolute points of s, top-left at (x, y).
//
// Points are sprite-absolute, which is what `draw` wants: place text by its
// top-left corner and get a pixel list back.
func Ink(s string, x, y int) ([][2]int, error) {
	upper, err := Normalise(s)
	if err != nil {
		return nil, err
	}

	points := make([][2]int, 0, len(upper)*GlyphHeight)
	for i, r := range upper {
		glyph, ok := textRows(r)
		if !ok {
			return nil, fmt.Errorf("glyph %q is malformed in the font table", r)
		}
		origin := x + i*Advance
		for row := 0; row < GlyphHeight; row++ {
			for col := 0; col < GlyphWidth; col++ {
				if glyph[row][col] == '#' {
					points = append(points, [2]int{origin + col, y + row})
				}
			}
		}
	}
	return points, nil
}
