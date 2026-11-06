package backend

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// isolate points every config path at a fresh temp home.
func isolate(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("APPDATA", dir)
	t.Setenv("USERPROFILE", dir)
	t.Setenv("HOME", dir)
	return dir
}

func TestSnippetsUseEachAgentsOwnShape(t *testing.T) {
	url := "http://127.0.0.1:9933/mcp"

	cases := []struct {
		id   string
		want []string
	}{
		{"claude-desktop", []string{`"mcpServers"`, `"url": "` + url}},
		{"cursor", []string{`"mcpServers"`, `"url": "` + url}},
		{"vscode", []string{`"servers"`, `"type": "http"`}},
		{"windsurf", []string{`"serverUrl"`}},
		{"cline", []string{`"type": "streamableHttp"`}},
		{"kiro", []string{`"mcpServers"`, `"url": "` + url}},
		{"gemini", []string{`"httpUrl"`}},
		{"zed", []string{`"context_servers"`}},
		{"opencode", []string{`"type": "remote"`, `"mcp"`}},
		{"codex", []string{"[mcp_servers.spriteware]", `url = "` + url + `"`}},
		{"claude-code", []string{"claude mcp add-json spriteware", "--scope user"}},
	}

	for _, c := range cases {
		got := entryFor(c.id, url, "streamable-http", t.TempDir()).snippet
		for _, want := range c.want {
			if !strings.Contains(got, want) {
				t.Errorf("%s snippet is missing %q\ngot: %s", c.id, want, got)
			}
		}
	}
}

func TestSSEChangesTheFieldEachAgentExpects(t *testing.T) {
	url := "http://127.0.0.1:9933/sse"

	if got := entryFor("vscode", url, "sse", t.TempDir()).snippet; !strings.Contains(got, `"type": "sse"`) {
		t.Errorf("VS Code should say sse: %s", got)
	}
	if got := entryFor("cline", url, "sse", t.TempDir()).snippet; !strings.Contains(got, `"type": "sse"`) {
		t.Errorf("Cline should say sse: %s", got)
	}
	// Gemini has one field for Streamable HTTP and another for SSE.
	if got := entryFor("gemini", url, "sse", t.TempDir()).snippet; !strings.Contains(got, `"httpUrl"`) {
		t.Errorf("Gemini keeps httpUrl as the server key: %s", got)
	}
}

func TestMergeCreatesTheFileAndKeepsTheRest(t *testing.T) {
	isolate(t)
	path := filepath.Join(homeDir(), ".kiro", "settings", "mcp.json")

	// A file that already has another server and an unrelated setting.
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	body := `{"mcpServers":{"other":{"url":"http://localhost:1/mcp"}},"theme":"dark"}`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := mergeConfig(entryFor("kiro", "http://127.0.0.1:9933/mcp", "streamable-http", t.TempDir())); err != nil {
		t.Fatalf("merge: %v", err)
	}

	var root map[string]any
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &root); err != nil {
		t.Fatalf("output is not JSON: %v", err)
	}

	servers, _ := root["mcpServers"].(map[string]any)
	if _, ok := servers["other"]; !ok {
		t.Error("the existing server was dropped")
	}
	if _, ok := servers["spriteware"]; !ok {
		t.Error("spriteware was not added")
	}
	if root["theme"] != "dark" {
		t.Error("an unrelated setting was dropped")
	}

	if _, err := os.Stat(path + ".bak"); err != nil {
		t.Error("no backup was kept")
	}
}

func TestMergeRefusesAFileWithComments(t *testing.T) {
	isolate(t)
	path := zedSettings()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("// my settings\n{\n  \"theme\": \"One Dark\"\n}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	err := mergeConfig(entryFor("zed", "http://127.0.0.1:9933/mcp", "streamable-http", t.TempDir()))
	if err == nil {
		t.Fatal("expected a refusal")
	}
	if !strings.Contains(err.Error(), "comments") {
		t.Errorf("refusal should say why: %v", err)
	}

	raw, _ := os.ReadFile(path)
	if !strings.Contains(string(raw), "// my settings") {
		t.Error("the file was modified")
	}
}

func TestTOMLIsAppendedNotRewritten(t *testing.T) {
	isolate(t)
	path := filepath.Join(homeDir(), ".codex", "config.toml")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	original := "model = \"gpt-5-codex\" # my favourite\n"
	if err := os.WriteFile(path, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := mergeConfig(entryFor("codex", "http://127.0.0.1:9933/mcp", "streamable-http", t.TempDir())); err != nil {
		t.Fatalf("append: %v", err)
	}

	raw, _ := os.ReadFile(path)
	text := string(raw)
	if !strings.Contains(text, original[:len(original)-1]) {
		t.Error("the existing config lost its comment")
	}
	if !strings.Contains(text, "[mcp_servers.spriteware]") {
		t.Error("the table was not appended")
	}

	// A second install must not duplicate the table.
	err := mergeConfig(entryFor("codex", "http://127.0.0.1:9933/mcp", "streamable-http", t.TempDir()))
	if err == nil || !strings.Contains(err.Error(), "already has") {
		t.Fatalf("second install should refuse, got %v", err)
	}
}

func TestEveryClientHasAnEntry(t *testing.T) {
	for _, id := range clientIDs {
		entry := entryFor(id, "http://127.0.0.1:9933/mcp", "streamable-http", t.TempDir())
		if entry.label == "" {
			t.Errorf("%s has no label", id)
		}
		if strings.TrimSpace(entry.snippet) == "" {
			t.Errorf("%s has no snippet", id)
		}
		if entry.cli == "" && entry.path == "" {
			t.Errorf("%s has neither a path nor a command", id)
		}
	}
}

func TestDescribeClientReportsDetection(t *testing.T) {
	dir := isolate(t)

	missing := describeClient("cursor", "http://127.0.0.1:9933/mcp", "streamable-http", t.TempDir())
	if missing.Target.Detected {
		t.Error("a file that does not exist cannot be detected")
	}
	if !missing.Install.Can {
		t.Error("a missing config file is still installable: we create it")
	}

	path := filepath.Join(dir, ".cursor", "mcp.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}

	found := describeClient("cursor", "http://127.0.0.1:9933/mcp", "streamable-http", t.TempDir())
	if !found.Target.Detected {
		t.Error("the file exists and should be detected")
	}
}

func TestShortPathHidesHome(t *testing.T) {
	home := isolate(t)
	got := shortPath(filepath.Join(home, ".kiro", "settings", "mcp.json"))
	if strings.HasPrefix(got, home) {
		t.Errorf("path still leaks the home directory: %s", got)
	}
	if !strings.HasPrefix(got, "~") {
		t.Errorf("expected a ~ path, got %s", got)
	}
}
