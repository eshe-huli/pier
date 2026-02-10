package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/eshe-huli/pier/internal/docker"
)

var envCmd = &cobra.Command{
	Use:   "env [name]",
	Short: "Show environment variables for a running container",
	Long: `Displays the environment variables injected into a running container.

Examples:
  pier env              Show env for current project
  pier env myapp        Show env for myapp`,
	Args: cobra.MaximumNArgs(1),
	RunE: runEnv,
}

func init() {
	rootCmd.AddCommand(envCmd)
}

func runEnv(cmd *cobra.Command, args []string) error {
	name, err := resolveProjectName(args)
	if err != nil {
		return err
	}

	ctx := context.Background()
	info, err := docker.GetContainer(ctx, name)
	if err != nil {
		return fmt.Errorf("container %s not found: %w", name, err)
	}

	fmt.Println()
	fmt.Printf("  %s %s\n", bold("Environment:"), cyan(name))
	fmt.Printf("  %s\n", dim(strings.Repeat("─", 50)))

	for _, env := range info.Config.Env {
		parts := strings.SplitN(env, "=", 2)
		if len(parts) == 2 {
			fmt.Printf("  %s=%s\n", green(parts[0]), parts[1])
		} else {
			fmt.Printf("  %s\n", env)
		}
	}
	fmt.Println()
	return nil
}
