//go:build windows

package onpath

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows/registry"
)

// Windows keeps a persistent PATH in two places. The installer writes the machine
// one when it runs elevated; everything here writes the user one, which needs no
// elevation and is what the button can do without a UAC prompt.
const (
	userKey    = `Environment`
	machineKey = `SYSTEM\CurrentControlSet\Control\Session Manager\Environment`
)

// Add appends the target's folder to the user's PATH. Adding a folder twice is a
// no-op, so the button can be pressed without thinking.
func Add(target string) (string, error) {
	dir := filepath.Dir(target)
	if dir == "" || dir == "." {
		return "", fmt.Errorf("%s has no folder to add", target)
	}
	if inScratch(dir) {
		return "", fmt.Errorf("%s is a temporary folder, so adding it would leave a dead PATH entry", dir)
	}

	if contains(readPath(), dir) {
		return dir, nil
	}

	current, ok := pathValue(registry.CURRENT_USER, userKey)
	if !ok {
		current = ""
	}
	next := strings.TrimSpace(current)
	if next != "" && !strings.HasSuffix(next, ";") {
		next += ";"
	}
	next += dir

	k, err := registry.OpenKey(registry.CURRENT_USER, userKey, registry.SET_VALUE)
	if err != nil {
		return "", fmt.Errorf("open HKCU\\Environment: %w", err)
	}
	defer k.Close()
	// PATH is REG_EXPAND_SZ on a normal install, and writing REG_SZ would stop
	// any %VARS% already in it from expanding.
	if err := k.SetExpandStringValue("Path", next); err != nil {
		return "", fmt.Errorf("write HKCU\\Environment Path: %w", err)
	}
	broadcast()
	return dir, nil
}

// Remove drops the target's folder from the user's PATH. A machine-scope entry is
// left alone: removing that needs elevation, so it belongs to the uninstaller.
func Remove(target string) error {
	value, ok := pathValue(registry.CURRENT_USER, userKey)
	if !ok {
		return nil
	}
	dir := filepath.Clean(filepath.Dir(target))

	parts := strings.Split(value, ";")
	kept := make([]string, 0, len(parts))
	for _, p := range parts {
		if strings.TrimSpace(p) == "" {
			continue
		}
		if strings.EqualFold(filepath.Clean(os.ExpandEnv(strings.TrimSpace(p))), dir) {
			continue
		}
		kept = append(kept, p)
	}
	if len(kept) == len(parts) {
		return nil
	}

	k, err := registry.OpenKey(registry.CURRENT_USER, userKey, registry.SET_VALUE)
	if err != nil {
		return fmt.Errorf("open HKCU\\Environment: %w", err)
	}
	defer k.Close()
	if err := k.SetExpandStringValue("Path", strings.Join(kept, ";")); err != nil {
		return fmt.Errorf("write HKCU\\Environment Path: %w", err)
	}
	broadcast()
	return nil
}

// Status reports the state without changing anything.
func Status(target string) State {
	dir := filepath.Dir(target)
	st := State{Dir: dir}

	if found, err := exec.LookPath(targetName(target)); err == nil && isExecutable(found) {
		st.Found = found
		st.InThisShell = sameFile(found, target)
	}
	st.OnPath = contains(readPath(), dir)
	if !st.OnPath {
		st.Problem = dir + " is not in the user or machine Path"
	}
	return st
}

// readPath is every PATH entry this account can see, machine first.
func readPath() []string {
	var entries []string
	for _, k := range []struct {
		root registry.Key
		sub  string
	}{
		{registry.LOCAL_MACHINE, machineKey},
		{registry.CURRENT_USER, userKey},
	} {
		if value, ok := pathValue(k.root, k.sub); ok {
			entries = append(entries, splitPath(value)...)
		}
	}
	return entries
}

func pathValue(root registry.Key, sub string) (string, bool) {
	k, err := registry.OpenKey(root, sub, registry.QUERY_VALUE)
	if err != nil {
		return "", false
	}
	defer k.Close()
	value, _, err := k.GetStringValue("Path")
	if err != nil {
		return "", false
	}
	return value, true
}

func splitPath(value string) []string {
	parts := strings.Split(value, ";")
	entries := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(os.ExpandEnv(strings.TrimSpace(p)))
		if p == "" {
			continue
		}
		entries = append(entries, filepath.Clean(p))
	}
	return entries
}

func contains(entries []string, dir string) bool {
	want := filepath.Clean(dir)
	for _, entry := range entries {
		if strings.EqualFold(entry, want) {
			return true
		}
	}
	return false
}

const (
	hwndBroadcast   = 0xFFFF
	wmSettingChange = 0x001A
	smtoAbortIfHung = 0x0002
)

var procSendMessageTimeout = syscall.NewLazyDLL("user32.dll").NewProc("SendMessageTimeoutW")

// broadcast tells Explorer and anything started from it that the environment
// changed. Existing terminals keep their copy, which is why Add prints a note.
func broadcast() {
	name, err := syscall.UTF16PtrFromString("Environment")
	if err != nil {
		return
	}
	_, _, _ = procSendMessageTimeout.Call(
		hwndBroadcast, wmSettingChange, 0, uintptr(unsafe.Pointer(name)), smtoAbortIfHung, 5000, 0,
	)
}
