package main

// The tests in this file guard the link between the git tag and the number the
// binary reports. cmd/stamp handles the package metadata; backend.Version is
// only ever set by the linker, so a build step that forgets to pass the version
// ships a binary that claims 0.1.0 (the fallback in backend/mcp.go) while the
// installer around it is correctly stamped. That is exactly what happened for
// the v0.2.x tags, and nothing failed: the release run was green, the exe
// metadata was right, and only the window's Server screen was wrong.

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// versionArg is the Taskfile variable the platform builds turn into -ldflags.
const versionArg = "VERSION=${{ needs.validate-tag.outputs.version }}"

// TestReleaseWorkflowPassesTheTagToEveryBuild reads the release workflow and
// insists every `wails3 task` build step receives the validated tag. A tagged
// build without it compiles backend.Version's fallback into the binary.
func TestReleaseWorkflowPassesTheTagToEveryBuild(t *testing.T) {
	const path = "../../.github/workflows/release.yml"

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	builds := 0
	for i, line := range strings.Split(string(raw), "\n") {
		step := strings.TrimSpace(line)
		if strings.HasPrefix(step, "#") || !strings.Contains(step, "wails3 task ") {
			continue
		}
		builds++

		if !strings.Contains(step, versionArg) {
			t.Errorf("release.yml:%d does not pass %s:\n\t%s\n\tthe binary would report backend.Version's fallback instead of the tag",
				i+1, versionArg, step)
		}
	}

	// One build per platform. A new platform has to be wired up here as well,
	// so adding a build cannot quietly skip the version.
	if builds != 3 {
		t.Fatalf("release.yml has %d `wails3 task` build steps, want 3 (windows, macOS, linux); add the new one to this test", builds)
	}
}

// TestTaskfilesKeepTheVersionLdflag is the other half of the same link: the
// workflow can only pass a variable the Taskfiles read. A renamed marker breaks
// the release instead of shipping a stale number, which is how cmd/stamp treats
// its own markers.
func TestTaskfilesKeepTheVersionLdflag(t *testing.T) {
	const marker = "{{if .VERSION}} -X github.com/d3uceY/spriteware/backend.Version={{.VERSION}}{{end}}"

	for _, platform := range []string{"windows", "darwin", "linux"} {
		path := filepath.Join("..", "..", "build", platform, "Taskfile.yml")

		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(raw), marker) {
			t.Errorf("%s no longer carries %s, so -ldflags drops the version", path, marker)
		}
	}
}

// TestBackendVersionStaysAVar keeps the ldflag able to do anything at all: the
// linker's -X only rewrites a var, so a const would silently win.
func TestBackendVersionStaysAVar(t *testing.T) {
	raw, err := os.ReadFile("../../backend/mcp.go")
	if err != nil {
		t.Fatal(err)
	}

	if !regexp.MustCompile(`(?m)^var Version = "\d+\.\d+\.\d+"$`).Match(raw) {
		t.Error("backend/mcp.go must keep Version a var with a plain fallback: -ldflags -X cannot rewrite a const")
	}
}
