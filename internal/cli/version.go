package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

// newVersionCmd reports the version baked into the binary. --version does the
// same thing; this exists because a script that greps the output should not have
// to know which of the two the CLI picked that release.
func newVersionCmd(version string) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the version",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if jsonOut {
				return printJSON(cmd.OutOrStdout(), map[string]string{"version": version})
			}
			_, err := fmt.Fprintln(cmd.OutOrStdout(), "spriteware "+version)
			return err
		},
	}
}
