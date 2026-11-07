//go:build !windows

package onpath

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// candidateDirs are the folders a link may go in, best first. /usr/local/bin is
// where macOS and most Linux setups expect one; ~/.local/bin needs no elevation,
// so it is the fallback. A variable so a test can point it at a scratch folder.
var candidateDirs = func() []string {
	dirs := []string{"/usr/local/bin"}
	if home, err := os.UserHomeDir(); err == nil {
		dirs = append(dirs, filepath.Join(home, ".local", "bin"))
	}
	return dirs
}

// Add links target into the first folder it can write to.
func Add(target string) (string, error) {
	if inScratch(filepath.Dir(target)) {
		return "", fmt.Errorf("%s is a temporary folder, so a link to it would go stale", filepath.Dir(target))
	}

	var lastErr error
	for _, dir := range candidateDirs() {
		link := filepath.Join(dir, targetName(target))

		if existing, err := os.Readlink(link); err == nil {
			if sameFile(resolve(dir, existing), target) {
				return dir, nil
			}
			lastErr = fmt.Errorf("%s already points at %s", link, existing)
			continue
		}
		if err := os.MkdirAll(dir, 0o755); err != nil {
			lastErr = fmt.Errorf("create %s: %w", dir, err)
			continue
		}
		if err := os.Symlink(target, link); err != nil {
			lastErr = fmt.Errorf("link %s: %w", link, err)
			continue
		}
		return dir, nil
	}
	if lastErr == nil {
		lastErr = errors.New("no usable folder to link into")
	}
	return "", lastErr
}

// Remove deletes a link that points at target. A link pointing anywhere else is
// left alone.
func Remove(target string) error {
	for _, dir := range candidateDirs() {
		link := filepath.Join(dir, targetName(target))
		existing, err := os.Readlink(link)
		if err != nil {
			continue
		}
		if !sameFile(resolve(dir, existing), target) {
			continue
		}
		if err := os.Remove(link); err != nil {
			return err
		}
	}
	return nil
}

// Status reports the state without changing anything.
func Status(target string) State {
	st := State{Dir: preferredDir()}

	for _, dir := range pathDirs() {
		link := filepath.Join(dir, targetName(target))
		existing, err := os.Readlink(link)
		if err != nil {
			continue
		}
		if sameFile(resolve(dir, existing), target) {
			st.OnPath = true
			st.Dir = dir
			st.Found = link
		}
	}
	if st.Found == "" {
		if found, err := exec.LookPath(targetName(target)); err == nil {
			st.Found = found
		}
	}
	if st.Found != "" {
		st.InThisShell = sameFile(st.Found, target)
	}
	if !st.OnPath {
		st.Problem = "no link on PATH points at this program"
	}
	return st
}

// preferredDir is the folder Add would reach for first.
func preferredDir() string {
	if home, err := os.UserHomeDir(); err == nil {
		dir := filepath.Join(home, ".local", "bin")
		if info, err := os.Stat(dir); err == nil && info.IsDir() {
			return dir
		}
	}
	return candidateDirs()[0]
}

func pathDirs() []string {
	var dirs []string
	for _, dir := range filepath.SplitList(os.Getenv("PATH")) {
		if dir != "" {
			dirs = append(dirs, dir)
		}
	}
	return dirs
}

// resolve turns a relative link target into one relative to its folder, which is
// how a link written by hand may read.
func resolve(dir, link string) string {
	if filepath.IsAbs(link) {
		return link
	}
	return filepath.Join(dir, link)
}
