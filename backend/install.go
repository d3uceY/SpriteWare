package backend

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/d3uceY/spriteware/internal/skill"
)

// This file installs the MCP server into the config file of each assistant.
//
// Every agent wraps the same server differently: four top-level keys, three ways
// to spell a remote URL, one TOML and one JSONC file. The snippet and the written
// object come from one description, so the card cannot show one thing and
// install another.

// ClientTarget says where an agent keeps its config and whether we found it.
type ClientTarget struct {
	Path     string `json:"path"`
	Detected bool   `json:"detected"`
}

// ClientInstall says whether this app can write that config for the user.
type ClientInstall struct {
	Can bool   `json:"can"`
	Why string `json:"why"`
}

// ClientSpec is one assistant: name, config path, the text to copy, and whether
// Install works here.
type ClientSpec struct {
	ID      string        `json:"id"`
	Label   string        `json:"label"`
	Where   string        `json:"where"`
	Note    string        `json:"note"`
	Snippet string        `json:"snippet"`
	Target  ClientTarget  `json:"target"`
	Install ClientInstall `json:"install"`
	Skill   ClientSkill   `json:"skill"`
}

// ClientSkill is where this assistant looks for Agent Skills, at both scopes.
// No directory means Available false and MCP only. Notes are per scope: Claude
// Code's personal skills are skipped by Cowork and cloud sessions.
type ClientSkill struct {
	Available  bool   `json:"available"`
	Project    string `json:"project"`
	Global     string `json:"global"`
	Note       string `json:"note"`
	GlobalNote string `json:"globalNote"`
}

// InstallResult is always a message, never an error: a failed install is shown,
// not thrown.
type InstallResult struct {
	OK      bool   `json:"ok"`
	Message string `json:"message"`
	Path    string `json:"path"`
}

// publicURL is the address an assistant should connect to, running or not:
// showing the address it will use beats showing nothing.
func (s *MCPService) publicURL() string {
	s.mu.Lock()
	port := s.port
	transport := s.transport
	s.mu.Unlock()

	if port <= 0 {
		cfg := s.cfg.Get()
		port = cfg.Port
		transport = cfg.Transport
	}
	return fmt.Sprintf("http://127.0.0.1:%d%s", port, endpointPath(transport))
}

// ListClients returns the agents this build knows how to set up.
func (s *MCPService) ListClients() []ClientSpec {
	s.mu.Lock()
	transport := s.transport
	s.mu.Unlock()
	if transport == "" {
		transport = s.cfg.Get().Transport
	}

	url := s.publicURL()
	out := make([]ClientSpec, 0, len(clientIDs))
	for _, id := range clientIDs {
		out = append(out, describeClient(id, url, string(transport), s.projectDir()))
	}
	return out
}

// clientIDs fixes tab order: the common ones first, terminal ones last.
var clientIDs = []string{
	"claude-desktop",
	"cursor",
	"vscode",
	"claude-code",
	"windsurf",
	"codex",
	"gemini",
	"zed",
	"cline",
	"kiro",
	"opencode",
}

// describeClient builds one spec: copy text, resolved path, install possible.
func describeClient(id, url, transport, projectDir string) ClientSpec {
	spec := ClientSpec{ID: id}
	entry := entryFor(id, url, transport, projectDir)

	spec.Label = entry.label
	spec.Where = entry.where
	spec.Note = entry.note
	spec.Snippet = entry.snippet
	spec.Skill = describeSkill(id, projectDir)

	if entry.cli != "" {
		path, err := exec.LookPath(entry.cli)
		spec.Target.Path = entry.cliHuman
		spec.Target.Detected = err == nil
		if err == nil {
			spec.Install.Can = true
			spec.Install.Why = "Runs `" + filepath.Base(path) + "`."
		} else {
			spec.Install.Why = "`" + entry.cli + "` not on PATH. Copy the snippet."
		}
		return spec
	}

	spec.Target.Path = entry.path
	spec.Target.Detected = exists(entry.path)
	spec.Install.Can = true
	switch entry.format {
	case formatJSONC:
		spec.Install.Why = "Only writes a file with no comments."
	case formatTOML:
		spec.Install.Why = "Appends [mcp_servers.spriteware]."
	default:
		spec.Install.Why = "Adds the entry, keeps the rest."
	}
	if !spec.Target.Detected {
		spec.Install.Why = "Creates " + shortPath(entry.path) + "."
	}
	return spec
}

