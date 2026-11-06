package config

import (
	"os"
	"path/filepath"
	"testing"
)

// Notepad and PowerShell write a UTF-8 BOM by default and encoding/json refuses
// one, so a hand-edited config used to be renamed to .bad.json and reset.
func TestAConfigWithABOMIsNotTreatedAsCorrupt(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	body := "\xef\xbb\xbf" + `{"port":9977,"bridgePort":9978,"mode":"live"}`

	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	store, err := LoadFrom(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	cfg := store.Get()
	if cfg.Port != 9977 || cfg.BridgePort != 9978 {
		t.Fatalf("the settings were lost: port=%d bridge=%d", cfg.Port, cfg.BridgePort)
	}
	if cfg.Mode != ModeLive {
		t.Errorf("mode = %q, want live", cfg.Mode)
	}

	// And the file was not moved aside.
	if _, err := os.Stat(path + ".bad.json"); err == nil {
		t.Error("a readable config was quarantined as corrupt")
	}
}

// A genuinely broken file still gets replaced rather than stopping the app.
func TestACorruptConfigIsStillReplaced(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte("{ this is not json"), 0o644); err != nil {
		t.Fatal(err)
	}

	store, err := LoadFrom(path)
	if err != nil {
		t.Fatalf("a corrupt file must not stop the app: %v", err)
	}
	if store.Get().Port != DefaultPort {
		t.Errorf("defaults were not applied: %+v", store.Get())
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(path), "config.bad.json")); err != nil {
		t.Error("the corrupt file was not kept for inspection")
	}
}

// SPRITEWARE_CONFIG lets a portable install or a second copy use its own
// settings.
func TestTheConfigPathCanBeRedirected(t *testing.T) {
	want := filepath.Join(t.TempDir(), "elsewhere.json")
	t.Setenv("SPRITEWARE_CONFIG", want)

	got, err := Path()
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("Path() = %q, want %q", got, want)
	}
}
