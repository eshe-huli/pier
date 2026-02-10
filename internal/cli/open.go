package cli

import (
	"fmt"
	"os/exec"
	"runtime"

	"github.com/spf13/cobra"

	"github.com/eshe-huli/pier/internal/config"
)

var openCmd = &cobra.Command{
	Use:   "open [name]",
	Short: "Open a project in the browser",
	Long: `Opens the .dock domain for a project in your default browser.

Examples:
  pier open              Open the current project
  pier open myapp        Open myapp.dock`,
	Args: cobra.MaximumNArgs(1),
	RunE: runOpen,
}

func init() {
	rootCmd.AddCommand(openCmd)
}

func runOpen(cmd *cobra.Command, args []string) error {
	name, err := resolveProjectName(args)
	if err != nil {
		return err
	}

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	url := fmt.Sprintf("http://%s.%s", name, cfg.TLD)

	fmt.Printf("  Opening %s...\n", cyan(url))

	var openCmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		openCmd = exec.Command("open", url)
	case "linux":
		openCmd = exec.Command("xdg-open", url)
	default:
		return fmt.Errorf("unsupported OS: %s", runtime.GOOS)
	}

	return openCmd.Run()
}
