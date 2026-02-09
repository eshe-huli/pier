package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/eshe-huli/pier/internal/config"
	"github.com/eshe-huli/pier/internal/docker"
	"github.com/eshe-huli/pier/internal/infra"
	"github.com/eshe-huli/pier/internal/pierfile"
	"github.com/eshe-huli/pier/internal/proxy"
)

var downAll bool

var downCmd = &cobra.Command{
	Use:   "down [name]",
	Short: "Stop a project or all Pier infrastructure",
	Long: `Stop a project container and remove its Traefik route.

Examples:
  pier down              Stop the project in the current directory
  pier down my-project   Stop a specific project
  pier down --all        Stop everything (projects + infra + Traefik)`,
	Args: cobra.MaximumNArgs(1),
	RunE: runDown,
}

func init() {
	downCmd.Flags().BoolVar(&downAll, "all", false, "Stop everything including shared infrastructure")
	rootCmd.AddCommand(downCmd)
}

func runDown(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	fmt.Println()

	if downAll {
		return runDownAll(ctx)
	}

	// Determine which project to stop
	var name string
	if len(args) > 0 {
		name = args[0]
	} else {
		// Use current directory
		dir, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("getting working directory: %w", err)
		}
		name = filepath.Base(dir)
		if pierfile.Exists(dir) {
			pf, err := pierfile.Load(dir)
			if err == nil && pf.Name != "" {
				name = pf.Name
			}
		}
	}

	step(1, fmt.Sprintf("Stopping %s...", cyan(name)))

	// Stop the container
	if docker.IsContainerRunning(ctx, name) {
		if err := docker.StopAndRemoveContainer(ctx, name); err != nil {
			fail(fmt.Sprintf("Failed to stop container: %s", err))
			return err
		}
		success(fmt.Sprintf("Container %s stopped", bold(name)))
	} else {
		info(fmt.Sprintf("Container %s is not running", name))
	}

	// Remove Traefik route
	if proxy.FileProxyExists(name) {
		if err := proxy.RemoveFileProxy(name); err != nil {
			warn(fmt.Sprintf("Could not remove route: %s", err))
		} else {
			success("Route removed")
		}
	}

	fmt.Println()
	return nil
}

func runDownAll(ctx context.Context) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	// List all containers on pier network
	containers, err := docker.ListContainers(ctx, cfg.Network, cfg.TLD)
	if err != nil {
		return fmt.Errorf("listing containers: %w", err)
	}

	// Stop project containers (non-infra, non-traefik)
	step(1, "Stopping project containers...")
	for _, c := range containers {
		if c.Name == "pier-traefik" || infra.IsInfraContainer(c.Name) {
			continue
		}
		if c.State != "running" {
			continue
		}
		fmt.Printf("    → %s ", cyan(c.Name))
		if err := docker.StopAndRemoveContainer(ctx, c.Name); err != nil {
			fmt.Println(red("✗"))
		} else {
			fmt.Println(green("✓"))
		}
		// Remove route
		if proxy.FileProxyExists(c.Name) {
			_ = proxy.RemoveFileProxy(c.Name)
		}
	}

	// Stop infra containers
	step(2, "Stopping shared infrastructure...")
	for _, c := range containers {
		if !infra.IsInfraContainer(c.Name) {
			continue
		}
		if c.State != "running" {
			continue
		}
		fmt.Printf("    → %s ", cyan(c.Name))
		if err := docker.StopAndRemoveContainer(ctx, c.Name); err != nil {
			fmt.Println(red("✗"))
		} else {
			fmt.Println(green("✓"))
		}
	}

	// Stop Traefik
	step(3, "Stopping Traefik...")
	if proxy.IsTraefikRunning(ctx) {
		if err := proxy.StopTraefik(ctx); err != nil {
			fail(fmt.Sprintf("Failed to stop Traefik: %s", err))
		} else {
			success("Traefik stopped")
		}
	} else {
		info("Traefik is not running")
	}

	// Clean up all dynamic route files
	dynamicDir := config.TraefikDynamicDir()
	entries, err := os.ReadDir(dynamicDir)
	if err == nil {
		for _, e := range entries {
			if strings.HasSuffix(e.Name(), ".yaml") {
				_ = os.Remove(filepath.Join(dynamicDir, e.Name()))
			}
		}
	}

	fmt.Println()
	success("Everything stopped")
	fmt.Println()
	return nil
}
