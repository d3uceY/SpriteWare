//go:build !server

package main

import (
	"bytes"
	"image/png"
	"testing"
)

// The tray icons are embedded bytes, so a missing or malformed PNG would only
// show up as a missing tray icon at runtime. Decode them here instead.
func TestTrayIcons(t *testing.T) {
	for _, tc := range []struct {
		name  string
		data  []byte
		red   uint32
		green uint32
		blue  uint32
	}{
		{"black", trayBlack, 0, 0, 0},
		{"white", trayWhite, 0xffff, 0xffff, 0xffff},
	} {
		img, err := png.Decode(bytes.NewReader(tc.data))
		if err != nil {
			t.Fatalf("%s tray icon is not a PNG: %v", tc.name, err)
		}
		if got := img.Bounds().Size(); got.X != 64 || got.Y != 64 {
			t.Errorf("%s tray icon is %v, want 64x64", tc.name, got)
		}
		r, g, b, a := img.At(8, 20).RGBA()
		if a != 0xffff {
			t.Errorf("%s tray icon center is transparent", tc.name)
		}
		if r != tc.red || g != tc.green || b != tc.blue {
			t.Errorf("%s tray icon center is #%04x%04x%04x, want the named colour", tc.name, r, g, b)
		}
	}
}
