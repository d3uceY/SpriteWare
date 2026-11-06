package link

import (
	"context"
	"testing"

	"github.com/d3uceY/spriteware/internal/doc"
)

// call runs one command and decodes the reply into out.
func call(t *testing.T, h *Headless, cmd string, args map[string]any, out any) {
	t.Helper()
	if err := h.Call(context.Background(), cmd, args, out); err != nil {
		t.Fatalf("%s: %v", cmd, err)
	}
}

type layerEntry struct {
	Name    string `json:"name"`
	Depth   int    `json:"depth"`
	IsGroup bool   `json:"isGroup"`
	Visible bool   `json:"visible"`
}

type layerList struct {
	Layers []layerEntry `json:"layers"`
}

// group moves the named layers into one group, and the group holds exactly
// those layers.
func TestIntegrationLayerGroupAndUngroup(t *testing.T) {
	h := realHeadless(t)
	newDoc(t, h, 8, 8)

	for _, name := range []string{"body", "head"} {
		call(t, h, "layer.create", map[string]any{"name": name}, &map[string]any{})
	}

	var grouped map[string]any
	call(t, h, "layer.group", map[string]any{"layers": []any{"body", "head"}, "name": "rig"}, &grouped)

	var listed layerList
	call(t, h, "layer.list", nil, &listed)

	groups := 0
	for _, l := range listed.Layers {
		if l.IsGroup {
			groups++
			if l.Name != "rig" {
				t.Fatalf("the group is named %q", l.Name)
			}
		}
	}
	if groups != 1 {
		t.Fatalf("layer.list reports %d group(s), want 1: %+v", groups, listed.Layers)
	}

	// A grouped child sits one level deeper than the group.
	depth := map[string]int{}
	for _, l := range listed.Layers {
		depth[l.Name] = l.Depth
	}
	if depth["body"] <= depth["rig"] || depth["head"] != depth["body"] {
		t.Fatalf("the children did not move under the group: %+v", listed.Layers)
	}

	var ungrouped map[string]any
	call(t, h, "layer.ungroup", map[string]any{"name": "rig"}, &ungrouped)

	var after layerList
	call(t, h, "layer.list", nil, &after)
	for _, l := range after.Layers {
		if l.IsGroup {
			t.Fatalf("the group survived ungroup: %+v", after.Layers)
		}
	}
	if len(after.Layers) != 3 {
		t.Fatalf("after ungroup there are %d layers, want 3: %+v", len(after.Layers), after.Layers)
	}
}

// merge keeps both sets of pixels and removes the sources.
func TestIntegrationLayerMergeKeepsBothPictures(t *testing.T) {
	h := realHeadless(t)
	newDoc(t, h, 8, 8)

	call(t, h, "layer.create", map[string]any{"name": "red"}, &map[string]any{})
	draw(t, h, map[string]any{
		"kind": "rect", "x": 0, "y": 0, "width": 2, "height": 2, "color": "#ff0000",
		"target": map[string]any{"layer": "red"},
	})
	call(t, h, "layer.create", map[string]any{"name": "blue"}, &map[string]any{})
	draw(t, h, map[string]any{
		"kind": "rect", "x": 4, "y": 4, "width": 2, "height": 2, "color": "#0000ff",
		"target": map[string]any{"layer": "blue"},
	})

	var merged map[string]any
	call(t, h, "layer.merge", map[string]any{
		"layers": []any{"red", "blue"}, "to": "flat",
	}, &merged)

	var listed layerList
	call(t, h, "layer.list", nil, &listed)
	for _, l := range listed.Layers {
		if l.Name == "red" || l.Name == "blue" {
			t.Fatalf("%q survived the merge: %+v", l.Name, listed.Layers)
		}
	}

	d := mustDump(t, h)
	idx := d.IndexOfLayer("flat")
	if idx < 0 {
		t.Fatalf("the merged layer is missing: %v", d.LayerNames())
	}

	if r, _, _, a := pixelOf(t, d, 1, 0, 0); r != 255 || a != 255 {
		t.Errorf("the red block was lost: (%d,%d,%d,%d)", r, a, r, a)
	}
	if _, _, b, a := pixelOf(t, d, 1, 4, 4); b != 255 || a != 255 {
		t.Errorf("the blue block was lost")
	}
}

