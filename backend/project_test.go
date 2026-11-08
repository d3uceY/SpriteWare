package backend

import (
	"os"
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

	skill := describeClient("cursor", "http://127.0.0.1:9933/mcp", "streamable-http", svc.chosenProjectDir()).Skill
	if !strings.HasPrefix(skill.Project, "~") && !strings.Contains(skill.Project, "my-game") {
		t.Errorf("the skill path does not point into the chosen folder: %q", skill.Project)
	}
}

// The VS Code row writes into .vscode/, so it depends on the project folder too.
func TestTheChosenProjectFolderIsUsedForVSCodeConfig(t *testing.T) {
	chosen := t.TempDir()
	svc := projectService(t, chosen)

	entry := entryFor("vscode", "http://127.0.0.1:9933/mcp", "streamable-http", svc.chosenProjectDir())
	want := filepath.Join(chosen, ".vscode", "mcp.json")
	if entry.path != want {
		t.Fatalf("vscode config path = %q, want %q", entry.path, want)
	}
}

// No folder chosen means no folder, not the working directory: from a shortcut
// that is the install folder, where a skill is never read.
func TestAnUnchosenProjectFolderIsEmptyNotTheWorkingDirectory(t *testing.T) {
	workingDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	svc := projectService(t, "")

	got, chosen := svc.ProjectFolder()
	if got != "" || chosen {
		t.Fatalf("ProjectFolder = %q (chosen=%v), want \"\" and false", got, chosen)
	}
	if got == workingDir {
		t.Fatalf("ProjectFolder returned the working directory %q", workingDir)
	}

	// The card shows a relative path instead of a folder nobody picked.
	cursor := describeClient("cursor", "http://127.0.0.1:9933/mcp", "streamable-http", svc.chosenProjectDir()).Skill
	want := filepath.Join(".cursor", "skills", "spriteware")
	if cursor.Project != want {
		t.Errorf("unchosen skill path = %q, want %q", cursor.Project, want)
	}

	// VS Code's config lives in the workspace, so it cannot be installed yet.
	vscode := describeClient("vscode", "http://127.0.0.1:9933/mcp", "streamable-http", svc.chosenProjectDir())
	if vscode.Install.Can {
		t.Error("VS Code offers an install with no project folder chosen")
	}
	if vscode.Target.Detected {
		t.Error("VS Code reports its config as detected with no project folder chosen")
	}
}

// Clearing the choice must leave nothing behind that looks like a project.
func TestClearingTheProjectFolderLeavesNoFolder(t *testing.T) {
	svc := projectService(t, filepath.Join(t.TempDir(), "my-game"))

	if _, err := svc.ClearProjectFolder(); err != nil {
		t.Fatalf("clear: %v", err)
	}
	if svc.cfg.Get().ProjectPath != "" {
		t.Error("clearing did not persist")
	}
	if got, chosen := svc.ProjectFolder(); got != "" || chosen {
		t.Fatalf("after clearing, ProjectFolder = %q (chosen=%v)", got, chosen)
	}
}

// A project-scope install with no folder chosen must refuse, not write to
// whatever directory the app happens to be running from.
func TestInstallingASkillWithoutAProjectFolderRefuses(t *testing.T) {
	isolate(t)
	svc := projectService(t, "")

	got := svc.InstallSkill("gemini", "project")
	if got.OK {
		t.Fatalf("installed anyway: %q", got.Path)
	}
	if !strings.Contains(got.Message, "project folder") {
		t.Errorf("the refusal does not say what is missing: %q", got.Message)
	}

	// Nothing may have been written: a refused install is a no-op, not a
	// quieter write into the launch folder.
	launch := filepath.Join(mustGetwd(t), ".gemini", "skills", "spriteware")
	if _, err := os.Stat(launch); err == nil {
		t.Errorf("the refused install wrote %s anyway", launch)
	}
	if got.Path != "" {
		t.Errorf("a refused install reported a path: %q", got.Path)
	}
}

func mustGetwd(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	return dir
}

// A result line replaces the card's own caveat, so it must not repeat it:
// Gemini's restart hint and its note both said "run /skills reload".
func TestInstallingASkillSaysOneInstruction(t *testing.T) {
	isolate(t)
	project := t.TempDir()
	svc := projectService(t, project)

	got := svc.InstallSkill("gemini", "project")
	if !got.OK {
		t.Fatalf("install failed: %s", got.Message)
	}
	if n := strings.Count(got.Message, "/skills reload"); n != 1 {
		t.Errorf("the message says /skills reload %d times: %s", n, got.Message)
	}

	// The copy has to be on disk, not just reported.
	skillFile := filepath.Join(project, ".gemini", "skills", "spriteware", "SKILL.md")
	if _, err := os.Stat(skillFile); err != nil {
		t.Fatalf("the skill was not written: %v", err)
	}
}
