// Package aseprite puts SpriteWare's extension where Aseprite will load it.
//
// A live-mode user should never copy a Lua file by hand, so the app writes it:
// one package.json and one script, in Aseprite's own extension layout.
package aseprite

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/d3uceY/spriteware/internal/luaext"
)

// Folder is the extension's directory name, and the `name` in package.json.
const Folder = "spriteware"

// EnvOverride points the installer away from the platform default.
const EnvOverride = "SPRITEWARE_EXTENSIONS_DIR"

// Result reports what an install did, so a failure can be shown.
type Result struct {
	Dir    string `json:"dir"`
	Wrote  bool   `json:"wrote"`
	Reason string `json:"reason"`
}

// ExtensionsDir is where Aseprite looks for installed extensions.
func ExtensionsDir() (string, error) {
	if custom := os.Getenv(EnvOverride); custom != "" {
		return custom, nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("no home directory: %w", err)
	}

	switch runtime.GOOS {
	case "windows":
		appData := os.Getenv("APPDATA")
		if appData == "" {
			return "", fmt.Errorf("APPDATA is not set, so the Aseprite folder is unknown")
		}
		return filepath.Join(appData, "Aseprite", "extensions"), nil
	case "darwin":
		return filepath.Join(home, "Library", "Application Support", "Aseprite", "extensions"), nil
	default:
		return filepath.Join(home, ".config", "aseprite", "extensions"), nil
	}
}

// packageJSON mirrors the file a real Aseprite extension ships with.
type packageJSON struct {
	Name        string      `json:"name"`
	DisplayName string      `json:"displayName"`
	Description string      `json:"description"`
	Version     string      `json:"version"`
	Author      string      `json:"author"`
	Publisher   string      `json:"publisher"`
	License     string      `json:"license"`
	Categories  []string    `json:"categories"`
	Contributes contributes `json:"contributes"`
	API         int         `json:"api"`
}

type contributes struct {
	Scripts []script `json:"scripts"`
}

type script struct {
	Path string `json:"path"`
}

// Install writes the extension with the given bridge port baked in.
//
// Idempotent: a second call with the same port leaves the files alone, so it is
// safe to run on every start.
func Install(port int) (Result, error) {
	dir, err := ExtensionsDir()
	if err != nil {
		return Result{}, err
	}
	target := filepath.Join(dir, Folder)

	manifest, err := json.MarshalIndent(packageJSON{
		Name:        Folder,
		DisplayName: "SpriteWare",
		Description: "Lets SpriteWare draw in this Aseprite session over a local socket. Loopback only; nothing leaves the machine.",
		Version:     "0.1.0",
		Author:      "d3uceY",
		Publisher:   "d3uceY",
		License:     "MIT",
		Categories:  []string{"Scripts"},
		Contributes: contributes{Scripts: []script{{Path: "./spriteware.lua"}}},
		API:         1,
	}, "", "  ")
	if err != nil {
		return Result{Dir: target}, fmt.Errorf("encode the extension manifest: %w", err)
	}

	script := luaext.Render(port)
	if len(script) == 0 {
		return Result{Dir: target}, fmt.Errorf("the embedded extension script is missing")
	}

	wrote, err := writeIfChanged(target, "package.json", append(manifest, '\n'))
	if err != nil {
		return Result{Dir: target}, err
	}
	scriptWrote, err := writeIfChanged(target, "spriteware.lua", script)
	if err != nil {
		return Result{Dir: target}, err
	}

	if !wrote && !scriptWrote {
		return Result{Dir: target, Reason: "already installed and current"}, nil
	}
	return Result{Dir: target, Wrote: true, Reason: "installed for Aseprite to load on its next start"}, nil
}

// writeIfChanged rewrites a file only when its bytes differ.
func writeIfChanged(dir, name string, body []byte) (bool, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return false, fmt.Errorf("could not create %s: %w", dir, err)
	}
	path := filepath.Join(dir, name)

	if existing, err := os.ReadFile(path); err == nil && string(existing) == string(body) {
		return false, nil
	}

	tmp, err := os.CreateTemp(dir, ".spriteware-*.tmp")
	if err != nil {
		return false, fmt.Errorf("could not stage %s: %w", name, err)
	}
	staged := tmp.Name()
	defer os.Remove(staged)

	if _, err := tmp.Write(body); err != nil {
		tmp.Close()
		return false, fmt.Errorf("could not write %s: %w", name, err)
	}
	if err := tmp.Close(); err != nil {
		return false, fmt.Errorf("could not close %s: %w", name, err)
	}
	if err := os.Rename(staged, path); err != nil {
		return false, fmt.Errorf("could not replace %s: %w", path, err)
	}
	return true, nil
}

// Installed reports whether an extension is present, and which port it dials.
func Installed() (string, int, bool) {
	dir, err := ExtensionsDir()
	if err != nil {
		return "", 0, false
	}
	body, err := os.ReadFile(filepath.Join(dir, Folder, "spriteware.lua"))
	if err != nil {
		return "", 0, false
	}
	return filepath.Join(dir, Folder), luaext.PortIn(body), true
}
