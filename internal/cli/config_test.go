package cli

import (
	"io"
	"path/filepath"
	"testing"

	"github.com/d3uceY/spriteware/internal/config"
)

// run executes one command line the way main.go does, with output discarded.
func run(t *testing.T, args ...string) error {
	t.Helper()
	root := newRoot("test")
	root.SetArgs(args)
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	return root.Execute()
}

func TestConfigSetRejectsOutOfRangePort(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	t.Setenv("SPRITEWARE_CONFIG", path)

	if err := run(t, "config", "set", "port", "70000"); err == nil {
		t.Fatal("a port outside 1..65535 must be refused")
	}

	// The rejected value must not have been written: the file still has the
	// default that Load created.
	store, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if got := store.Get().Port; got != config.DefaultPort {
		t.Fatalf("port = %d, want the default %d", got, config.DefaultPort)
	}
}

func TestConfigSetPersistsAValidValue(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	t.Setenv("SPRITEWARE_CONFIG", path)

	if err := run(t, "config", "set", "port", "12000"); err != nil {
		t.Fatalf("set: %v", err)
	}

	store, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if got := store.Get().Port; got != 12000 {
		t.Fatalf("port = %d, want 12000", got)
	}
}

func TestConfigSetRejectsAnUnknownKey(t *testing.T) {
	t.Setenv("SPRITEWARE_CONFIG", filepath.Join(t.TempDir(), "config.json"))
	if err := run(t, "config", "set", "prot", "12000"); err == nil {
		t.Fatal("an unknown key must be refused, not silently ignored")
	}
}
