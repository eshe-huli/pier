package cli

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	"github.com/eshe-huli/pier/internal/config"
	"github.com/eshe-huli/pier/internal/dns"
	"github.com/eshe-huli/pier/internal/docker"
	"github.com/eshe-huli/pier/internal/proxy"
)

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Diagnose Pier issues",
	Long:  `Runs comprehensive checks on all Pier components and suggests fixes for any problems.`,
	RunE:  runDoctor,
}

func init() {
	rootCmd.AddCommand(doctorCmd)
}

type checkResult struct {
	Name   string
	OK     bool
	Detail string
	Fix    string
}

func runDoctor(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	ctx := context.Background()

	header := color.New(color.FgCyan, color.Bold)
	fmt.Println()
	header.Println("  🩺 Pier Doctor")
	fmt.Printf("  %s\n\n", dim("Running diagnostics..."))

	checks := []checkResult{}

	// 1. Docker
	{
		c := checkResult{Name: "Docker daemon"}
		if docker.IsDockerRunning() {
			c.OK = true
			c.Detail = "running"
		} else {
			c.Detail = "not running"
			c.Fix = "Start Docker Desktop or OrbStack"
		}
		checks = append(checks, c)
	}

	// 2. Docker network
	{
		c := checkResult{Name: fmt.Sprintf("Docker network '%s'", cfg.Network)}
		exists, _ := docker.NetworkExists(ctx, cfg.Network)
		if exists {
			c.OK = true
			c.Detail = "exists"
		} else {
			c.Detail = "not found"
			c.Fix = "pier init"
		}
		checks = append(checks, c)
	}

	// 3. Traefik container
	{
		c := checkResult{Name: "Traefik container"}
		if proxy.IsTraefikRunning(ctx) {
			c.OK = true
			c.Detail = "running"
		} else {
			c.Detail = "not running"
			c.Fix = "pier init"
		}
		checks = append(checks, c)
	}

	// 4. dnsmasq installed
	{
		c := checkResult{Name: "dnsmasq installed"}
		if dns.CheckDnsmasqInstalled() {
			c.OK = true
			c.Detail = "found"
		} else {
			c.Detail = "not found"
			c.Fix = "brew install dnsmasq && sudo brew services start dnsmasq"
		}
		checks = append(checks, c)
	}

	// 5. dnsmasq configured
	{
		c := checkResult{Name: fmt.Sprintf("dnsmasq .%s entry", cfg.TLD)}
		configured, _ := dns.CheckDnsmasqConfigured(cfg.TLD)
		if configured {
			c.OK = true
			c.Detail = fmt.Sprintf("address=/.%s/127.0.0.1 found", cfg.TLD)
		} else {
			c.Detail = "not configured"
			c.Fix = fmt.Sprintf("Add 'address=/.%s/127.0.0.1' to /opt/homebrew/etc/dnsmasq.conf\nThen: sudo brew services restart dnsmasq", cfg.TLD)
		}
		checks = append(checks, c)
	}

	// 6. Resolver file
	{
		c := checkResult{Name: fmt.Sprintf("/etc/resolver/%s", cfg.TLD)}
		if dns.CheckResolverExists(cfg.TLD) {
			c.OK = true
			c.Detail = "exists"
		} else {
			c.Detail = "not found"
			c.Fix = dns.ResolverCreateInstruction(cfg.TLD)
		}
		checks = append(checks, c)
	}

	// 7. nginx config
	{
		c := checkResult{Name: "nginx config linked"}
		if proxy.IsNginxConfigLinked() {
			c.OK = true
			c.Detail = "symlink present"
		} else {
			c.Detail = "not linked"
			c.Fix = proxy.NginxSymlinkInstruction()
		}
		checks = append(checks, c)
	}

	// 8. nginx running
	{
		c := checkResult{Name: "nginx process"}
		if proxy.IsNginxRunning() {
			c.OK = true
			c.Detail = "running"
		} else {
			c.Detail = "not detected"
			c.Fix = "sudo brew services start nginx"
		}
		checks = append(checks, c)
	}

	// 9. Traefik API reachable
	{
		c := checkResult{Name: "Traefik API"}
		apiURL := fmt.Sprintf("http://127.0.0.1:%d/api/overview", cfg.Traefik.Port+1)
		httpClient := &http.Client{Timeout: 3 * time.Second}
		resp, err := httpClient.Get(apiURL)
		if err == nil && resp.StatusCode == http.StatusOK {
			c.OK = true
			c.Detail = fmt.Sprintf("reachable at :%d", cfg.Traefik.Port+1)
			resp.Body.Close()
		} else {
			c.Detail = "not reachable"
			c.Fix = "pier restart"
		}
		checks = append(checks, c)
	}

	// Print results
	passed := 0
	failed := 0
	for _, c := range checks {
		if c.OK {
			fmt.Printf("  %s  %-30s %s\n", green("✅"), c.Name, dim(c.Detail))
			passed++
		} else {
			fmt.Printf("  %s  %-30s %s\n", red("❌"), c.Name, red(c.Detail))
			if c.Fix != "" {
				fmt.Printf("      %s %s\n", yellow("Fix:"), cyan(c.Fix))
			}
			failed++
		}
	}

	// Summary
	fmt.Println()
	fmt.Printf("  %s\n", dim("─────────────────────────────"))
	if failed == 0 {
		fmt.Printf("  %s All %d checks passed! 🎉\n", green("✅"), passed)
	} else {
		fmt.Printf("  %s passed, %s failed\n",
			green(fmt.Sprintf("%d", passed)),
			red(fmt.Sprintf("%d", failed)),
		)
	}
	fmt.Println()

	return nil
}
