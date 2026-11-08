package backend

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/d3uceY/spriteware/internal/mcpserver"
	"github.com/d3uceY/spriteware/internal/skill"
)

// This file installs the MCP server into the config file of each assistant.
//
// Every agent names the same server differently: four top-level keys, three
// spellings for a remote URL, one TOML file, one JSONC file. The snippet and
// the written object both come from one description, so the card and the
// install can never disagree.

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

// ClientSpec is one assistant: label, config location, copy text and whether
// the install button can do it.
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

// ClientSkill is where this assistant looks for Agent Skills. Available false
// means no skills folder. Notes are per scope because some caveats only apply
// to one: Claude Code's personal skills are skipped by Cowork and cloud.
type ClientSkill struct {
	Available  bool   `json:"available"`
	Project    string `json:"project"`
	Global     string `json:"global"`
	Note       string `json:"note"`
	GlobalNote string `json:"globalNote"`
}

// InstallResult is the outcome of Install. Always a message, never an error: a
// failed install is shown, not thrown.
type InstallResult struct {
	OK      bool   `json:"ok"`
	Message string `json:"message"`
	Path    string `json:"path"`
}

// publicURL is the address an assistant should connect to, running or not:
// showing the address it will use beats showing nothing.
func (s *MCPService) publicURL() string {
	state := s.host.State()
	port := state.Port
	transport := state.Transport
	if port <= 0 {
		cfg := s.cfg.Get()
		port = cfg.Port
		transport = cfg.Transport
	}
	return fmt.Sprintf("http://127.0.0.1:%d%s", port, mcpserver.EndpointPath(transport))
}

// ListClients returns the agents this build knows how to set up.
func (s *MCPService) ListClients() []ClientSpec {
	transport := s.host.State().Transport
	if transport == "" {
		transport = s.cfg.Get().Transport
	}

	url := s.publicURL()
	// Unchosen project is "", not the launch folder: a card should not print a
	// path the user never picked.
	projectDir := s.chosenProjectDir()
	out := make([]ClientSpec, 0, len(clientIDs))
	for _, id := range clientIDs {
		out = append(out, describeClient(id, url, string(transport), projectDir))
	}
	return out
}

// clientIDs fixes the order the tabs appear in: the ones most people have
// first, the ones that need a terminal last.
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

// describeClient builds one spec: the copy text, the resolved path, and whether
// an install is possible here.
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
	if entry.needsProject && projectDir == "" {
		spec.Target.Path = shortPath(entry.path)
		spec.Target.Detected = false
		spec.Install.Can = false
		spec.Install.Why = "Choose the project folder first. This file lives inside the workspace."
		return spec
	}
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
	label        string
	where        string
	note         string
	snippet      string
	format       string
	path         string
	cli          string
	cliHuman     string
	rootKey      string
	serverObj    map[string]any
	needsProject bool
}

// entryFor is the single description of each agent. The snippet and the written
// object both come from here.
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
			format:       formatJSON,
			path:         filepath.Join(projectDir, ".vscode", "mcp.json"),
			rootKey:      "servers",
			serverObj:    map[string]any{"type": transport, "url": url},
			needsProject: true,
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
// did. Anything it cannot do becomes a message with the reason, so the user can
// fall back to the copy button.
func (s *MCPService) InstallClient(id string) InstallResult {
	transport := s.host.State().Transport
	if transport == "" {
		transport = s.cfg.Get().Transport
	}

	entry := entryFor(id, s.publicURL(), string(transport), s.chosenProjectDir())
	if entry.label == id && entry.rootKey == "" {
		return InstallResult{Message: "No setup for " + id + "."}
	}

	if entry.needsProject && s.chosenProjectDir() == "" {
		return InstallResult{
			Message: "Choose the project folder first. This config lives inside the workspace.",
		}
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

// runCLI uses the agent's own command where it has one, because those files
// hold state a hand-written merge should not touch.
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
		// project file other people might commit.
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

// mergeConfig sets one key and writes atomically, keeping a .bak of the first
// version so nothing is lost.
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
		// A fresh file is normal for an unconfigured client.
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

// appendTOML appends one table. Stdlib has no TOML parser and a rewrite would
// drop comments, so append only when the table is absent.
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

// writeAtomic writes beside the target and renames, so a crash leaves no
// half-written config.
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

func zedSettings() string {
	if runtime.GOOS == "windows" {
		return filepath.Join(configDir(), "Zed", "settings.json")
	}
	return filepath.Join(homeDir(), ".config", "zed", "settings.json")
}

// clineSettings finds the VS Code extension's storage. Stable, Insiders and
// portable all resolve through APPDATA.
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

// describeSkill resolves where this agent looks for skills. With no folder
// chosen the path stays relative, so the card says where the skill goes without
// claiming a folder the user never picked.
func describeSkill(id, projectDir string) ClientSkill {
	project := ""
	if projectDir != "" {
		project = skill.ProjectRoot(projectDir)
	}

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

// SkillText is the SKILL.md as it ships, so the copy button has something to
// give when an agent's directory is somewhere this app cannot write.
func (s *MCPService) SkillText() string { return skill.Text() }

// InstallSkill drops the skill folder into one agent's skills directory. A
// skill is just a folder with a SKILL.md, so installing is a copy: nothing of
// the user's to preserve, which is why it overwrites its own folder.
func (s *MCPService) InstallSkill(agent, scope string) InstallResult {
	chosen := skill.ScopeProject
	if scope == "global" {
		chosen = skill.ScopeGlobal
	}

	project := s.chosenProjectDir()
	if chosen == skill.ScopeProject {
		if project == "" {
			return InstallResult{
				Message: "Choose the project folder first. A skill written beside the app is one no agent reads.",
			}
		}
		project = skill.ProjectRoot(project)
	}

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

	// One instruction, not three: the restart hint is what makes the skill appear,
	// so the directory's note usually repeats it (Gemini says "run /skills reload"
	// in both). Kiro has no restart step, so its note is the only advice.
	advice := restartHint(agent)
	if advice == "" {
		advice = at.Note
	}
	message := "Installed in " + shortPath(path) + " for " + where + "."
	if advice != "" {
		message += " " + advice
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
