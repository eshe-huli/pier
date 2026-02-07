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

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize Pier (one-time setup)",
	Long: `Sets up everything Pier needs:
  • Docker network 'pier'
  • Traefik reverse proxy container
  • DNS configuration check
  • nginx routing configuration

Some steps require sudo and will print commands for you to run.`,
	RunE: runInit,
}

func init() {
	rootCmd.AddCommand(initCmd)
}

func runInit(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	manualSteps := []string{}
	stepNum := 0

	header := color.New(color.FgCyan, color.Bold)
	header.Println("\n🔩 Pier — Initializing...\n")

	// Step 1: Check Docker
	stepNum++
	step(stepNum, "Checking Docker...")
	if !docker.IsDockerRunning() {
		fail("Docker is not running")
		fmt.Println()
		fmt.Println("    Please start Docker Desktop or OrbStack and try again.")
		return fmt.Errorf("Docker is not running")
	}
	success("Docker is running")

	// Step 2: Check dnsmasq
	stepNum++
	step(stepNum, "Checking dnsmasq...")
	if !dns.CheckDnsmasqInstalled() {
		fail("dnsmasq not found at /opt/homebrew/etc/dnsmasq.conf")
		fmt.Println()
		fmt.Println("    Install dnsmasq:")
		manual("brew install dnsmasq")
		manual("sudo brew services start dnsmasq")
		return fmt.Errorf("dnsmasq not installed")
	}
	success("dnsmasq is installed")

	// Step 3: Load or create config
	stepNum++
	step(stepNum, "Creating configuration...")
	cfg := config.Default()
	if err := config.EnsureDirectories(); err != nil {
		return fmt.Errorf("creating directories: %w", err)
	}
	if err := config.Save(cfg); err != nil {
		return fmt.Errorf("saving config: %w", err)
	}
	success(fmt.Sprintf("Config saved to %s", dim(config.ConfigPath())))

	// Step 4: Create Docker network
	stepNum++
	step(stepNum, fmt.Sprintf("Creating Docker network '%s'...", cfg.Network))
	created, err := docker.EnsureNetwork(ctx, cfg.Network)
	if err != nil {
		fail(fmt.Sprintf("Failed to create network: %s", err))
		return err
	}
	if created {
		success(fmt.Sprintf("Docker network '%s' created", cfg.Network))
	} else {
		success(fmt.Sprintf("Docker network '%s' already exists", cfg.Network))
	}

	// Step 5: Generate Traefik config
	stepNum++
	step(stepNum, "Generating Traefik configuration...")
	if err := proxy.GenerateTraefikConfig(cfg); err != nil {
		return fmt.Errorf("generating Traefik config: %w", err)
	}
	success(fmt.Sprintf("Traefik config at %s", dim(config.TraefikConfigPath())))

	// Step 6: Start Traefik container
	stepNum++
	step(stepNum, "Starting Traefik container...")
	if err := proxy.StartTraefik(ctx, cfg); err != nil {
		fail(fmt.Sprintf("Failed to start Traefik: %s", err))
		return err
	}
	success(fmt.Sprintf("Traefik running on :%d (dashboard :%d)", cfg.Traefik.Port, cfg.Traefik.Port+1))

	// Step 7: Check dnsmasq configuration
	stepNum++
	step(stepNum, fmt.Sprintf("Checking dnsmasq for .%s...", cfg.TLD))
	configured, err := dns.CheckDnsmasqConfigured(cfg.TLD)
	if err != nil {
		warn(fmt.Sprintf("Could not check dnsmasq config: %s", err))
	}
	if configured {
		success(fmt.Sprintf("dnsmasq configured for .%s", cfg.TLD))
	} else {
		warn(fmt.Sprintf("dnsmasq not configured for .%s", cfg.TLD))
		instruction := dns.DnsmasqAddInstruction(cfg.TLD)
		manualSteps = append(manualSteps, instruction)
	}

	// Step 8: Check resolver file
	stepNum++
	step(stepNum, fmt.Sprintf("Checking /etc/resolver/%s...", cfg.TLD))
	if dns.CheckResolverExists(cfg.TLD) {
		success(fmt.Sprintf("/etc/resolver/%s exists", cfg.TLD))
	} else {
		warn(fmt.Sprintf("/etc/resolver/%s not found", cfg.TLD))
		manualSteps = append(manualSteps, dns.ResolverCreateInstruction(cfg.TLD))
	}

	// Step 9: Generate nginx config
	stepNum++
	step(stepNum, "Generating nginx configuration...")
	if err := proxy.GenerateNginxConfig(cfg); err != nil {
		return fmt.Errorf("generating nginx config: %w", err)
	}
	success(fmt.Sprintf("nginx config at %s", dim(config.NginxConfigPath())))

	// Step 10: Check nginx symlink
	stepNum++
	step(stepNum, "Checking nginx symlink...")
	if proxy.IsNginxConfigLinked() {
		success("nginx config is linked")
	} else {
		warn("nginx config not linked")
		manualSteps = append(manualSteps, proxy.NginxSymlinkInstruction())
	}

	// Summary
	fmt.Println()
	divider := color.New(color.FgCyan)
	divider.Println("  ─────────────────────────────────────────")
	fmt.Println()

	if len(manualSteps) > 0 {
		header.Println("  📋 Manual steps needed (requires sudo):\n")
		for i, s := range manualSteps {
			fmt.Printf("  %s %s\n", yellow(fmt.Sprintf("%d.", i+1)), s)
			fmt.Println()
		}
	}

	header.Println("  🎉 Pier initialized!\n")
	fmt.Printf("  TLD:        %s\n", green("."+cfg.TLD))
	fmt.Printf("  Network:    %s\n", green(cfg.Network))
	fmt.Printf("  Traefik:    %s\n", green(fmt.Sprintf(":%d", cfg.Traefik.Port)))
	fmt.Printf("  Dashboard:  %s\n", cyan(fmt.Sprintf("http://traefik.%s", cfg.TLD)))
	fmt.Println()
	fmt.Printf("  %s Add any Docker container to the '%s' network\n", dim("→"), cfg.Network)
	fmt.Printf("  %s and it gets a clean .%s domain automatically.\n", dim(" "), cfg.TLD)
	fmt.Println()

	return nil
}
