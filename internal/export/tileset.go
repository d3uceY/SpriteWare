package export

import (
	"encoding/json"
	"fmt"
	"image"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/d3uceY/spriteware/internal/render"
)

// TilesetOptions describes a tile export.
type TilesetOptions struct {
	// Frames are canvas-sized images, one per exported frame.
	Frames []*image.NRGBA
	TileW  int
	TileH  int
	// Path is the destination PNG. The metadata file lands beside it.
	Path  string
	Scale int
	// Format is "json" (default), "tiled" or "godot", which add an import file
	// for that tool.
	Format string
}

// Tileset is the JSON beside a tile sheet: the unique tiles on the image, and
// where each one lands on the canvas of every frame.
//
// Grid numbers are 1-based, and 0 means empty, which is the numbering Tiled
// itself uses with firstgid=1, so one number means the same thing in both files.
type Tileset struct {
	Image      string         `json:"image"`
	TileWidth  int            `json:"tileWidth"`
	TileHeight int            `json:"tileHeight"`
	Columns    int            `json:"columns"`
	TileCount  int            `json:"tileCount"`
	Scale      int            `json:"scale"`
	Frames     []TilesetFrame `json:"frames"`
}

// TilesetFrame is one frame's grid. Row 0 is the top of the canvas.
type TilesetFrame struct {
	Index int     `json:"index"`
	Rows  [][]int `json:"rows"`
}

// PackTileset slices every frame into a tile grid, keeps one copy of each
// distinct tile, and writes the sheet plus its metadata.
//
// Duplicates are the point: a 64x64 map made of 16x16 tiles is 16 cells but
// usually six pictures, and shipping 16 wastes most of the texture budget.
func PackTileset(opts TilesetOptions) ([]string, Tileset, error) {
	if len(opts.Frames) == 0 {
		return nil, Tileset{}, fmt.Errorf("nothing to export: the document has no frames")
	}
	if opts.TileW <= 0 || opts.TileH <= 0 {
		return nil, Tileset{}, fmt.Errorf("a tileset needs a positive tile size; got %dx%d", opts.TileW, opts.TileH)
	}
	scale := opts.Scale
	if scale < 1 {
		scale = 1
	}

	frames := make([]*image.NRGBA, 0, len(opts.Frames))
	for _, f := range opts.Frames {
		frames = append(frames, render.Scale(f, scale))
	}

	tileW, tileH := opts.TileW*scale, opts.TileH*scale
	canvasW, canvasH := frames[0].Bounds().Dx(), frames[0].Bounds().Dy()
	if canvasW%tileW != 0 || canvasH%tileH != 0 {
		return nil, Tileset{}, fmt.Errorf(
			"the canvas is %dx%d, which is not a whole number of %dx%d tiles; resize the canvas or change the tile size",
			canvasW, canvasH, tileW, tileH)
	}
	cols, rows := canvasW/tileW, canvasH/tileH

	// First pass: find the distinct tiles, in the order they appear.
	index := map[string]int{}
	var unique []*image.NRGBA
	all := make([][][]*image.NRGBA, 0, len(frames))
	for _, frame := range frames {
		grid := make([][]*image.NRGBA, 0, rows)
		for ry := 0; ry < rows; ry++ {
			line := make([]*image.NRGBA, 0, cols)
			for rx := 0; rx < cols; rx++ {
				tile := crop(frame, rx*tileW, ry*tileH, tileW, tileH)
				if blank(tile) {
					line = append(line, nil)
					continue
				}
				key := tileKey(tile)
				if _, seen := index[key]; !seen {
					unique = append(unique, tile)
					index[key] = len(unique) // 1-based
				}
				line = append(line, tile)
			}
			grid = append(grid, line)
		}
		all = append(all, grid)
	}

	if len(unique) == 0 {
		return nil, Tileset{}, fmt.Errorf("nothing to export: every tile is empty")
	}

	sheetW := len(unique) * tileW
	sheet := image.NewNRGBA(image.Rect(0, 0, sheetW, tileH))
	for i, tile := range unique {
		blit(sheet, tile, i*tileW, 0)
	}

	ts := Tileset{
		Image:      filepath.Base(opts.Path),
		TileWidth:  tileW,
		TileHeight: tileH,
		Columns:    len(unique),
		TileCount:  len(unique),
		Scale:      scale,
	}

	// Second pass: number the grid now that every tile has an index.
	for fi, grid := range all {
		out := TilesetFrame{Index: fi + 1, Rows: make([][]int, 0, len(grid))}
		for _, line := range grid {
			numbered := make([]int, 0, len(line))
			for _, tile := range line {
				if tile == nil {
					numbered = append(numbered, 0)
					continue
				}
				numbered = append(numbered, index[tileKey(tile)])
			}
			out.Rows = append(out.Rows, numbered)
		}
		ts.Frames = append(ts.Frames, out)
	}

	files := make([]string, 0, 3)
	pngPath, err := PNG(sheet, opts.Path, 1)
	if err != nil {
		return nil, Tileset{}, err
	}
	files = append(files, pngPath)

	jsonPath := jsonPathFor(opts.Path)
	body, err := json.MarshalIndent(ts, "", "  ")
	if err != nil {
		return nil, Tileset{}, fmt.Errorf("encode the tileset: %w", err)
	}
	if err := os.WriteFile(jsonPath, append(body, '\n'), 0o644); err != nil {
		return nil, Tileset{}, fmt.Errorf("write %s: %w", jsonPath, err)
	}
	files = append(files, jsonPath)

	switch opts.Format {
	case "", "json":
	case "tiled":
		extra, err := writeTiled(ts, opts.Path)
		if err != nil {
			return nil, Tileset{}, err
		}
		files = append(files, extra...)
	case "godot":
		path, err := writeGodot(ts, opts.Path)
		if err != nil {
			return nil, Tileset{}, err
		}
		files = append(files, path)
	default:
		return nil, Tileset{}, fmt.Errorf("unknown tileset format %q; use json, tiled or godot", opts.Format)
	}

	return files, ts, nil
}

