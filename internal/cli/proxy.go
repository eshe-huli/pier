package cli

import (
	"fmt"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/eshe-huli/pier/internal/config"
	"github.com/eshe-huli/pier/internal/proxy"
)

var proxyCmd = &cobra.Command{
	Use:   "proxy <name> <port>",
	Short: "Route a local domain to a bare-metal process",
	Long: `Creates a domain routing rule so <name>.dock points to localhost:<port>.

Example:
  pier proxy myapp 3000    → http://myapp.dock → localhost:3000
  pier proxy api 8080      → http://api.dock → localhost:8080`,
	Args: cobra.ExactArgs(2),
	RunE: runProxy,
}

func init() {
	rootCmd.AddCommand(proxyCmd)
}

func runProxy(cmd *cobra.Command, args []string) error {
	name := args[0]
	port, err := strconv.Atoi(args[1])
	if err != nil || port < 1 || port > 65535 {
		return fmt.Errorf("invalid port: %s (must be 1-65535)", args[1])
	}

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	// Check if proxy already exists
	if proxy.FileProxyExists(name) {
		warn(fmt.Sprintf("Proxy '%s' already exists, overwriting...", name))
	}

	// Create the file proxy
	if err := proxy.CreateFileProxy(name, port, cfg.TLD); err != nil {
		return fmt.Errorf("creating proxy: %w", err)
	}

	domain := fmt.Sprintf("%s.%s", name, cfg.TLD)
	fmt.Println()
	success(fmt.Sprintf("http://%s → localhost:%d", cyan(domain), port))
	fmt.Println()
	info("Traefik will auto-detect the new route within seconds.")
	fmt.Println()

	return nil
}
