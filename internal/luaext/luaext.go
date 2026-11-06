// Package luaext embeds the Aseprite-side Lua so the binary installs nothing
// alongside itself.
package luaext

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
)

//go:embed extension/spriteware.lua headless/runner.lua
var files embed.FS

// Paths inside the package, slash-separated.
const (
	ExtensionPath = "extension/spriteware.lua"
	RunnerPath    = "headless/runner.lua"
)

// portLine matches the assignment the installer rewrites. The end marker makes
// the rewrite safe: a bare `or 9934` also appears in prose.
var portLine = regexp.MustCompile(`(?m)^local BRIDGE_PORT = [^\r\n]*-- spriteware:bridge-port\r?$`)

// Prepare unpacks both Lua files and returns the directory plus a cleanup func.
//
// An explicit root with both files present is used as-is, so live edits work;
// cleanup then does nothing, since those files are the user's.
func Prepare(root string) (string, func(), error) {
	if root != "" && hasBoth(root) {
		return root, func() {}, nil
	}

	dir, err := os.MkdirTemp("", "spriteware-lua-*")
	if err != nil {
		return "", func() {}, fmt.Errorf("create lua scratch directory: %w", err)
	}
	if err := writeAll(dir); err != nil {
		_ = os.RemoveAll(dir)
		return "", func() {}, err
	}
	// The folder was created here, so it is this package's to remove.
	return dir, func() { _ = os.RemoveAll(dir) }, nil
}

// Render bakes the bridge port into the extension script, so an install dials
// the port the app listens on.
func Render(port int) []byte {
	body, err := files.ReadFile(ExtensionPath)
	if err != nil {
		return nil
	}
	if port <= 0 || port > 65535 {
		return body
	}
	line := "local BRIDGE_PORT = tonumber(app.params[\"bridgePort\"] or \"\") or " +
		strconv.Itoa(port) + " -- spriteware:bridge-port"
	return portLine.ReplaceAll(body, []byte(line))
}

// EmbeddedPort reports the port the shipped script falls back to.
func EmbeddedPort() int {
	body, err := files.ReadFile(ExtensionPath)
	if err != nil {
		return 0
	}
	return PortIn(body)
}

// PortIn reads the port out of a rendered script, or 0 when the line is missing.
func PortIn(body []byte) int {
	match := portLine.FindSubmatch(body)
	if match == nil {
		return 0
	}
	digits := regexp.MustCompile(`or (\d+)`).FindSubmatch(match[0])
	if digits == nil {
		return 0
	}
	port, _ := strconv.Atoi(string(digits[1]))
	return port
}

func hasBoth(root string) bool {
	for _, rel := range []string{ExtensionPath, RunnerPath} {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel))); err != nil {
			return false
		}
	}
	return true
}

func writeAll(dir string) error {
	for _, rel := range []string{ExtensionPath, RunnerPath} {
		data, err := files.ReadFile(rel)
		if err != nil {
			return fmt.Errorf("read embedded %s: %w", rel, err)
		}
		target := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return fmt.Errorf("create %s: %w", filepath.Dir(target), err)
		}
		if err := os.WriteFile(target, data, 0o644); err != nil {
			return fmt.Errorf("write %s: %w", rel, err)
		}
	}
	return nil
}