// ---------------------------------------------------------------------------
// The agents
// ---------------------------------------------------------------------------

const (
	formatJSON  = "json"
	formatJSONC = "jsonc"
	formatTOML  = "toml"
	formatCLI   = "cli"
)

type clientEntry struct {
	label     string
	where     string
	note      string
	snippet   string
	format    string
	path      string
	cli       string
	cliHuman  string
	rootKey   string
	serverObj map[string]any
}

// entryFor is the single description of each agent.
func entryFor(id, url, transport, projectDir string) clientEntry {
	http := transport != "sse"
	// The name those clients use for each transport in their own config.
	if http {
		transport = "http"
	} else {
		transport = "sse"
	}

	remote := map[string]any{"url": url}

	switch id {
	case "claude-desktop":
		return clientEntry{
			label:     "Claude Desktop",
			where:     "claude_desktop_config.json, then quit and reopen Claude.",
			note:      "Settings → Developer → Edit Config.",
			snippet:   prettyJSON(map[string]any{"mcpServers": map[string]any{"spriteware": remote}}),
			format:    formatJSON,
			path:      filepath.Join(configDir(), "Claude", "claude_desktop_config.json"),
			rootKey:   "mcpServers",
			serverObj: remote,
		}

	case "cursor":
		return clientEntry{
			label:     "Cursor",
			where:     "~/.cursor/mcp.json for every project, or .cursor/mcp.json for one.",
			note:      "Project file wins on a conflict.",
			snippet:   prettyJSON(map[string]any{"mcpServers": map[string]any{"spriteware": remote}}),
			format:    formatJSON,
			path:      filepath.Join(homeDir(), ".cursor", "mcp.json"),
			rootKey:   "mcpServers",
			serverObj: remote,
		}

	case "vscode":
		return clientEntry{
			label: "VS Code",
			where: ".vscode/mcp.json in the workspace.",
			note:  "Key is servers, not mcpServers, plus a type field. Needs a trusted folder.",
			snippet: prettyJSON(map[string]any{"servers": map[string]any{
				"spriteware": map[string]any{"type": transport, "url": url},
			}}),
			format:    formatJSON,
			path:      filepath.Join(projectDir, ".vscode", "mcp.json"),
			rootKey:   "servers",
			serverObj: map[string]any{"type": transport, "url": url},
		}

	case "claude-code":
		payload := prettyJSON(map[string]any{"type": transport, "url": url})
		return clientEntry{
			label:    "Claude Code",
			where:    "Run in a terminal.",
			note:     "Writes .mcp.json or ~/.claude.json, leaving its auth state alone.",
			snippet:  "claude mcp add-json spriteware '" + payload + "' --scope user",
			format:   formatCLI,
			cli:      "claude",
			cliHuman: ".mcp.json in the project root, or ~/.claude.json for user scope",
			rootKey:  "mcpServers",
		}

	case "windsurf":
		obj := map[string]any{"serverUrl": url}
		return clientEntry{
			label:     "Windsurf",
			where:     "mcp_config.json.",
			note:      "Address key is serverUrl, not url. Also Settings → Cascade → MCP Servers → Add remote server.",
			snippet:   prettyJSON(map[string]any{"mcpServers": map[string]any{"spriteware": obj}}),
			format:    formatJSON,
			path:      filepath.Join(homeDir(), ".codeium", "windsurf", "mcp_config.json"),
			rootKey:   "mcpServers",
			serverObj: obj,
		}

	case "codex":
		return clientEntry{
			label:   "OpenAI Codex",
			where:   "~/.codex/config.toml.",
			note:    "Table is mcp_servers, with an underscore.",
			snippet: "[mcp_servers.spriteware]\nurl = \"" + url + "\"\n",
			format:  formatTOML,
			path:    filepath.Join(homeDir(), ".codex", "config.toml"),
			rootKey: "mcp_servers",
		}

	case "gemini":
		key := "httpUrl"
		obj := map[string]any{key: url}
		return clientEntry{
			label:     "Gemini CLI",
			where:     "~/.gemini/settings.json.",
			note:      "Merged, not replaced. httpUrl is Streamable HTTP; url the older SSE field.",
			snippet:   prettyJSON(map[string]any{"mcpServers": map[string]any{"spriteware": obj}}),
			format:    formatJSON,
			path:      filepath.Join(homeDir(), ".gemini", "settings.json"),
			rootKey:   "mcpServers",
			serverObj: obj,
		}

	case "zed":
		return clientEntry{
			label:     "Zed",
			where:     "Zed's settings.json.",
			note:      "Reads context_servers, not mcpServers. Install refuses a file with comments.",
			snippet:   prettyJSON(map[string]any{"context_servers": map[string]any{"spriteware": remote}}),
			format:    formatJSONC,
			path:      zedSettings(),
			rootKey:   "context_servers",
			serverObj: remote,
		}

	case "cline":
		obj := map[string]any{"type": clineTransport(http), "url": url}
		return clientEntry{
			label:     "Cline",
			where:     "cline_mcp_settings.json.",
			note:      "type must be streamableHttp or sse, or it assumes stdio.",
			snippet:   prettyJSON(map[string]any{"mcpServers": map[string]any{"spriteware": obj}}),
			format:    formatJSON,
			path:      clineSettings(),
			rootKey:   "mcpServers",
			serverObj: obj,
		}

	case "kiro":
		return clientEntry{
			label:     "Kiro",
			where:     "~/.kiro/settings/mcp.json.",
			note:      "Project can override at .kiro/settings/mcp.json.",
			snippet:   prettyJSON(map[string]any{"mcpServers": map[string]any{"spriteware": remote}}),
			format:    formatJSON,
			path:      filepath.Join(homeDir(), ".kiro", "settings", "mcp.json"),
			rootKey:   "mcpServers",
			serverObj: remote,
		}

	case "opencode":
		obj := map[string]any{"type": "remote", "url": url, "enabled": true}
		return clientEntry{
			label:     "OpenCode",
			where:     "~/.config/opencode/opencode.json, or opencode.json in a project.",
			note:      "Remote server is type remote under the mcp key.",
			snippet:   prettyJSON(map[string]any{"mcp": map[string]any{"spriteware": obj}}),
			format:    formatJSON,
			path:      filepath.Join(homeDir(), ".config", "opencode", "opencode.json"),
			rootKey:   "mcp",
			serverObj: obj,
		}
	}

	return clientEntry{label: id, format: formatJSON, rootKey: "mcpServers", serverObj: remote}
}

