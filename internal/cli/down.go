package cli

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/eshe-huli/pier/internal/proxy"
)

var downCmd = &cobra.Command{
	Use:   "down",
	Short: "Stop Pier infrastructure",
	Long:  `Stops and removes the Traefik container. DNS and nginx config remain in place.`,
	RunE:  runDown,
}

func init() {
	rootCmd.AddCommand(downCmd)
}

func runDown(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	fmt.Println()

	if !proxy.IsTraefikRunning(ctx) {
		info("Traefik is not running")
		fmt.Println()
		return nil
	}

	if err := proxy.StopTraefik(ctx); err != nil {
		fail(fmt.Sprintf("Failed to stop Traefik: %s", err))
		return err
	}

	success("Traefik stopped and removed")
	fmt.Println()
	info("DNS and nginx config are still in place. Run 'pier init' to start again.")
	fmt.Println()

	return nil
}
