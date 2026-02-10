package cli

import (
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/eshe-huli/pier/internal/config"
	"github.com/eshe-huli/pier/internal/dashboard"
)

var dashboardCmd = &cobra.Command{
	Use:   "dashboard",
	Short: "Launch the Pier dev hub in your browser",
	Long: `Starts the Pier dashboard server and opens it at pier.<tld> in your browser.

The dashboard shows all running services, their .dock domains, status, 
and type (Docker, linked dev server, or proxy).

Press Ctrl+C to stop the dashboard server.`,
	Aliases: []string{"hub", "dash"},
	RunE:    runDashboard,
}

var dashboardNoOpen bool

func init() {
	dashboardCmd.Flags().BoolVarP(&dashboardNoOpen, "no-open", "n", false, "Don't open browser automatically")
	rootCmd.AddCommand(dashboardCmd)
}

func runDashboard(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	// Start the dashboard server
	port, err := dashboard.Start()
	if err != nil {
		return fmt.Errorf("starting dashboard: %w", err)
	}

	domain := fmt.Sprintf("pier.%s", cfg.TLD)
	url := fmt.Sprintf("http://%s", domain)

	// Register the dashboard as a Traefik route
	if err := registerDashboardRoute(cfg, port); err != nil {
		// Non-fatal — dashboard still works on localhost
		warn(fmt.Sprintf("Could not register %s route: %v", domain, err))
		url = fmt.Sprintf("http://127.0.0.1:%d", port)
	}

	fmt.Println()
	fmt.Printf("  ⚓ %s\n", bold("Pier Dashboard"))
	fmt.Printf("     %s\n", cyan(url))
	fmt.Printf("     Local: %s\n", dim(fmt.Sprintf("http://127.0.0.1:%d", port)))
	fmt.Println()
	info("Press Ctrl+C to stop")
	fmt.Println()

	// Open browser
	if !dashboardNoOpen {
		openBrowser(url)
	}

	// Wait for interrupt
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	fmt.Println()
	info("Dashboard stopped")

	// Clean up route
	removeDashboardRoute(cfg)

	return nil
}

func registerDashboardRoute(cfg *config.Config, port int) error {
	routeFile := filepath.Join(config.TraefikDynamicDir(), "pier-dashboard.yaml")

	content := fmt.Sprintf(`http:
  routers:
    pier-dashboard:
      rule: "Host(`+"`pier.%s`"+`)"
      service: pier-dashboard
      entryPoints:
        - web
  services:
    pier-dashboard:
      loadBalancer:
        servers:
          - url: "http://host.docker.internal:%d"
`, cfg.TLD, port)

	return os.WriteFile(routeFile, []byte(content), 0644)
}

func removeDashboardRoute(cfg *config.Config) {
	routeFile := filepath.Join(config.TraefikDynamicDir(), "pier-dashboard.yaml")
	os.Remove(routeFile)
}

func openBrowser(url string) error {
	var cmd *exec.Cmd

	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "linux":
		cmd = exec.Command("xdg-open", url)
	default:
		return fmt.Errorf("unsupported platform: %s", runtime.GOOS)
	}

	return cmd.Start()
}