func clineTransport(http bool) string {
	if http {
		return "streamableHttp"
	}
	return "sse"
}

// ---------------------------------------------------------------------------
// Installing
// ---------------------------------------------------------------------------

// InstallClient writes the server into one agent's config and reports what it
// did. What it cannot do becomes a message with the reason.
func (s *MCPService) InstallClient(id string) InstallResult {
	s.mu.Lock()
	transport := s.transport
	s.mu.Unlock()
	if transport == "" {
		transport = s.cfg.Get().Transport
	}

	entry := entryFor(id, s.publicURL(), string(transport), s.projectDir())
	if entry.label == id && entry.rootKey == "" {
		return InstallResult{Message: "No setup for " + id + "."}
	}

	if entry.format == formatCLI {
		return runCLI(entry)
	}

	if err := mergeConfig(entry); err != nil {
		return InstallResult{Message: err.Error(), Path: entry.path}
	}
	return InstallResult{
		OK:      true,
		Message: "Added to " + shortPath(entry.path) + ". Restart " + entry.label + ".",
		Path:    entry.path,
	}
}

// runCLI uses the agent's own command, because those files hold state a
// hand-written merge should not touch.
func runCLI(entry clientEntry) InstallResult {
	if _, err := exec.LookPath(entry.cli); err != nil {
		return InstallResult{
			Message: entry.cli + " is not on your PATH. Copy the snippet.",
		}
	}

	var args []string
	switch entry.cli {
	case "claude":
		// The payload carries the transport; --scope user keeps it out of the
		// project file others might commit.
		args = []string{"mcp", "add-json", "spriteware", compactJSON(entry.serverObj), "--scope", "user"}
	case "codex":
		args = []string{"mcp", "add", "spriteware", "--url", stringOf(entry.serverObj["url"])}
	case "gemini":
		args = []string{"mcp", "add", "spriteware", stringOf(entry.serverObj["httpUrl"]), "-s", "user"}
	default:
		return InstallResult{Message: "No install command for " + entry.cli + "."}
	}

	cmd := exec.Command(entry.cli, args...)
	output, err := cmd.CombinedOutput()
	text := strings.TrimSpace(string(output))
	if err != nil {
		if text == "" {
			text = err.Error()
		}
		return InstallResult{Message: "`" + entry.cli + " " + strings.Join(args, " ") + "` failed: " + text}
	}
	if text == "" {
		text = "Added spriteware."
	}
	return InstallResult{OK: true, Message: text}
}

