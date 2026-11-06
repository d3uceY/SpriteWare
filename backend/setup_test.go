package backend

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/d3uceY/spriteware/internal/config"
)

// A detected path is a suggestion: nothing is confirmed until the user says so.
func TestSetupStartsUnconfirmed(t *testing.T) {
	svc := newTestService(t)

	state := svc.GetSetupState()
	if state.Confirmed {
		t.Fatal("a fresh install reported a confirmed path")
	}
	if state.Path != "" {
		t.Errorf("path = %q, want empty", state.Path)
	}
}

func TestConfirmRejectsAFolder(t *testing.T) {
	svc := newTestService(t)

	if _, err := svc.ConfirmAsepritePath(t.TempDir()); err == nil {
		t.Fatal("a directory was accepted as the Aseprite program")
	}
	if svc.GetSetupState().Confirmed {
		t.Error("a rejected path still marked setup as done")
	}
}

func TestConfirmRejectsAMissingPath(t *testing.T) {
	svc := newTestService(t)

	if _, err := svc.ConfirmAsepritePath(filepath.Join(t.TempDir(), "nope.exe")); err == nil {
		t.Fatal("a path that does not exist was accepted")
	}
}

func TestConfirmPersistsAndFlipsConfirmed(t *testing.T) {
	svc := newTestService(t)

	// Any real file stands in for the executable: the check is existence, not
	// that it is Aseprite.
	fake := filepath.Join(t.TempDir(), "aseprite.exe")
	if err := os.WriteFile(fake, []byte("not really a program"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	state, err := svc.ConfirmAsepritePath(fake)
	if err != nil {
		t.Fatalf("ConfirmAsepritePath: %v", err)
	}
	if !state.Confirmed || state.Path != fake {
		t.Fatalf("state after confirm = %+v", state)
	}

	// It has to survive a restart, or the question is asked every launch.
	reloaded, err := config.LoadFrom(svc.cfg.FilePath())
	if err != nil {
		t.Fatalf("reload config: %v", err)
	}
	got := reloaded.Get()
	if !got.AsepritePathConfirmed || got.AsepritePath != fake {
		t.Errorf("after reload the config is %+v, want the confirmed path", got)
	}
}

func TestForgetReturnsToSetup(t *testing.T) {
	svc := newTestService(t)

	fake := filepath.Join(t.TempDir(), "aseprite.exe")
	if err := os.WriteFile(fake, []byte("x"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, err := svc.ConfirmAsepritePath(fake); err != nil {
		t.Fatalf("confirm: %v", err)
	}

	state, err := svc.ForgetAsepritePath()
	if err != nil {
		t.Fatalf("ForgetAsepritePath: %v", err)
	}
	if state.Confirmed || state.Path != "" {
		t.Errorf("state after forget = %+v", state)
	}
}

func TestListToolsMatchesTheRegisteredSurface(t *testing.T) {
	svc := newTestService(t)

	tools := svc.ListTools()
	if len(tools) == 0 {
		t.Fatal("the app was told the server has no tools")
	}
	if len(tools) != svc.GetMCPStatus().ToolCount {
		t.Errorf("the Tools screen lists %d but the server registered %d",
			len(tools), svc.GetMCPStatus().ToolCount)
	}
	for _, tool := range tools {
		if tool.Name == "" || tool.Blurb == "" {
			t.Errorf("incomplete entry: %+v", tool)
		}
	}
}
