package export

import (
	"encoding/json"
	"image"
	"image/color"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// tileMap builds a canvas from a grid of colours: "" means transparent. Each
// entry becomes a solid tile of size x size.
func tileMap(size int, rows [][]string) *image.NRGBA {
	h := len(rows)
	w := 0
	for _, r := range rows {
		if len(r) > w {
			w = len(r)
		}
	}
	img := image.NewNRGBA(image.Rect(0, 0, w*size, h*size))

	paint := map[string]color.NRGBA{
		"r": {R: 255, A: 255},
		"g": {G: 255, A: 255},
		"b": {B: 255, A: 255},
		"w": {R: 255, G: 255, B: 255, A: 255},
	}
	for ty, row := range rows {
		for tx, key := range row {
			if key == "" {
				continue
			}
			c, ok := paint[key]
			if !ok {
				panic("unknown tile colour " + key)
			}
			for y := 0; y < size; y++ {
				for x := 0; x < size; x++ {
					img.SetNRGBA(tx*size+x, ty*size+y, c)
				}
			}
		}
	}
	return img
}

func TestTilesetKeepsOneCopyOfEachTile(t *testing.T) {
	// Four cells, but only two pictures: red twice and green once, plus a gap.
	canvas := tileMap(2, [][]string{
		{"r", "g"},
		{"r", ""},
	})
	path := filepath.Join(t.TempDir(), "arena.png")

	files, ts, err := PackTileset(TilesetOptions{Frames: []*image.NRGBA{canvas}, TileW: 2, TileH: 2, Path: path})
	if err != nil {
		t.Fatalf("tileset: %v", err)
	}
	if ts.TileCount != 2 || ts.Columns != 2 {
		t.Fatalf("kept %d tile(s) in %d column(s), want 2 and 2", ts.TileCount, ts.Columns)
	}
	if len(files) != 2 {
		t.Fatalf("wrote %v, want the sheet and its json", files)
	}

	want := [][]int{{1, 2}, {1, 0}}
	got := ts.Frames[0].Rows
	if len(got) != len(want) {
		t.Fatalf("grid has %d rows, want %d", len(got), len(want))
	}
	for y := range want {
		for x := range want[y] {
			if got[y][x] != want[y][x] {
				t.Fatalf("grid = %v, want %v", got, want)
			}
		}
	}

	// The sheet is one row of distinct tiles, so its width is the count times the
	// tile width.
	if ts.Columns*ts.TileWidth != 4 || ts.TileHeight != 2 {
		t.Fatalf("sheet geometry = %+v", ts)
	}

	// And the json must be readable without this package.
	raw, err := os.ReadFile(strings.TrimSuffix(path, ".png") + ".json")
	if err != nil {
		t.Fatal(err)
	}
	var back Tileset
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatalf("the tileset json is not valid: %v", err)
	}
	if back.TileCount != 2 || len(back.Frames) != 1 {
		t.Fatalf("round trip lost data: %+v", back)
	}
}

func TestTilesetRefusesAPartialTile(t *testing.T) {
	canvas := tileMap(2, [][]string{{"r", "g"}})
	_, _, err := PackTileset(TilesetOptions{
		Frames: []*image.NRGBA{canvas}, TileW: 3, TileH: 2,
		Path: filepath.Join(t.TempDir(), "x.png"),
	})
	if err == nil {
		t.Fatal("a 4px wide canvas is not a whole number of 3px tiles")
	}
	if !strings.Contains(err.Error(), "whole number") {
		t.Errorf("the refusal should say what is wrong: %v", err)
	}
}

func TestTilesetRefusesAnEmptyDocument(t *testing.T) {
	canvas := image.NewNRGBA(image.Rect(0, 0, 4, 4))
	_, _, err := PackTileset(TilesetOptions{
		Frames: []*image.NRGBA{canvas}, TileW: 2, TileH: 2,
		Path: filepath.Join(t.TempDir(), "x.png"),
	})
	if err == nil {
		t.Fatal("every tile is blank, so there is nothing to write")
	}
}

func TestTilesetWritesATiledMap(t *testing.T) {
	canvas := tileMap(2, [][]string{{"r", "g"}})
	dir := t.TempDir()
	path := filepath.Join(dir, "arena.png")

	files, _, err := PackTileset(TilesetOptions{
		Frames: []*image.NRGBA{canvas}, TileW: 2, TileH: 2, Path: path, Format: "tiled",
	})
	if err != nil {
		t.Fatalf("tileset: %v", err)
	}
	if len(files) != 4 {
		t.Fatalf("wrote %v, want png, json, tsx and tmx", files)
	}

	tsx, err := os.ReadFile(filepath.Join(dir, "arena.tsx"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`tilewidth="2"`, `tileheight="2"`, `tilecount="2"`, `source="arena.png"`} {
		if !strings.Contains(string(tsx), want) {
			t.Errorf("the tsx is missing %s:\n%s", want, tsx)
		}
	}

	tmx, err := os.ReadFile(filepath.Join(dir, "arena.tmx"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`firstgid="1"`, `source="arena.tsx"`, `width="2"`, "1,2"} {
		if !strings.Contains(string(tmx), want) {
			t.Errorf("the tmx is missing %s:\n%s", want, tmx)
		}
	}
}

func TestTilesetWritesAGodotTileSet(t *testing.T) {
	canvas := tileMap(2, [][]string{{"r", "g"}})
	dir := t.TempDir()

	files, _, err := PackTileset(TilesetOptions{
		Frames: []*image.NRGBA{canvas}, TileW: 2, TileH: 2,
		Path: filepath.Join(dir, "arena.png"), Format: "godot",
	})
	if err != nil {
		t.Fatalf("tileset: %v", err)
	}
	if len(files) != 3 {
		t.Fatalf("wrote %v, want png, json and tres", files)
	}

	body, err := os.ReadFile(filepath.Join(dir, "arena.tres"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`[gd_resource type="TileSet"`,
		`type="TileSetAtlasSource"`,
		`tile_size = Vector2i(2, 2)`,
		`sources/0 = SubResource`,
		"0:0/0 = 0",
		"1:0/0 = 0",
	} {
		if !strings.Contains(string(body), want) {
			t.Errorf("the tres is missing %s:\n%s", want, body)
		}
	}
}

func TestTilesetRefusesAnUnknownFormat(t *testing.T) {
	canvas := tileMap(2, [][]string{{"r"}})
	_, _, err := PackTileset(TilesetOptions{
		Frames: []*image.NRGBA{canvas}, TileW: 2, TileH: 2,
		Path: filepath.Join(t.TempDir(), "x.png"), Format: "unity",
	})
	if err == nil {
		t.Fatal("an unknown format must be refused, not silently ignored")
	}
}

// Two frames share the tile pool: a tile that appears in both is stored once.
func TestTilesetSharesTilesAcrossFrames(t *testing.T) {
	first := tileMap(2, [][]string{{"r", "g"}})
	second := tileMap(2, [][]string{{"g", "r"}})

	_, ts, err := PackTileset(TilesetOptions{
		Frames: []*image.NRGBA{first, second}, TileW: 2, TileH: 2,
		Path: filepath.Join(t.TempDir(), "x.png"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if ts.TileCount != 2 {
		t.Fatalf("kept %d tiles, want 2: the frames use the same two pictures", ts.TileCount)
	}
	if len(ts.Frames) != 2 {
		t.Fatalf("the tileset has %d frames, want 2", len(ts.Frames))
	}
}
