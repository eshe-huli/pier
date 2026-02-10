package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/eshe-huli/pier/internal/config"
	"github.com/eshe-huli/pier/internal/detect"
	"github.com/eshe-huli/pier/internal/pierfile"
	"github.com/eshe-huli/pier/internal/proxy"
)

var linkPort int

var linkCmd = &cobra.Command{
	Use:   "link [name]",
	Short: "Link a local dev server to a .dock domain",
	Long: `Starts the dev server for the current project and creates a .dock proxy.
No Docker needed — runs bare-metal with automatic framework detection.

If a Pierfile exists with a 'command' field, that command is used.
Otherwise, Pier auto-detects the framework and runs the standard dev command.

Examples:
  pier link                    Auto-detect and link
  pier link --port 3001        Link with specific port
  pier link myapp              Link with custom name`,
	Args: cobra.MaximumNArgs(1),
	RunE: runLink,
}

func init() {
	linkCmd.Flags().IntVar(&linkPort, "port", 0, "Override the dev server port")
	rootCmd.AddCommand(linkCmd)
}

func runLink(cmd *cobra.Command, args []string) error {
	dir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting working directory: %w", err)
	}

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	// Resolve project name
	name := filepath.Base(dir)
	var pf *pierfile.Pierfile
	if pierfile.Exists(dir) {
		pf, _ = pierfile.Load(dir)
		if pf != nil && pf.Name != "" {
			name = pf.Name
		}
	}
	if len(args) > 0 {
		name = args[0]
	}

	// Resolve port
	port := linkPort
	if port == 0 && pf != nil && pf.Port > 0 {
		port = pf.Port
	}

	// Resolve command
	devCmd := ""
	if pf != nil {
		// Check if any service entry has a command
		for _, s := range pf.Services {
			if s.Command != "" {
				devCmd = s.Command
				break
			}
		}
	}

	// Auto-detect framework if no command
	if devCmd == "" {
		fw, fwErr := detect.DetectFramework(dir)
		if fwErr != nil && port == 0 {
			return fmt.Errorf("could not detect framework and no port specified. Use --port or add a Pierfile")
		}
		if fw != nil {
			if port == 0 {
				port = fw.Port
			}
			devCmd = detectDevCommand(fw, port)
		}
	}

	if port == 0 {
		return fmt.Errorf("could not determine port. Use --port or set port in Pierfile")
	}

	fmt.Println()
	step(1, fmt.Sprintf("Project: %s", cyan(name)))

	// Create proxy first
	step(2, "Creating route...")
	if proxy.FileProxyExists(name) {
		_ = proxy.RemoveFileProxy(name)
	}
	if err := proxy.CreateFileProxy(name, port, cfg.TLD); err != nil {
		return fmt.Errorf("creating proxy: %w", err)
	}

	domain := fmt.Sprintf("%s.%s", name, cfg.TLD)
	success(fmt.Sprintf("http://%s → localhost:%d", cyan(domain), port))

	if devCmd == "" {
		// No dev command — just proxy, user starts their own server
		fmt.Println()
		info(fmt.Sprintf("Proxy created. Start your dev server on port %d.", port))
		fmt.Println()
		return nil
	}

	// Start dev server in background
	step(3, fmt.Sprintf("Starting dev server: %s", dim(devCmd)))

	// Write PID file to .pier/
	pierDir := filepath.Join(dir, ".pier")
	_ = os.MkdirAll(pierDir, 0755)
	pidFile := filepath.Join(pierDir, "dev.pid")
	logFile := filepath.Join(pierDir, "dev.log")

	// Kill existing if running
	killExistingDev(pidFile)

	log, err := os.Create(logFile)
	if err != nil {
		return fmt.Errorf("creating log file: %w", err)
	}

	parts := strings.Fields(devCmd)
	c := exec.Command(parts[0], parts[1:]...)
	c.Dir = dir
	c.Stdout = log
	c.Stderr = log
	c.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	if err := c.Start(); err != nil {
		log.Close()
		return fmt.Errorf("starting dev server: %w", err)
	}
	log.Close()

	// Save PID
	_ = os.WriteFile(pidFile, []byte(strconv.Itoa(c.Process.Pid)), 0644)

	// Don't wait — detach
	go func() { _ = c.Wait() }()

	success(fmt.Sprintf("Dev server started (PID %d)", c.Process.Pid))
	fmt.Println()
	fmt.Printf("  %s %s\n", green("🌐"), bold(fmt.Sprintf("http://%s", domain)))
	fmt.Printf("  %s %s\n", dim("Logs:"), dim(logFile))
	fmt.Println()

	return nil
}

func detectDevCommand(fw *detect.Framework, port int) string {
	switch fw.Name {
	case "nextjs":
		return fmt.Sprintf("npx next dev -p %d", port)
	case "nuxt":
		return fmt.Sprintf("npx nuxi dev --port %d", port)
	case "nestjs":
		return "npm run start:dev"
	case "express", "fastify":
		return "npm run dev"
	case "django":
		return fmt.Sprintf("python manage.py runserver 0.0.0.0:%d", port)
	case "fastapi":
		return fmt.Sprintf("uvicorn main:app --reload --port %d", port)
	case "flask":
		return fmt.Sprintf("flask run --port %d", port)
	case "go":
		return "go run ."
	case "rails":
		return fmt.Sprintf("rails server -p %d", port)
	case "phoenix":
		return "mix phx.server"
	case "laravel":
		return fmt.Sprintf("php artisan serve --port=%d", port)
	case "spring-boot":
		return "./mvnw spring-boot:run"
	default:
		return ""
	}
}

func killExistingDev(pidFile string) {
	data, err := os.ReadFile(pidFile)
	if err != nil {
		return
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return
	}
	// Kill the process group
	_ = syscall.Kill(-pid, syscall.SIGTERM)
	_ = os.Remove(pidFile)
}
