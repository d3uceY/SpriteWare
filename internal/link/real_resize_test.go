package link

import "testing"

func TestIntegrationDocResizeChangesTheCanvas(t *testing.T) {
	h := realHeadless(t)
	newDoc(t, h, 64, 64)

	var out map[string]any
	call(t, h, "doc.resize", map[string]any{"width": 48, "height": 48}, &out)

	d := mustDump(t, h)
	if d.Sprite.Width != 48 || d.Sprite.Height != 48 {
		t.Fatalf("canvas is %dx%d after resize to 48x48 (reply %v)", d.Sprite.Width, d.Sprite.Height, out)
	}
}