// crop copies one tile out of a frame.
func crop(src *image.NRGBA, x, y, w, h int) *image.NRGBA {
	out := image.NewNRGBA(image.Rect(0, 0, w, h))
	b := src.Bounds()
	for ty := 0; ty < h; ty++ {
		for tx := 0; tx < w; tx++ {
			sx, sy := x+tx, y+ty
			if sx < b.Min.X || sy < b.Min.Y || sx >= b.Max.X || sy >= b.Max.Y {
				continue
			}
			out.SetNRGBA(tx, ty, src.NRGBAAt(sx, sy))
		}
	}
	return out
}

// blank reports a tile with no ink at all, which becomes grid number 0.
func blank(tile *image.NRGBA) bool {
	b := tile.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			if tile.NRGBAAt(x, y).A != 0 {
				return false
			}
		}
	}
	return true
}

func tileKey(tile *image.NRGBA) string {
	var b strings.Builder
	b.Grow(tile.Bounds().Dx() * tile.Bounds().Dy() * 4)
	bd := tile.Bounds()
	for y := bd.Min.Y; y < bd.Max.Y; y++ {
		for x := bd.Min.X; x < bd.Max.X; x++ {
			c := tile.NRGBAAt(x, y)
			b.WriteByte(c.R)
			b.WriteByte(c.G)
			b.WriteByte(c.B)
			b.WriteByte(c.A)
		}
	}
	return b.String()
}