// mergeConfig sets one key and writes the file back atomically, keeping a .bak
// the first time.
func mergeConfig(entry clientEntry) error {
	dir := filepath.Dir(entry.path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("could not create %s: %w", shortPath(dir), err)
	}

	root := map[string]any{}
	raw, err := os.ReadFile(entry.path)
	switch {
	case err == nil:
		if entry.format == formatTOML {
			return appendTOML(entry, raw)
		}
		if len(raw) > 0 {
			if err := json.Unmarshal(raw, &root); err != nil {
				if entry.format == formatJSONC {
					return fmt.Errorf("%s has comments. Won't rewrite it; paste the snippet", shortPath(entry.path))
				}
				return fmt.Errorf("%s is not plain JSON. Left it alone: %v", shortPath(entry.path), err)
			}
		}
		if err := backup(entry.path, raw); err != nil {
			return err
		}
	case os.IsNotExist(err):
		// A fresh file is normal for a client never configured before.
	default:
		return fmt.Errorf("could not read %s: %w", shortPath(entry.path), err)
	}

	section, ok := root[entry.rootKey].(map[string]any)
	if !ok {
		section = map[string]any{}
	}
	section["spriteware"] = entry.serverObj
	root[entry.rootKey] = section

	body, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return fmt.Errorf("could not encode the config: %w", err)
	}
	return writeAtomic(entry.path, append(body, '\n'))
}

// appendTOML adds one table. No TOML parser in the standard library, and a
// rewrite would drop comments, so the entry is only appended when absent.
func appendTOML(entry clientEntry, raw []byte) error {
	header := "[mcp_servers.spriteware]"
	if strings.Contains(string(raw), header) {
		return fmt.Errorf("%s already has a %s table. Edit it by hand.", shortPath(entry.path), header)
	}
	if err := backup(entry.path, raw); err != nil {
		return err
	}

	body := string(raw)
	if body != "" && !strings.HasSuffix(body, "\n") {
		body += "\n"
	}
	if body != "" {
		body += "\n"
	}
	body += entry.snippet
	return writeAtomic(entry.path, []byte(body))
}

// backup keeps one copy of the file as it was before the first write.
func backup(path string, raw []byte) error {
	if len(raw) == 0 {
		return nil
	}
	bak := path + ".bak"
	if _, err := os.Stat(bak); err == nil {
		return nil
	}
	return os.WriteFile(bak, raw, 0o644)
}

// writeAtomic writes beside the target and renames.
func writeAtomic(path string, body []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".spriteware-*.tmp")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)

	if _, err := tmp.Write(body); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(name, path); err != nil {
		return fmt.Errorf("could not replace %s: %w", shortPath(path), err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Paths
// ---------------------------------------------------------------------------

func homeDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "."
	}
	return home
}

// configDir is where desktop apps keep their settings on this platform.
func configDir() string {
	switch runtime.GOOS {
	case "windows":
		if appData := os.Getenv("APPDATA"); appData != "" {
			return appData
		}
	case "darwin":
		return filepath.Join(homeDir(), "Library", "Application Support")
	}
	return filepath.Join(homeDir(), ".config")
}

// workspaceDir is the folder the app was started from.
func workspaceDir() string {
	if wd, err := os.Getwd(); err == nil {
		return wd
	}
	return homeDir()
}

// projectDir is the folder "This project" means.
//
// A stored setting rather than the working directory: a shortcut starts the app
// somewhere unrelated to the user's work, where a skill is never read.
func (s *MCPService) projectDir() string {
	if chosen := strings.TrimSpace(s.cfg.Get().ProjectPath); chosen != "" {
		return chosen
	}
	return workspaceDir()
}

