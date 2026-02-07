package cli

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/eshe-huli/pier/internal/config"
	"github.com/eshe-huli/pier/internal/proxy"
)

var restartCmd = &cobra.Command{
	Use:   "restart",
	Short: "Restart Pier infrastructure",
	Long:  `Stops and restarts the Traefik container.`,
	RunE:  runRestart,
}

func init() {
	rootCmd.AddCommand(restartCmd)
}

func runRestart(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	fmt.Println()

	// Stop Traefik
	step(1, "Stopping Traefik...")
	if err := proxy.StopTraefik(ctx); err != nil {
		warn(fmt.Sprintf("Stop returned: %s", err))
	}
	success("Traefik stopped")

	// Regenerate config (in case TLD or settings changed)
	step(2, "Regenerating Traefik config...")
	if err := proxy.GenerateTraefikConfig(cfg); err != nil {
		return fmt.Errorf("generating config: %w", err)
	}

	// Start Traefik
	step(3, "Starting Traefik...")
	if err := proxy.StartTraefik(ctx, cfg); err != nil {
		return fmt.Errorf("starting Traefik: %w", err)
	}

	success(fmt.Sprintf("Traefik restarted on :%d", cfg.Traefik.Port))
	fmt.Println()

	return nil
}
