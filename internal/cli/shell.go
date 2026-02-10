package cli

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/spf13/cobra"
)

var shellCmd = &cobra.Command{
	Use:   "shell [name]",
	Short: "Open a shell in a running container",
	Long: `Opens an interactive shell inside a running container.
Tries /bin/sh if bash is not available.

Examples:
  pier shell              Shell into current project's container
  pier shell myapp        Shell into myapp`,
	Aliases: []string{"exec", "sh"},
	Args:    cobra.MaximumNArgs(1),
	RunE:    runShell,
}

func init() {
	rootCmd.AddCommand(shellCmd)
}

func runShell(cmd *cobra.Command, args []string) error {
	name, err := resolveProjectName(args)
	if err != nil {
		return err
	}

	// Try bash first, fall back to sh
	for _, shell := range []string{"/bin/bash", "/bin/sh"} {
		c := exec.Command("docker", "exec", "-it", name, shell)
		c.Stdin = os.Stdin
		c.Stdout = os.Stdout
		c.Stderr = os.Stderr
		if err := c.Run(); err == nil {
			return nil
		}
	}

	return fmt.Errorf("could not open shell in container %s", name)
}
