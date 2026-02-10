package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/eshe-huli/pier/internal/config"
	"github.com/eshe-huli/pier/internal/proxy"
)

var cleanCmd = &cobra.Command{
	Use:   "clean",
	Short: "Remove stale proxies pointing to dead ports",
	Long: `Scans all file-based proxy routes and removes any whose backend port
is no longer listening. Useful for cleaning up after crashed dev servers.

Examples:
  pier clean`,
	RunE: runClean,
}

func init() {
	rootCmd.AddCommand(cleanCmd)
}

func runClean(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	proxies, err := proxy.ListFileProxies(cfg.TLD)
	if err != nil {
		return fmt.Errorf("listing proxies: %w", err)
	}

	fmt.Println()

	removed := 0
	for _, p := range proxies {
		if p.Port > 0 && !proxy.IsProxyBackendAlive(p.Port) {
			fmt.Printf("  %s %s (port %d dead)\n", red("✗"), p.Name, p.Port)
			if err := proxy.RemoveFileProxy(p.Name); err == nil {
				removed++
			}
		}
	}

	if removed == 0 {
		info("No stale proxies found. All clean! 🧹")
	} else {
		success(fmt.Sprintf("Removed %d stale proxy route(s)", removed))
	}

	fmt.Println()
	return nil
}
