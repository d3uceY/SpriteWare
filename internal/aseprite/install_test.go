package aseprite

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/d3uceY/spriteware/internal/luaext"
)

func isolated(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv(EnvOverride, dir)
	return dir
}

// The whole point: one call leaves a loadable extension behind.
func TestInstallWritesALoadableExtension(t *testing.T) {
	root := isolated(t)

	result, err := Install(9934)
	if err != nil {
		t.Fatalf("install: %v", err)
	}
	if !result.Wrote {
		t.Error("the first install should report that it wrote files")
	}
	if result.Dir != filepath.Join(root, Folder) {
		t.Fatalf("installed into %s", result.Dir)
	}

	manifest, err := os.ReadFile(filepath.Join(result.Dir, "package.json"))
	if err != nil {
		t.Fatalf("no manifest: %v", err)
	}
	var parsed packageJSON
	if err := json.Unmarshal(manifest, &parsed); err != nil {
		t.Fatalf("the manifest is not valid JSON: %v", err)
	}
	if parsed.Name != Folder {
		t.Errorf("manifest name = %q, the folder is %q", parsed.Name, Folder)
	}
	if len(parsed.Contributes.Scripts) != 1 || parsed.Contributes.Scripts[0].Path != "./spriteware.lua" {
		t.Errorf("Aseprite would load nothing: %+v", parsed.Contributes)
	}
	// Aseprite refuses a manifest without an API version.
	if parsed.API != 1 {
		t.Errorf("api = %d, want 1", parsed.API)
	}

	script, err := os.ReadFile(filepath.Join(result.Dir, "spriteware.lua"))
	if err != nil {
		t.Fatalf("no script: %v", err)
	}
	if len(script) < 1000 {
		t.Errorf("the script is only %d bytes, which looks truncated", len(script))
	}
	if !strings.Contains(string(script), "start_live") {
		t.Error("the installed script is not the extension")
	}
}

// A second install must not rewrite the files: that would look like a change on
// every start.
func TestInstallIsIdempotent(t *testing.T) {
	root := isolated(t)

	first, err := Install(9934)
	if err != nil {
		t.Fatal(err)
	}
	before, err := os.Stat(filepath.Join(first.Dir, "spriteware.lua"))
	if err != nil {
		t.Fatal(err)
	}

	second, err := Install(9934)
	if err != nil {
		t.Fatal(err)
	}
	if second.Wrote {
		t.Error("the second install rewrote identical files")
	}
	if second.Reason == "" {
		t.Error("the second install gave no reason")
	}

	after, err := os.Stat(filepath.Join(first.Dir, "spriteware.lua"))
	if err != nil {
		t.Fatal(err)
	}
	if !after.ModTime().Equal(before.ModTime()) {
		t.Error("the file was touched even though its contents were current")
	}
	_ = root
}

// A port change has to reach the installed copy, or live mode goes silently
// dead.
func TestChangingThePortRewritesTheInstalledScript(t *testing.T) {
	isolated(t)

	if _, err := Install(9934); err != nil {
		t.Fatal(err)
	}
	result, err := Install(10001)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Wrote {
		t.Fatal("a new port must rewrite the script")
	}

	_, port, ok := Installed()
	if !ok {
		t.Fatal("Installed() says nothing is there")
	}
	if port != 10001 {
		t.Errorf("the installed script dials %d, want 10001", port)
	}
}

func TestInstalledReportsNothingWhenEmpty(t *testing.T) {
	isolated(t)

	if _, _, ok := Installed(); ok {
		t.Error("an empty extensions folder reported an install")
	}
}

// The shipped script and the rendered one must agree on the default port.
func TestRenderedDefaultMatchesTheEmbeddedDefault(t *testing.T) {
	body := luaext.Render(0)
	if got := luaext.PortIn(body); got != luaext.EmbeddedPort() {
		t.Fatalf("rendered default port is %d, the embedded one is %d", got, luaext.EmbeddedPort())
	}
	if got := luaext.PortIn(luaext.Render(9999)); got != 9999 {
		t.Fatalf("rendering with 9999 produced %d", got)
	}
}

func TestOverrideIsUsedForTheFolder(t *testing.T) {
	root := isolated(t)
	dir, err := ExtensionsDir()
	if err != nil {
		t.Fatal(err)
	}
	if dir != root {
		t.Fatalf("ExtensionsDir = %s, want the override %s", dir, root)
	}
}
