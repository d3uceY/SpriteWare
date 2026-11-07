// Package onpath puts this program on PATH: one registry value on Windows, one
// symlink everywhere else.
//
// The installers do the same thing at install time. This package is for the
// portable install, the button in the window, and for explaining a refusal
// instead of failing silently.
package onpath

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// State answers "can I type the command yet".
type State struct {
	// OnPath is true when a new shell will find the target.
	OnPath bool `json:"onPath"`
	// InThisShell is true when this process already does. A PATH change never
	// reaches a terminal that is already open.
	InThisShell bool `json:"inThisShell"`
	// Dir is the folder on PATH that holds, or should hold, the target.
	Dir string `json:"dir"`
	// Found is what a lookup resolves to, when it resolves at all.
	Found string `json:"found,omitempty"`
	// Problem explains a refusal: a link pointing elsewhere, a folder that
	// cannot be written.
	Problem string `json:"problem,omitempty"`
}

// targetName is the command a user types: the executable's own file name.
func targetName(target string) string { return filepath.Base(target) }

// sameFile reports whether two paths name the same file, following links.
func sameFile(a, b string) bool {
	x, err := filepath.EvalSymlinks(a)
	if err != nil {
		return false
	}
	y, err := filepath.EvalSymlinks(b)
	if err != nil {
		return false
	}
	if runtime.GOOS == "windows" {
		return strings.EqualFold(x, y)
	}
	return x == y
}

// isExecutable is a cheap sanity check: a path that does not exist is not on
// PATH no matter what the registry says.
func isExecutable(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

// inScratch reports whether dir is a folder that will not exist next week: a
// `go run` binary, or anything else staged under the temp directory. Putting
// one of those on PATH leaves a dead entry behind for good.
func inScratch(dir string) bool {
	scratch := filepath.Clean(os.TempDir())
	clean := filepath.Clean(dir)
	if runtime.GOOS == "windows" {
		return strings.HasPrefix(strings.ToLower(clean), strings.ToLower(scratch)+string(filepath.Separator))
	}
	return strings.HasPrefix(clean, scratch+string(filepath.Separator))
}
