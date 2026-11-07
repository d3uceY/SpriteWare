package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadWritesDefaultsOnFirstRun(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	store, err := LoadFrom(path)
	if err != nil {
		t.Fatalf("LoadFrom: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("no config file was written: %v", err)
	}
	got := store.Get()
	want := Default()
	if got.Mode != want.Mode || got.Port != want.Port || got.Transport != want.Transport {
		t.Errorf("defaults on disk = %+v, want %+v", got, want)
	}
}

// A hand-edited file must never break the runtime: everything invalid falls
// back.
func TestNormalizeFillsInEverything(t *testing.T) {
	cases := []Config{
		{},                             // empty file
		{Mode: "nonsense"},             // bad enum
		{Port: 0},                      // unset
		{Port: 70000},                  // out of range
		{Transport: "carrier-pigeon"},  // bad enum
		{Port: 5000, BridgePort: 5000}, // the two listeners would collide
	}
	for _, in := range cases {
		out := in.Normalize()
		if out.Mode != ModeHeadless && out.Mode != ModeLive {
			t.Errorf("mode %q was not normalised", out.Mode)
		}
		if out.Port < 1 || out.Port > 65535 {
			t.Errorf("port %d is not usable", out.Port)
		}
		if out.BridgePort < 1 || out.BridgePort > 65535 {
			t.Errorf("bridge port %d is not usable", out.BridgePort)
		}
		if out.BridgePort == out.Port {
			t.Errorf("mcp port and bridge port are both %d", out.Port)
		}
		if out.Transport != TransportSSE && out.Transport != TransportStreamableHTTP {
			t.Errorf("transport %q was not normalised", out.Transport)
		}
	}
}

// A corrupt file is moved aside rather than stopping the app from starting.
func TestCorruptConfigIsSetAside(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte("{ this is not json "), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	store, err := LoadFrom(path)
	if err != nil {
		t.Fatalf("LoadFrom returned an error for corrupt JSON: %v", err)
	}
	if store.Get().Port != Default().Port {
		t.Error("defaults were not applied after a corrupt file")
	}
	if _, statErr := os.Stat(filepath.Join(dir, "config.bad.json")); statErr != nil {
		t.Error("the corrupt file was not kept as config.bad.json")
	}
	// The replacement must be valid, or the next launch repeats the problem.
	if _, reloadErr := LoadFrom(path); reloadErr != nil {
		t.Errorf("the rewritten config is not readable: %v", reloadErr)
	}
}

func TestSaveRoundTrips(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "config.json")
	store, err := LoadFrom(path)
	if err != nil {
		t.Fatalf("LoadFrom: %v", err)
	}
	if _, err := store.Save(Config{
		AsepritePath: "C:/tools/aseprite.exe",
		Mode:         ModeLive,
		Port:         7331,
		Transport:    TransportSSE,
		AllowLua:     true,
	}); err != nil {
		t.Fatalf("Save: %v", err)
	}

	reloaded, err := LoadFrom(path)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	got := reloaded.Get()
	if got.Mode != ModeLive || got.Port != 7331 || got.Transport != TransportSSE || !got.AllowLua {
		t.Errorf("settings did not round-trip: %+v", got)
	}
	if got.AsepritePath != "C:/tools/aseprite.exe" {
		t.Errorf("aseprite path = %q", got.AsepritePath)
	}
}

func TestFindAsepriteRejectsAFolder(t *testing.T) {
	dir := t.TempDir()
	if _, err := FindAseprite(dir); err == nil {
		t.Error("a directory was accepted as the Aseprite executable")
	}
}

// A corrupt file is moved aside *and replaced*, so the next reader finds a
// usable config rather than nothing at all.
func TestLoadReplacesACorruptFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte("{ not json"), 0o644); err != nil {
		t.Fatal(err)
	}

	store, err := LoadFrom(path)
	if err != nil {
		t.Fatalf("LoadFrom: %v", err)
	}
	if got := store.Get().Port; got != DefaultPort {
		t.Errorf("port = %d, want the default %d", got, DefaultPort)
	}
	if _, err := os.Stat(filepath.Join(dir, "config.bad.json")); err != nil {
		t.Errorf("the corrupt file was not kept: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("no replacement config was written: %v", err)
	}
}
