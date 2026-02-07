package cli

import (
	"fmt"
	"os/exec"
	"runtime"

	"github.com/spf13/cobra"

	"github.com/eshe-huli/pier/internal/config"
)

var dashboardCmd = &cobra.Command{
	Use:   "dashboard",
	Short: "Open the Pier dashboard in your browser",
	Long:  `Opens the Traefik dashboard at traefik.<tld> in your default browser.`,
	RunE:  runDashboard,
}

func init() {
	rootCmd.AddCommand(dashboardCmd)
}

func runDashboard(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	url := fmt.Sprintf("http://traefik.%s", cfg.TLD)

	fmt.Println()
	info(fmt.Sprintf("Opening %s...", cyan(url)))
	fmt.Println()

	return openBrowser(url)
}

func openBrowser(url string) error {
	var cmd *exec.Cmd

	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "linux":
		cmd = exec.Command("xdg-open", url)
	default:
		return fmt.Errorf("unsupported platform: %s", runtime.GOOS)
	}

	return cmd.Start()
}