// writeTiled writes a .tsx tileset plus one .tmx map per frame.
//
// A map is one frame, because that is the shape Tiled works in: an animation is
// a stack of maps a game plays back, not one map with a hidden dimension.
func writeTiled(ts Tileset, pngPath string) ([]string, error) {
	dir := filepath.Dir(pngPath)
	base := strings.TrimSuffix(filepath.Base(pngPath), filepath.Ext(pngPath))

	tsxPath := filepath.Join(dir, base+".tsx")
	tsx := "<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n" +
		"<tileset version=\"1.10\" tiledversion=\"1.10.2\" name=\"" + xmlEscape(base) + "\"" +
		" tilewidth=\"" + strconv.Itoa(ts.TileWidth) + "\" tileheight=\"" + strconv.Itoa(ts.TileHeight) + "\"" +
		" tilecount=\"" + strconv.Itoa(ts.TileCount) + "\" columns=\"" + strconv.Itoa(ts.Columns) + "\">\n" +
		" <image source=\"" + xmlEscape(filepath.Base(pngPath)) + "\" width=\"" +
		strconv.Itoa(ts.Columns*ts.TileWidth) + "\" height=\"" + strconv.Itoa(ts.TileHeight) + "\"/>\n" +
		"</tileset>\n"
	if err := os.WriteFile(tsxPath, []byte(tsx), 0o644); err != nil {
		return nil, fmt.Errorf("write %s: %w", tsxPath, err)
	}

	files := []string{tsxPath}
	for _, frame := range ts.Frames {
		rows, cols := len(frame.Rows), 0
		if rows > 0 {
			cols = len(frame.Rows[0])
		}

		var csv strings.Builder
		for ri, line := range frame.Rows {
			if ri > 0 {
				csv.WriteString(",\n")
			}
			for ci, gid := range line {
				if ci > 0 {
					csv.WriteString(",")
				}
				csv.WriteString(strconv.Itoa(gid))
			}
		}

		name := base
		if len(ts.Frames) > 1 {
			name = fmt.Sprintf("%s-f%d", base, frame.Index)
		}
		tmxPath := filepath.Join(dir, name+".tmx")
		tmx := "<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n" +
			"<map version=\"1.10\" tiledversion=\"1.10.2\" orientation=\"orthogonal\" renderorder=\"right-down\"" +
			" width=\"" + strconv.Itoa(cols) + "\" height=\"" + strconv.Itoa(rows) + "\"" +
			" tilewidth=\"" + strconv.Itoa(ts.TileWidth) + "\" tileheight=\"" + strconv.Itoa(ts.TileHeight) + "\"" +
			" infinite=\"0\" nextlayerid=\"2\" nextobjectid=\"1\">\n" +
			" <tileset firstgid=\"1\" source=\"" + xmlEscape(filepath.Base(tsxPath)) + "\"/>\n" +
			" <layer id=\"1\" name=\"" + xmlEscape(name) + "\" width=\"" + strconv.Itoa(cols) + "\" height=\"" +
			strconv.Itoa(rows) + "\">\n" +
			"  <data encoding=\"csv\">\n" + csv.String() + "\n</data>\n" +
			" </layer>\n" +
			"</map>\n"
		if err := os.WriteFile(tmxPath, []byte(tmx), 0o644); err != nil {
			return nil, fmt.Errorf("write %s: %w", tmxPath, err)
		}
		files = append(files, tmxPath)
	}
	return files, nil
}

// writeGodot writes a Godot 4 TileSet resource that points at the sheet.
//
// Only the used tiles are declared, so Godot does not offer hundreds of empty
// cells the art never had.
func writeGodot(ts Tileset, pngPath string) (string, error) {
	base := strings.TrimSuffix(filepath.Base(pngPath), filepath.Ext(pngPath))
	path := filepath.Join(filepath.Dir(pngPath), base+".tres")

	var b strings.Builder
	b.WriteString("[gd_resource type=\"TileSet\" load_steps=3 format=3]\n\n")
	b.WriteString("[ext_resource type=\"Texture2D\" path=\"res://" + filepath.Base(pngPath) + "\" id=\"1_texture\"]\n\n")
	b.WriteString("[sub_resource type=\"TileSetAtlasSource\" id=\"TileSetAtlasSource_1\"]\n")
	b.WriteString("texture = ExtResource(\"1_texture\")\n")
	for i := 0; i < ts.TileCount; i++ {
		b.WriteString(fmt.Sprintf("%d:0/0 = 0\n", i))
	}
	b.WriteString("\n[resource]\n")
	b.WriteString(fmt.Sprintf("tile_size = Vector2i(%d, %d)\n", ts.TileWidth, ts.TileHeight))
	b.WriteString("sources/0 = SubResource(\"TileSetAtlasSource_1\")\n")

	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		return "", fmt.Errorf("write %s: %w", path, err)
	}
	return path, nil
}

func xmlEscape(s string) string {
	replacer := strings.NewReplacer(
		"&", "&amp;", "<", "&lt;", ">", "&gt;", "\"", "&quot;", "'", "&apos;",
	)
	return replacer.Replace(s)
}
