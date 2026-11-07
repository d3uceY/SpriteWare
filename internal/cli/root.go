// Package cli is the terminal front door: the tools, the settings and the checks
// the window offers, reachable from a shell and from a script.
//
// One binary, two faces. main.go hands the arguments here and never builds a
// window; with no arguments it opens the window and this package is not entered.
package cli

import (
	"github.com/spf13/cobra"
)

// Run parses args and returns the process exit code.
func Run(version string, args []string) int {
	// A production Windows build is -H windowsgui: it owns no console, so
	// Ctrl+C never reaches it. Attaching to the parent's console fixes that.
	attachConsole()

	root := newRoot(version)
	root.SetArgs(args)
	if err := root.Execute(); err != nil {
		return 1
	}
	return 0
}

// newRoot builds the command tree. Separate from Run so a test can walk the
// same help a user sees.
func newRoot(version string) *cobra.Command {
	root := &cobra.Command{
		Use:   "spriteware",
		Short: "Pixel-art tooling an AI agent can actually use",
		Long: `SpriteWare serves pixel-art tools to an AI agent over MCP.

  spriteware mcp      serve MCP in this terminal until Ctrl+C
  spriteware doctor   check Aseprite, the ports, the extension and PATH
  spriteware config   read and write the settings file
  spriteware path     put this program on PATH, or take it off
  spriteware eval     run and score the practice benchmarks

With no arguments the desktop window opens instead.`,
		Version:      version,
		SilenceUsage: true,
	}
	root.PersistentFlags().BoolVar(&jsonOut, "json", false, "print machine-readable JSON")
	root.AddCommand(
		newMCPCmd(version),
		newDoctorCmd(),
		newConfigCmd(),
		newPathCmd(),
		newVersionCmd(version),
		newEvalCmd(),
	)
	return root
}
