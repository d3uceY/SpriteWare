// Package skill carries the Agent Skill folder that teaches an assistant how to
// drive SpriteWare, and knows where each assistant looks for skills.
//
// A skill is a folder holding a SKILL.md, so installing is a copy, not a merge:
// no JSON, no TOML, nothing to corrupt.
package skill

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
)

// Name is the skill's folder name and the `name` in its frontmatter. Agent
// Skills require the two to match, in strict kebab-case.
const Name = "spriteware"

// The whole tree is embedded, not just SKILL.md: every workflow links to a
// reference chapter, and a dead link is a dead end discovered halfway through a
// job.
//

//go:embed SKILL.md new draw shade palette rig animate tileset fix review export references rules
var tree embed.FS

// Text is the entry document exactly as it ships, for the copy button.
func Text() string {
	body, _ := tree.ReadFile("SKILL.md")
	return string(body)
}

// Files lists every file in the skill, relative to the skill folder, in sorted
// order so an install and a resource listing always agree.
func Files() []string {
	out := make([]string, 0, 20)
	_ = fs.WalkDir(tree, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		out = append(out, p)
		return nil
	})
	sort.Strings(out)
	return out
}

// Read returns one file's body.
func Read(rel string) (string, error) {
	body, err := tree.ReadFile(path.Clean(rel))
	if err != nil {
		return "", fmt.Errorf("the skill has no file %q", rel)
	}
	return string(body), nil
}

// Has reports whether a path is in the skill. The link test uses it to catch a
// typo in a markdown link.
func Has(rel string) bool {
	_, err := tree.ReadFile(path.Clean(rel))
	return err == nil
}

// Scope is where a skill goes: the project you are working in, or your home
// directory so every project sees it.
type Scope string

const (
	ScopeProject Scope = "project"
	ScopeGlobal  Scope = "global"
)

// Target is one assistant's skills directory.
type Target struct {
	// Agent is the id used by the connect card, so both cards agree.
	Agent string
	// Label is what to call it in the UI.
	Label string
	// Dir is the skills directory itself, not the skill's folder inside it.
	Dir string
	// Note explains anything the user has to do by hand.
	Note string
}

// DirFor returns the skills directory for one agent at one scope.
//
// Per-agent rather than "write everywhere": .agents/skills is read by Codex,
// Cursor, Gemini, Copilot, Windsurf and OpenCode, but each also has a native
// folder, and writing that is what survives a change of fallbacks.
func DirFor(agent string, scope Scope, projectRoot string) (Target, bool) {
	home, _ := os.UserHomeDir()
	base := projectRoot
	if scope == ScopeGlobal {
		base = home
	}

	switch agent {
	case "claude-code":
		// Personal skills are not loaded by Cowork or cloud sessions, so the
		// caveat belongs to the global scope only.
		note := ""
		if scope == ScopeGlobal {
			note = "Personal skills don't load in Cowork or cloud sessions."
		}
		return Target{Agent: agent, Label: "Claude Code",
			Dir: filepath.Join(base, ".claude", "skills"), Note: note}, true

	case "codex":
		return Target{Agent: agent, Label: "OpenAI Codex",
			Dir:  filepath.Join(base, ".agents", "skills"),
			Note: "Codex reads .agents/skills, not .codex/skills."}, true

	case "cursor":
		return Target{Agent: agent, Label: "Cursor",
			Dir:  filepath.Join(base, ".cursor", "skills"),
			Note: "Cursor also reads .agents/skills."}, true

	case "gemini":
		return Target{Agent: agent, Label: "Gemini CLI",
			Dir:  filepath.Join(base, ".gemini", "skills"),
			Note: "Run /skills reload after installing."}, true

	case "vscode":
		dir := filepath.Join(base, ".agents", "skills")
		if scope == ScopeGlobal {
			dir = filepath.Join(base, ".copilot", "skills")
		}
		return Target{Agent: agent, Label: "VS Code",
			Dir:  dir,
			Note: "VS Code reads .agents/skills, .github/skills and .claude/skills."}, true

	case "opencode":
		dir := filepath.Join(base, ".opencode", "skills")
		if scope == ScopeGlobal {
			dir = filepath.Join(base, ".config", "opencode", "skills")
		}
		return Target{Agent: agent, Label: "OpenCode", Dir: dir}, true

	case "windsurf":
		dir := filepath.Join(base, ".windsurf", "skills")
		if scope == ScopeGlobal {
			dir = filepath.Join(base, ".codeium", "windsurf", "skills")
		}
		return Target{Agent: agent, Label: "Windsurf", Dir: dir}, true

	case "cline":
		return Target{Agent: agent, Label: "Cline",
			Dir:  filepath.Join(base, ".cline", "skills"),
			Note: "Experimental: turn on Settings → Features → Enable Skills. A global copy wins over a project one."}, true

	case "kiro":
		dir := filepath.Join(base, ".kiro", "skills")
		return Target{Agent: agent, Label: "Kiro", Dir: dir,
			Note: "Custom agents only: add skill://.kiro/skills/*/SKILL.md to the agent's resources."}, true
	}

	// Claude Desktop takes MCP servers but has no skills directory.
	return Target{}, false
}

// Install writes the whole skill folder into dir and returns that folder.
//
// It overwrites its own folder: the skill ships with the app, so an old copy is
// stale, and nothing of the user's is inside.
func Install(dir string) (string, error) {
	root := filepath.Join(dir, Name)
	if err := os.MkdirAll(root, 0o755); err != nil {
		return "", fmt.Errorf("could not create %s: %w", root, err)
	}

	for _, rel := range Files() {
		body, err := tree.ReadFile(rel)
		if err != nil {
			return "", fmt.Errorf("read %s from the embedded skill: %w", rel, err)
		}
		target := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return "", fmt.Errorf("could not create %s: %w", filepath.Dir(target), err)
		}
		if err := os.WriteFile(target, body, 0o644); err != nil {
			return "", fmt.Errorf("could not write %s: %w", target, err)
		}
	}
	return root, nil
}

// ProjectRoot walks up from dir looking for a repository root, because Claude
// Code and Codex both load project skills from the root down and would not see
// a folder buried in a subdirectory.
func ProjectRoot(dir string) string {
	current, err := filepath.Abs(dir)
	if err != nil {
		return dir
	}
	for {
		if info, err := os.Stat(filepath.Join(current, ".git")); err == nil &&
			(info.IsDir() || info.Mode().IsRegular()) {
			return current
		}
		parent := filepath.Dir(current)
		if parent == current {
			return dir
		}
		current = parent
	}
}
