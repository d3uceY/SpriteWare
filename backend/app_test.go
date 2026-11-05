package backend

import (
	"strings"
	"testing"
)

// These tests need no window and no Wails runtime, so they run in CI.

func TestInfoReportsTheRuntimeItCompiledWith(t *testing.T) {
	info, err := (&App{}).Info()
	if err != nil {
		t.Fatalf("Info() returned an error: %v", err)
	}
	if info.Version != Version {
		t.Errorf("Version = %q, want %q", info.Version, Version)
	}
	if !strings.HasPrefix(info.Go, "go1.") {
		t.Errorf("Go = %q, want a go1.x version", info.Go)
	}
	if info.Platform == "" {
		t.Error("Platform is empty")
	}
}

func TestEchoRejectsEmptyInput(t *testing.T) {
	if _, err := (&App{}).Echo("   "); err == nil {
		t.Fatal("Echo(\"   \") returned no error")
	}
}

func TestEchoReturnsTheMessage(t *testing.T) {
	got, err := (&App{}).Echo("ping")
	if err != nil {
		t.Fatalf("Echo() returned an error: %v", err)
	}
	if !strings.Contains(got, `"ping"`) {
		t.Errorf("Echo(\"ping\") = %q, want it to quote the input", got)
	}
}