// snap pulls a pixel onto the nearest palette entry and reports the move.
func TestIntegrationPaletteSnapMovesPixelsOntoThePalette(t *testing.T) {
	h := realHeadless(t)
	newDoc(t, h, 4, 4)

	call(t, h, "palette.set", map[string]any{"colors": []any{"#000000", "#ffffff"}}, &map[string]any{})
	draw(t, h, map[string]any{
		"kind": "rect", "x": 0, "y": 0, "width": 4, "height": 4, "color": "#f0f0f0",
	})

	var snapped map[string]any
	call(t, h, "palette.snap", nil, &snapped)

	if changed, _ := snapped["pixelsChanged"].(float64); changed != 16 {
		t.Fatalf("snap changed %v pixels, want 16: %+v", snapped["pixelsChanged"], snapped)
	}

	d := mustDump(t, h)
	if r, g, b, a := pixelOf(t, d, 1, 1, 1); r != 255 || g != 255 || b != 255 || a != 255 {
		t.Fatalf("after snap the pixel is (%d,%d,%d,%d), want white", r, g, b, a)
	}
}

// A selection clips drawing, and clearing it restores the whole canvas.
func TestIntegrationSelectionClipsDrawing(t *testing.T) {
	h := realHeadless(t)
	newDoc(t, h, 8, 8)

	var selected map[string]any
	call(t, h, "select.set", map[string]any{"x": 2, "y": 2, "width": 2, "height": 2}, &selected)
	if empty, _ := selected["empty"].(bool); empty {
		t.Fatal("select.set reported an empty selection")
	}
	if pixels, _ := selected["pixels"].(float64); pixels != 4 {
		t.Fatalf("the selection covers %v pixels, want 4", selected["pixels"])
	}

	draw(t, h, solid(0, 0, 8, 8, "#ff0000"))

	d := mustDump(t, h)
	if _, _, _, a := pixelOf(t, d, 1, 0, 0); a != 0 {
		t.Error("a pixel outside the selection was drawn")
	}
	if r, _, _, a := pixelOf(t, d, 1, 2, 2); r != 255 || a != 255 {
		t.Error("a pixel inside the selection was not drawn")
	}

	if _, err := decodeSelection(h); err != nil {
		t.Fatal(err)
	}

	call(t, h, "select.none", nil, &map[string]any{})
	draw(t, h, solid(0, 0, 1, 1, "#00ff00"))

	d = mustDump(t, h)
	if _, g, _, a := pixelOf(t, d, 1, 0, 0); g != 255 || a != 255 {
		t.Fatal("clearing the selection did not restore drawing outside it")
	}
}

// decodeSelection proves select.info answers on a fresh document.
func decodeSelection(h *Headless) (map[string]any, error) {
	var out map[string]any
	err := h.Call(context.Background(), "select.info", nil, &out)
	return out, err
}

// ignoreSelection is the escape hatch: a deliberate whole-cel edit while a
// selection is still set.
func TestIntegrationSelectionCanBeIgnored(t *testing.T) {
	h := realHeadless(t)
	newDoc(t, h, 8, 8)

	call(t, h, "select.set", map[string]any{"x": 4, "y": 4, "width": 2, "height": 2}, &map[string]any{})

	if err := h.Call(context.Background(), "draw.apply", map[string]any{
		"ignoreSelection": true,
		"ops": []any{map[string]any{
			"kind": "pixels", "color": "#ff0000", "points": []any{[]any{0, 0}},
		}},
	}, &map[string]any{}); err != nil {
		t.Fatalf("draw: %v", err)
	}

	d := mustDump(t, h)
	if r, _, _, a := pixelOf(t, d, 1, 0, 0); r != 255 || a != 255 {
		t.Fatal("ignoreSelection did not reach the draw, so the pixel outside the selection stayed empty")
	}
}

