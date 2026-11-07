package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/d3uceY/spriteware/internal/onpath"
)

func newPathCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "path",
		Short: "Put this program on PATH, or take it off",
		Long: `A terminal opened after this finds the command; one that is already open keeps
its old PATH until it is restarted.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error { return runPathStatus(cmd) },
	}
	cmd.AddCommand(
		&cobra.Command{
			Use:   "status",
			Short: "Report whether the command is reachable",
			Args:  cobra.NoArgs,
			RunE:  func(cmd *cobra.Command, _ []string) error { return runPathStatus(cmd) },
		},
		&cobra.Command{
			Use:   "add",
			Short: "Make the command reachable from a new terminal",
			Args:  cobra.NoArgs,
			RunE:  runPathAdd,
		},
		&cobra.Command{
			Use:   "remove",
			Short: "Take the command back off PATH",
			Args:  cobra.NoArgs,
			RunE:  runPathRemove,
		},
	)
	return cmd
}

// selfPath is the executable a user should be able to type.
func selfPath() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("cannot tell where this program is: %w", err)
	}
	// Resolve first: a link would put a link on PATH, and Remove has to match
	// what is actually there.
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	return exe, nil
}

func runPathStatus(cmd *cobra.Command) error {
	exe, err := selfPath()
	if err != nil {
		return err
	}
	return reportPath(cmd, onpath.Status(exe), false)
}

func runPathAdd(cmd *cobra.Command, _ []string) error {
	exe, err := selfPath()
	if err != nil {
		return err
	}
	if _, err := onpath.Add(exe); err != nil {
		return err
	}
	return reportPath(cmd, onpath.Status(exe), true)
}

func runPathRemove(cmd *cobra.Command, _ []string) error {
	exe, err := selfPath()
	if err != nil {
		return err
	}
	if err := onpath.Remove(exe); err != nil {
		return err
	}
	return reportPath(cmd, onpath.Status(exe), false)
}

// reportPath is the one shape all three subcommands print.
func reportPath(cmd *cobra.Command, st onpath.State, changed bool) error {
	if jsonOut {
		return printJSON(cmd.OutOrStdout(), st)
	}
	out := cmd.OutOrStdout()

	if st.OnPath {
		fmt.Fprintf(out, "ok    %s is on PATH\n", st.Dir)
		if changed && !st.InThisShell {
			fmt.Fprintln(out, "note  a terminal that is already open keeps its old PATH; open a new one")
		}
		return nil
	}
	if changed {
		fmt.Fprintf(out, "warn  %s\n", st.Problem)
	} else {
		fmt.Fprintf(out, "warn  %s\nfix   spriteware path add\n", st.Problem)
	}
	return nil
}
