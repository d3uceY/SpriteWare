package skill

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"unicode/utf8"
)

// An install that lands where the agent does not scan fails silently, so each
// row is pinned to the vendor's documented location at both scopes.
func TestDirForMatchesTheDocumentedPaths(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("no home directory: %v", err)
	}
	root := filepath.Join(home, "work", "my-project")

	cases := []struct {
		agent   string
		project []string
		global  []string
	}{
		{"claude-code", []string{".claude", "skills"}, []string{".claude", "skills"}},
		{"codex", []string{".agents", "skills"}, []string{".agents", "skills"}},
		{"cursor", []string{".cursor", "skills"}, []string{".cursor", "skills"}},
		{"gemini", []string{".gemini", "skills"}, []string{".gemini", "skills"}},
		{"vscode", []string{".agents", "skills"}, []string{".copilot", "skills"}},
		{"opencode", []string{".opencode", "skills"}, []string{".config", "opencode", "skills"}},
		{"windsurf", []string{".windsurf", "skills"}, []string{".codeium", "windsurf", "skills"}},
		{"cline", []string{".cline", "skills"}, []string{".cline", "skills"}},
		{"kiro", []string{".kiro", "skills"}, []string{".kiro", "skills"}},
	}

	for _, c := range cases {
		project, ok := DirFor(c.agent, ScopeProject, root)
		if !ok {
			t.Fatalf("%s has no project scope", c.agent)
		}
		if want := filepath.Join(append([]string{root}, c.project...)...); project.Dir != want {
			t.Errorf("%s project = %s, want %s", c.agent, project.Dir, want)
		}

		global, ok := DirFor(c.agent, ScopeGlobal, root)
		if !ok {
			t.Fatalf("%s has no global scope", c.agent)
		}
		if want := filepath.Join(append([]string{home}, c.global...)...); global.Dir != want {
			t.Errorf("%s global = %s, want %s", c.agent, global.Dir, want)
		}

		if project.Label == "" || global.Label != project.Label {
			t.Errorf("%s label changes with scope: %q and %q", c.agent, project.Label, global.Label)
		}
		if project.Agent != c.agent || global.Agent != c.agent {
			t.Errorf("%s reports a different agent id", c.agent)
		}
	}
}

// An agent with no skills directory must say so, or the card offers an install
// that cannot work.
func TestAgentsWithoutASkillsFolderAreReported(t *testing.T) {
	for _, agent := range []string{"claude-desktop", "zed", "not-an-agent"} {
		for _, scope := range []Scope{ScopeProject, ScopeGlobal} {
			if at, ok := DirFor(agent, scope, t.TempDir()); ok {
				t.Errorf("%s/%s resolved to %s, want no target", agent, scope, at.Dir)
			}
		}
	}
}

// The notes are the only place a silent failure is warned about, so the two
// that block loading must keep warning.
func TestNotesThatBlockAnInstallAreKept(t *testing.T) {
	cline, _ := DirFor("cline", ScopeProject, t.TempDir())
	if !strings.Contains(cline.Note, "Enable Skills") {
		t.Errorf("Cline needs its experimental toggle mentioned: %q", cline.Note)
	}

	kiro, _ := DirFor("kiro", ScopeProject, t.TempDir())
	if !strings.Contains(kiro.Note, "Custom agents only") {
		t.Errorf("Kiro's note should not imply the default agent is broken: %q", kiro.Note)
	}
	if strings.Contains(kiro.Note, "**") {
		t.Errorf("Kiro's own example globs with one star: %q", kiro.Note)
	}

	// Cowork and cloud sessions skip personal skills: global scope only.
	project, _ := DirFor("claude-code", ScopeProject, t.TempDir())
	global, _ := DirFor("claude-code", ScopeGlobal, t.TempDir())
	if project.Note != "" {
		t.Errorf("project scope has no caveat: %q", project.Note)
	}
	if !strings.Contains(global.Note, "Cowork") {
		t.Errorf("global scope should warn about Cowork: %q", global.Note)
	}
}

