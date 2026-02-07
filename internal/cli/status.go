package cli

import (
	"context"
	"fmt"

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	"github.com/eshe-huli/pier/internal/config"
	"github.com/eshe-huli/pier/internal/dns"
	"github.com/eshe-huli/pier/internal/docker"
	"github.com/eshe-huli/pier/internal/proxy"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Check Pier system health",
	Long:  `Shows the status of all Pier components: Docker, Traefik, DNS, nginx, and network.`,
	RunE:  runStatus,
}

func init() {
	rootCmd.AddCommand(statusCmd)
}

func runStatus(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	ctx := context.Background()

	header := color.New(color.FgCyan, color.Bold)
	fmt.Println()
	header.Printf("  🔩 Pier %s\n", Version)
	fmt.Printf("  %s\n", dim("─────────────────"))
	fmt.Println()

	// Docker
	if docker.IsDockerRunning() {
		fmt.Printf("  Docker:     %s\n", green("✅ running"))
	} else {
		fmt.Printf("  Docker:     %s\n", red("❌ not running"))
	}

	// Traefik
	if proxy.IsTraefikRunning(ctx) {
		routeCount := proxy.GetTraefikRouteCount(cfg.Traefik.Port + 1)
		if routeCount > 0 {
			fmt.Printf("  Traefik:    %s\n", green(fmt.Sprintf("✅ running (%d routes)", routeCount)))
		} else {
			fmt.Printf("  Traefik:    %s\n", green("✅ running"))
		}
	} else {
		fmt.Printf("  Traefik:    %s\n", red("❌ not running"))
	}

	// DNS
	if dns.CheckResolverExists(cfg.TLD) {
		fmt.Printf("  DNS:        %s\n", green(fmt.Sprintf("✅ *.%s resolves", cfg.TLD)))
	} else {
		fmt.Printf("  DNS:        %s\n", red(fmt.Sprintf("❌ resolver not configured")))
	}

	// nginx
	if proxy.IsNginxRunning() {
		fmt.Printf("  nginx:      %s\n", green("✅ running"))
	} else {
		fmt.Printf("  nginx:      %s\n", yellow("⚠️  not detected"))
	}

	// Network
	exists, _ := docker.NetworkExists(ctx, cfg.Network)
	if exists {
		fmt.Printf("  Network:    %s\n", green(fmt.Sprintf("✅ %s", cfg.Network)))
	} else {
		fmt.Printf("  Network:    %s\n", red(fmt.Sprintf("❌ %s not found", cfg.Network)))
	}

	// TLD
	fmt.Printf("  TLD:        %s\n", cyan("."+cfg.TLD))

	// Dashboard
	fmt.Printf("  Dashboard:  %s\n", cyan(fmt.Sprintf("http://traefik.%s", cfg.TLD)))

	fmt.Println()

	return nil
}
