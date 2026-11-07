//go:build !windows

package cli

// attachConsole is a no-op where the build already owns a console.
func attachConsole() {}
