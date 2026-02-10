package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/eshe-huli/pier/internal/pierfile"
	"github.com/eshe-huli/pier/internal/proxy"
)

var unlinkCmd = &cobra.Command{
	Use:   "unlink [name]",
	Short: "Stop a linked dev server and remove its route",
	Long: `Stops the dev server started by 'pier link' and removes the .dock proxy.

Examples:
  pier unlink              Unlink the current project
  pier unlink myapp        Unlink a specific project`,
	Args: cobra.MaximumNArgs(1),
	RunE: runUnlink,
}

func init() {
	rootCmd.AddCommand(unlinkCmd)
}

func runUnlink(cmd *cobra.Command, args []string) error {
	dir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting working directory: %w", err)
	}

	name := filepath.Base(dir)
	if pierfile.Exists(dir) {
		pf, err := pierfile.Load(dir)
		if err == nil && pf.Name != "" {
			name = pf.Name
		}
	}
	if len(args) > 0 {
		name = args[0]
	}

	fmt.Println()

	// Kill dev server
	pidFile := filepath.Join(dir, ".pier", "dev.pid")
	if _, err := os.Stat(pidFile); err == nil {
		killExistingDev(pidFile)
		success("Dev server stopped")
	} else {
		info("No dev server PID found")
	}

	// Remove proxy
	if proxy.FileProxyExists(name) {
		if err := proxy.RemoveFileProxy(name); err != nil {
			warn(fmt.Sprintf("Could not remove route: %s", err))
		} else {
			success("Route removed")
		}
	} else {
		info("No proxy route found")
	}

	fmt.Println()
	return nil
}