// ProjectFolder reports where "This project" resolves, and whether the user chose
// it.
func (s *MCPService) ProjectFolder() (string, bool) {
	chosen := strings.TrimSpace(s.cfg.Get().ProjectPath)
	if chosen != "" {
		return chosen, true
	}
	return workspaceDir(), false
}

func zedSettings() string {
	if runtime.GOOS == "windows" {
		return filepath.Join(configDir(), "Zed", "settings.json")
	}
	return filepath.Join(homeDir(), ".config", "zed", "settings.json")
}

// clineSettings finds the VS Code extension's storage. The install variant does
// not matter: the folder name comes from APPDATA.
func clineSettings() string {
	if runtime.GOOS == "darwin" {
		return filepath.Join(homeDir(), "Library", "Application Support", "Code", "User",
			"globalStorage", "saoudrizwan.claude-dev", "settings", "cline_mcp_settings.json")
	}
	return filepath.Join(configDir(), "Code", "User", "globalStorage",
		"saoudrizwan.claude-dev", "settings", "cline_mcp_settings.json")
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// shortPath keeps home out of the UI while staying recognisable.
func shortPath(path string) string {
	home := homeDir()
	if strings.HasPrefix(path, home) {
		return "~" + strings.TrimPrefix(path, home)
	}
	return path
}

// ---------------------------------------------------------------------------
// JSON text
// ---------------------------------------------------------------------------

func prettyJSON(v any) string {
	body, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return "{}"
	}
	return string(body)
}

func compactJSON(v any) string {
	body, err := json.Marshal(v)
	if err != nil {
		return "{}"
	}
	return string(body)
}

func stringOf(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

// ---------------------------------------------------------------------------
// The skill
// ---------------------------------------------------------------------------

// describeSkill resolves where this agent looks for skills, at both scopes.
func describeSkill(id, projectDir string) ClientSkill {
	project := skill.ProjectRoot(projectDir)

	at, ok := skill.DirFor(id, skill.ScopeProject, project)
	if !ok {
		return ClientSkill{}
	}

	out := ClientSkill{
		Available: true,
		Project:   shortPath(filepath.Join(at.Dir, skill.Name)),
		Note:      at.Note,
	}
	if global, ok := skill.DirFor(id, skill.ScopeGlobal, project); ok {
		out.Global = shortPath(filepath.Join(global.Dir, skill.Name))
		out.GlobalNote = global.Note
	}
	return out
}

// SkillText is the SKILL.md as it ships, for the copy button.
func (s *MCPService) SkillText() string { return skill.Text() }

// InstallSkill drops the skill folder into one agent's skills directory.
//
// A skill is a folder with a SKILL.md, so installing is a copy: no merge, and
// nothing of the user's to preserve.
func (s *MCPService) InstallSkill(agent, scope string) InstallResult {
	chosen := skill.ScopeProject
	if scope == "global" {
		chosen = skill.ScopeGlobal
	}

	project := skill.ProjectRoot(s.projectDir())
	at, ok := skill.DirFor(agent, chosen, project)
	if !ok {
		return InstallResult{
			Message: "No skills folder for this assistant. Add the server entry instead.",
		}
	}

	path, err := skill.Install(at.Dir)
	if err != nil {
		return InstallResult{Message: err.Error(), Path: path}
	}

	where := "this project"
	if chosen == skill.ScopeGlobal {
		where = "every project"
	}
	message := "Installed in " + shortPath(path) + " for " + where + ". " + restartHint(agent)
	if at.Note != "" {
		message += " " + at.Note
	}
	return InstallResult{OK: true, Message: message, Path: path}
}

// restartHint says the one thing that makes the skill appear, per agent.
func restartHint(agent string) string {
	switch agent {
	case "claude-code":
		return "Restart Claude Code if .claude/skills didn't exist when your session started."
	case "codex":
		return "Run /skills in Codex to confirm it loaded."
	case "gemini":
		return "Run /skills reload in Gemini CLI."
	case "vscode":
		return "Reload Window if it doesn't appear in Chat."
	case "cursor", "windsurf", "cline", "kiro", "opencode":
		return "Restart it."
	default:
		return ""
	}
}
