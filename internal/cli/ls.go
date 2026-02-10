package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	"github.com/eshe-huli/pier/internal/config"
	"github.com/eshe-huli/pier/internal/docker"
	"github.com/eshe-huli/pier/internal/infra"
	"github.com/eshe-huli/pier/internal/proxy"
)

var lsCmd = &cobra.Command{
	Use:   "ls",
	Short: "List all active services",
	Long:  `Shows all Docker containers on the pier network and bare-metal proxies.`,
	RunE:  runLs,
}

func init() {
	rootCmd.AddCommand(lsCmd)
}

type serviceEntry struct {
	Name   string
	Domain string
	Type   string
	Status string
	Uptime string
}

func runLs(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	ctx := context.Background()
	var entries []serviceEntry

	// Get Docker containers on pier network
	containers, err := docker.ListContainers(ctx, cfg.Network, cfg.TLD)
	if err != nil {
		warn(fmt.Sprintf("Could not list containers: %s", err))
	} else {
		for _, c := range containers {
			// Skip pier-traefik and infra containers (shown separately)
			if c.Name == "pier-traefik" {
				continue
			}
			if infra.IsInfraContainer(c.Name) {
				continue
			}

			status := formatContainerStatus(c.State)
			entries = append(entries, serviceEntry{
				Name:   c.Name,
				Domain: c.Domain,
				Type:   "app",
				Status: status,
				Uptime: c.Status,
			})
		}
	}

	// Get file-based proxies (check if backend is alive)
	proxies, err := proxy.ListFileProxies(cfg.TLD)
	if err != nil {
		warn(fmt.Sprintf("Could not list proxies: %s", err))
	} else {
		for _, p := range proxies {
			status := green("✅ active")
			if !proxy.IsProxyBackendAlive(p.Port) {
				status = yellow("⚠️  stale (port closed)")
			}
			entries = append(entries, serviceEntry{
				Name:   p.Name,
				Domain: p.Domain,
				Type:   "proxy",
				Status: status,
			})
		}
	}

	// Get shared infrastructure services
	infraServices := infra.ListRunning()
	for _, svc := range infraServices {
		entries = append(entries, serviceEntry{
			Name:   svc.Container,
			Domain: "—",
			Type:   "infra",
			Status: green("✅ running"),
		})
	}

	if len(entries) == 0 {
		fmt.Println()
		fmt.Printf("  %s No services found.\n", dim("ℹ"))
		fmt.Println()
		fmt.Printf("  Add a Docker container to the '%s' network, or use:\n", cfg.Network)
		fmt.Printf("    %s\n", cyan("pier proxy <name> <port>"))
		fmt.Println()
		return nil
	}

	// Print table
	fmt.Println()
	header := color.New(color.Bold)
	header.Printf("  %-20s %-28s %-8s %-16s %s\n", "NAME", "DOMAIN", "TYPE", "STATUS", "UPTIME")
	fmt.Printf("  %s\n", dim(strings.Repeat("─", 85)))

	for _, e := range entries {
		fmt.Printf("  %-20s %-28s %-8s %-16s %s\n",
			bold(e.Name),
			cyan(e.Domain),
			dim(e.Type),
			e.Status,
			dim(e.Uptime),
		)
	}

	fmt.Println()
	return nil
}

func formatContainerStatus(state string) string {
	switch state {
	case "running":
		return green("✅ running")
	case "exited":
		return red("⏹ stopped")
	case "created":
		return yellow("⏳ created")
	case "restarting":
		return yellow("🔄 restarting")
	case "paused":
		return yellow("⏸ paused")
	default:
		return dim(state)
	}
}