// A slice is carried in the dump, so an exporter can read it without a second
// round trip.
func TestIntegrationSlicesRoundTripThroughTheDump(t *testing.T) {
	h := realHeadless(t)
	newDoc(t, h, 16, 16)

	call(t, h, "slice.create", map[string]any{
		"name": "panel", "x": 1, "y": 2, "width": 10, "height": 12,
		"center": map[string]any{"x": 4, "y": 5, "width": 4, "height": 6},
	}, &map[string]any{})

	d := mustDump(t, h)
	if len(d.Slices) != 1 {
		t.Fatalf("the dump has %d slice(s), want 1", len(d.Slices))
	}
	s := d.Slices[0]
	if s.Name != "panel" || s.X != 1 || s.Y != 2 || s.Width != 10 || s.Height != 12 {
		t.Fatalf("the slice came back as %+v", s)
	}
	if s.Center == nil || s.Center.X != 4 || s.Center.Width != 4 {
		t.Fatalf("the 9-patch centre was lost: %+v", s.Center)
	}

	// A second slice with the same name is refused, not silently duplicated.
	if err := h.Call(context.Background(), "slice.create", map[string]any{
		"name": "panel", "x": 0, "y": 0, "width": 4, "height": 4,
	}, &map[string]any{}); err == nil {
		t.Fatal("a duplicate slice name was accepted")
	}

	call(t, h, "slice.update", map[string]any{"name": "panel", "width": 8}, &map[string]any{})
	d = mustDump(t, h)
	if d.Slices[0].Width != 8 || d.Slices[0].X != 1 {
		t.Fatalf("slice.update did not keep the untouched fields: %+v", d.Slices[0])
	}

	call(t, h, "slice.delete", map[string]any{"name": "panel"}, &map[string]any{})
	d = mustDump(t, h)
	if len(d.Slices) != 0 {
		t.Fatalf("the slice was not deleted: %+v", d.Slices)
	}
}

// layer.set writes visibility, opacity and blend mode, and the report uses the
// same blend names the setter accepts.
func TestIntegrationLayerSetReportsWhatItWrote(t *testing.T) {
	h := realHeadless(t)
	newDoc(t, h, 4, 4)

	var out map[string]any
	call(t, h, "layer.set", map[string]any{
		"name": "Layer 1", "visible": false, "opacity": 128, "blendMode": "multiply",
	}, &out)

	if out["visible"] != false {
		t.Errorf("visible = %v, want false", out["visible"])
	}
	if opacity, _ := out["opacity"].(float64); opacity != 128 {
		t.Errorf("opacity = %v, want 128", out["opacity"])
	}
	if out["blendMode"] != "multiply" {
		t.Errorf("blendMode = %v, want multiply", out["blendMode"])
	}

	// An HSL mode keeps its short name on the way back out.
	call(t, h, "layer.set", map[string]any{"name": "Layer 1", "blendMode": "luminosity"}, &out)
	if out["blendMode"] != "luminosity" {
		t.Errorf("blendMode = %v, want luminosity", out["blendMode"])
	}

	// And a name that is not a blend mode is refused with the list.
	if err := h.Call(context.Background(), "layer.set", map[string]any{
		"name": "Layer 1", "blendMode": "shiny",
	}, &map[string]any{}); err == nil {
		t.Fatal("an unknown blend mode was accepted")
	}
}

// A slice that names no layer still belongs to the document, and doc.info must
// report it: the UI reads doc.info, not a dump.
func TestIntegrationDocInfoCarriesSlices(t *testing.T) {
	h := realHeadless(t)
	newDoc(t, h, 8, 8)

	call(t, h, "slice.create", map[string]any{
		"name": "icon", "x": 0, "y": 0, "width": 8, "height": 8,
	}, &map[string]any{})

	info := &doc.Dump{}
	call(t, h, "doc.info", nil, info)
	if len(info.Slices) != 1 || info.Slices[0].Name != "icon" {
		t.Fatalf("doc.info reports %+v", info.Slices)
	}
}
