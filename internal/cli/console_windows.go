//go:build windows

package cli

import "syscall"

// ATTACH_PARENT_PROCESS is (DWORD)-1: attach to whatever console started us.
const attachParentProcess = ^uint32(0)

var procAttachConsole = syscall.NewLazyDLL("kernel32.dll").NewProc("AttachConsole")

// attachConsole reconnects a -H windowsgui binary to the terminal that launched
// it, so Ctrl+C reaches the process. The standard handles are inherited already,
// and rebinding them to CONOUT$ would break a shell redirect, so this only
// attaches.
//
// It fails, harmlessly, when there is no console: a double-click, or a shortcut.
func attachConsole() {
	_, _, _ = procAttachConsole.Call(uintptr(attachParentProcess))
}