// Descriptions are capped by the readers: Cline allows 1024 characters, OpenCode
// 1-1024. A longer one is rejected at load.
func TestDescriptionFitsTheTightestLimit(t *testing.T) {
	name, description, err := frontmatter(Text())
	if err != nil {
		t.Fatal(err)
	}

	if name != Name {
		t.Errorf("frontmatter name = %q, the folder is %q", name, Name)
	}
	if !regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`).MatchString(name) {
		t.Errorf("%q is not strict kebab-case", name)
	}

	if n := utf8.RuneCountInString(description); n < 1 || n > 1024 {
		t.Errorf("description is %d characters, the limit is 1-1024", n)
	}
}

// Installing writes the whole folder: a link to a file that was not copied is a
// dead end.
func TestInstallWritesTheWholeSkillFolder(t *testing.T) {
	dir := t.TempDir()

	root, err := Install(dir)
	if err != nil {
		t.Fatalf("install: %v", err)
	}
	if want := filepath.Join(dir, Name); root != want {
		t.Fatalf("wrote %s, want %s", root, want)
	}

	files := Files()
	if len(files) < 10 {
		t.Fatalf("the skill holds %d files; the workflows and references are missing", len(files))
	}

	for _, rel := range files {
		onDisk := filepath.Join(root, filepath.FromSlash(rel))
		body, err := os.ReadFile(onDisk)
		if err != nil {
			t.Fatalf("%s was not installed: %v", rel, err)
		}
		embedded, err := tree.ReadFile(rel)
		if err != nil {
			t.Fatal(err)
		}
		if string(body) != string(embedded) {
			t.Errorf("%s on disk differs from the embedded copy", rel)
		}
	}

	// A second install replaces the folder's contents instead of nesting a copy.
	if _, err := Install(dir); err != nil {
		t.Fatalf("second install: %v", err)
	}
	if got := countFiles(t, root); got != len(files) {
		t.Errorf("after two installs the folder holds %d files, want %d", got, len(files))
	}
}

func countFiles(t *testing.T, root string) int {
	t.Helper()
	n := 0
	err := filepath.WalkDir(root, func(_ string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			n++
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return n
}

// Every markdown link must land on a shipped file, or only the agent following
// it into nothing discovers the typo.
func TestEverySkillLinkResolves(t *testing.T) {
	link := regexp.MustCompile(`\]\((\.{1,2}/[^)#\s]+)\)`)

	for _, rel := range Files() {
		if !strings.HasSuffix(rel, ".md") {
			continue
		}
		body, err := tree.ReadFile(rel)
		if err != nil {
			t.Fatal(err)
		}

		for _, match := range link.FindAllStringSubmatch(string(body), -1) {
			target := path.Join(path.Dir(rel), match[1])
			if !Has(target) {
				t.Errorf("%s links to %s, which is not in the skill", rel, match[1])
			}
		}
	}
}

// Each workflow's frontmatter name equals its folder, as every Agent Skills host
// requires.
func TestWorkflowFrontmatterMatchesItsFolder(t *testing.T) {
	for _, rel := range Files() {
		if path.Base(rel) != "SKILL.md" || strings.HasPrefix(rel, "references/") {
			continue
		}

		folder := path.Base(path.Dir(rel))
		if rel == "SKILL.md" {
			// The entry document sits at the root, inside a folder named after the
			// skill.
			folder = Name
		}
		body, err := tree.ReadFile(rel)
		if err != nil {
			t.Fatal(err)
		}

		name, description, err := frontmatter(string(body))
		if err != nil {
			t.Errorf("%s: %v", rel, err)
			continue
		}
		if name != folder {
			t.Errorf("%s has name %q, the folder is %q", rel, name, folder)
		}
		if n := utf8.RuneCountInString(description); n < 1 || n > 1024 {
			t.Errorf("%s description is %d characters, the limit is 1-1024", rel, n)
		}
	}
}

// frontmatter pulls the two fields every agent requires out of the block.
//
// Line endings are normalised first: the file is LF in git and CRLF on Windows,
// so the parse must not depend on the working copy.
func frontmatter(raw string) (string, string, error) {
	body := strings.ReplaceAll(raw, "\r\n", "\n")
	const fence = "---\n"
	if !strings.HasPrefix(body, fence) {
		return "", "", fmt.Errorf("SKILL.md does not start with frontmatter")
	}
	rest := body[len(fence):]
	end := strings.Index(rest, "\n---")
	if end < 0 {
		return "", "", fmt.Errorf("frontmatter is never closed")
	}

	var name, description string
	for _, line := range strings.Split(rest[:end], "\n") {
		switch {
		case strings.HasPrefix(line, "name:"):
			name = strings.TrimSpace(strings.TrimPrefix(line, "name:"))
		case strings.HasPrefix(line, "description:"):
			description = strings.TrimSpace(strings.TrimPrefix(line, "description:"))
		}
	}
	if name == "" || description == "" {
		return "", "", fmt.Errorf("frontmatter needs both a name and a description")
	}
	return name, description, nil
}
