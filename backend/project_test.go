package backend

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/d3uceY/spriteware/internal/config"
	"github.com/d3uceY/spriteware/internal/link"
)

func projectService(t *testing.T, projectPath string) *MCPService {
	t.Helper()
	store, err := config.LoadFrom(filepath.Join(t.TempDir(), "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Save(config.Config{Mode: config.ModeHeadless, ProjectPath: projectPath}); err != nil {
		t.Fatal(err)
	}
	links := link.NewSelector(link.Options{Mode: config.ModeHeadless})
	t.Cleanup(func() { _ = links.Close() })
	return NewMCPService(store, links)
}

// "This project" must be a folder the user chose. Inheriting the working
// directory was the bug: from a shortcut that is the install directory.
func TestTheChosenProjectFolderIsUsedForSkills(t *testing.T) {
	chosen := filepath.Join(t.TempDir(), "my-game")
	svc := projectService(t, chosen)

	got, isChosen := svc.ProjectFolder()
	if got != chosen || !isChosen {
		t.Fatalf("ProjectFolder = %q (chosen=%v), want %q", got, isChosen, chosen)
	}

	skill := describeClient("cursor", "http://127.0.0.1:9933/mcp", "streamable-http", svc.projectDir()).Skill
	if !strings.HasPrefix(skill.Project, "~") && !strings.Contains(skill.Project, "my-game") {
		t.Errorf("the skill path does not point into the chosen folder: %q", skill.Project)
	}
}

// The VS Code row writes into .vscode/, so it depends on the project folder too.
func TestTheChosenProjectFolderIsUsedForVSCodeConfig(t *testing.T) {
	chosen := t.TempDir()
	svc := projectService(t, chosen)

	entry := entryFor("vscode", "http://127.0.0.1:9933/mcp", "streamable-http", svc.projectDir())
	want := filepath.Join(chosen, ".vscode", "mcp.json")
	if entry.path != want {
		t.Fatalf("vscode config path = %q, want %q", entry.path, want)
	}
}

func TestClearingTheProjectFolderFallsBackToTheWorkingDirectory(t *testing.T) {
	svc := projectService(t, "")
	if _, chosen := svc.ProjectFolder(); chosen {
		t.Fatal("an empty setting reported a chosen folder")
	}

	if _, err := svc.ClearProjectFolder(); err != nil {
		t.Fatalf("clear: %v", err)
	}
	if svc.cfg.Get().ProjectPath != "" {
		t.Error("clearing did not persist")
	}
}
