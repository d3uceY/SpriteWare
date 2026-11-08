package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// tree is a miniature of the files a release stamps, with the markers the real
// files carry.
func tree() map[string]string {
	return map[string]string{
		"build/config.yml": "version: '3'\n\ninfo:\n  companyName: \"d3uceY\"\n  version: \"0.1.0\" # The application version\n\ndev_mode:\n  root_path: .\n",
		"build/windows/info.json": `{
	"fixed": {
		"file_version": "0.1.0"
	},
	"info": {
		"0409": {
			"ProductVersion": "0.1.0",
			"ProductName": "SpriteWare"
		}
	}
}`,
		"build/windows/wails.exe.manifest": `<?xml version="1.0"?>
<assembly manifestVersion="1.0">
    <assemblyIdentity type="win32" name="com.d3ucey.spriteware" version="0.1.0" processorArchitecture="*"/>
    <dependency>
        <dependentAssembly>
            <assemblyIdentity type="win32" name="Microsoft.Windows.Common-Controls" version="6.0.0.0" processorArchitecture="*" publicKeyToken="6595b64144ccf1df" language="*"/>
        </dependentAssembly>
    </dependency>
</assembly>`,
		"build/windows/nsis/wails_tools.nsh": "!ifndef INFO_PRODUCTVERSION\n    !define INFO_PRODUCTVERSION \"0.1.0\"\n!endif\n",
		"build/linux/nfpm/nfpm.yaml":         "name: \"spriteware\"\nversion: \"0.1.0\"\nsection: \"default\"\n",
		"build/darwin/Info.plist": `<?xml version="1.0"?>
<plist version="1.0">
	<dict>
		<key>CFBundleShortVersionString</key>
		<string>0.1.0</string>
		<key>CFBundleVersion</key>
		<string>0.1.0</string>
	</dict>
</plist>`,
	}
}

func writeTree(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for name, body := range files {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func read(t *testing.T, root, name string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(name)))
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func TestStampAllRewritesEveryMarker(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, tree())

	if err := stampAll(root, "1.2.3"); err != nil {
		t.Fatal(err)
	}

	want := map[string][]string{
		"build/config.yml":                   {`version: "1.2.3"`},
		"build/windows/info.json":            {`"file_version": "1.2.3"`, `"ProductVersion": "1.2.3"`},
		"build/windows/wails.exe.manifest":   {`version="1.2.3.0"`},
		"build/windows/nsis/wails_tools.nsh": {`!define INFO_PRODUCTVERSION "1.2.3"`},
		"build/linux/nfpm/nfpm.yaml":         {`version: "1.2.3"`},
		"build/darwin/Info.plist": {`<key>CFBundleShortVersionString</key>
		<string>1.2.3</string>`, `<key>CFBundleVersion</key>
		<string>1.2.3</string>`},
	}
	for name, markers := range want {
		body := read(t, root, name)
		for _, marker := range markers {
			if !strings.Contains(body, marker) {
				t.Errorf("%s missing %q:\n%s", name, marker, body)
			}
		}
		if strings.Contains(body, "0.1.0") {
			t.Errorf("%s still holds the old version:\n%s", name, body)
		}
	}

	// The Common-Controls dependency is not the app's version and must survive.
	manifest := read(t, root, "build/windows/wails.exe.manifest")
	if !strings.Contains(manifest, `name="Microsoft.Windows.Common-Controls" version="6.0.0.0"`) {
		t.Fatalf("the dependency assembly identity was rewritten:\n%s", manifest)
	}
}

func TestStampAllLeavesTheToolchainVersionAlone(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, tree())

	if err := stampAll(root, "2.0.0"); err != nil {
		t.Fatal(err)
	}
	if body := read(t, root, "build/config.yml"); !strings.Contains(body, "version: '3'") {
		t.Fatalf("the file's own schema version was rewritten:\n%s", body)
	}
}

func TestStampAllIsIdempotent(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, tree())

	if err := stampAll(root, "3.4.5"); err != nil {
		t.Fatal(err)
	}
	first := read(t, root, "build/darwin/Info.plist")

	if err := stampAll(root, "3.4.5"); err != nil {
		t.Fatal(err)
	}
	if second := read(t, root, "build/darwin/Info.plist"); second != first {
		t.Fatalf("second run changed the file:\n%s\n%s", first, second)
	}
}

func TestStampAllFailsOnAMissingMarker(t *testing.T) {
	root := t.TempDir()
	files := tree()
	files["build/windows/nsis/wails_tools.nsh"] = "!define INFO_PRODUCTNAME \"SpriteWare\"\n"
	writeTree(t, root, files)

	err := stampAll(root, "1.0.0")
	if err == nil {
		t.Fatal("a file without the marker must fail the release")
	}
	if !strings.Contains(err.Error(), "wails_tools.nsh") {
		t.Fatalf("error does not name the file: %v", err)
	}
}

func TestStampAllFailsOnADuplicateMarker(t *testing.T) {
	root := t.TempDir()
	files := tree()
	files["build/linux/nfpm/nfpm.yaml"] = "version: \"0.1.0\"\nversion: \"0.1.0\"\n"
	writeTree(t, root, files)

	if err := stampAll(root, "1.0.0"); err == nil {
		t.Fatal("an ambiguous marker must fail rather than guess")
	}
}

func TestStampAllFailsOnAMissingFile(t *testing.T) {
	root := t.TempDir()
	files := tree()
	delete(files, "build/darwin/Info.plist")
	writeTree(t, root, files)

	if err := stampAll(root, "1.0.0"); err == nil {
		t.Fatal("a missing file must fail")
	}
}

func TestNormalize(t *testing.T) {
	ok := map[string]string{
		"1.2.3":   "1.2.3",
		"v1.2.3":  "1.2.3",
		"V1.2.3":  "1.2.3",
		" 2.0.10": "2.0.10",
	}
	for in, want := range ok {
		got, err := normalize(in)
		if err != nil {
			t.Errorf("normalize(%q): %v", in, err)
			continue
		}
		if got != want {
			t.Errorf("normalize(%q) = %q, want %q", in, got, want)
		}
	}

	for _, bad := range []string{"", "v1.2", "1.2.3-beta.1", "latest", "1.2.3.4", "v"} {
		if _, err := normalize(bad); err == nil {
			t.Errorf("normalize(%q) accepted an unstampable version", bad)
		}
	}
}
