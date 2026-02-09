package cli

import (
	"os"

	"github.com/spf13/cobra"
)

var projectCmd = &cobra.Command{
	Use:        "project",
	Short:      "Project-level commands (deprecated: use `pier init` instead)",
	Deprecated: "use `pier init` directly inside a project directory",
}

var projectInitCmd = &cobra.Command{
	Use:        "init",
	Short:      "Initialize a project for Pier (deprecated: use `pier init`)",
	Deprecated: "use `pier init` directly inside a project directory",
	RunE: func(cmd *cobra.Command, args []string) error {
		dir, err := os.Getwd()
		if err != nil {
			return err
		}
		return runProjectInitLogic(dir)
	},
}

func init() {
	projectCmd.AddCommand(projectInitCmd)
	rootCmd.AddCommand(projectCmd)
}
