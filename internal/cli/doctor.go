package cli

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	"github.com/eshe-huli/pier/internal/config"
	"github.com/eshe-huli/pier/internal/dns"
	"github.com/eshe-huli/pier/internal/docker"
	"github.com/eshe-huli/pier/internal/orchestrator"
	"github.com/eshe-huli/pier/internal/planner"
	"github.com/eshe-huli/pier/internal/proxy"
)

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Diagnose Pier issues",
	Long:  `Runs comprehensive checks on all Pier components and suggests fixes for any problems.`,
	RunE:  runDoctor,
}

var doctorProcessCmd = &cobra.Command{
	Use:   "process [dir]",
	Short: "Diagnose local process runtime dependencies",
	Long:  `Checks the inferred process-mode command and host executable dependencies for a Pier project.`,
	Args:  cobra.MaximumNArgs(1),
	RunE:  runDoctorProcess,
}

func init() {
	rootCmd.AddCommand(doctorCmd)
	doctorCmd.AddCommand(doctorProcessCmd)
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
			if proxy.IsTraefikComposeManaged(ctx) {
				c.OK = true
				c.Detail = "running"
			} else {
				c.Detail = "legacy container"
				c.Fix = "pier restart"
			}
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

	if cfg.Nginx.Managed {
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
	} else {
		c := checkResult{Name: "HTTP edge mode", OK: true, Detail: proxy.EdgeDescription(cfg)}
		checks = append(checks, c)
	}

	// 9. Traefik API reachable
	{
		c := checkResult{Name: "Traefik API"}
		apiURL := fmt.Sprintf("http://127.0.0.1:%d/api/overview", proxy.DashboardHostPort(cfg))
		httpClient := &http.Client{Timeout: 3 * time.Second}
		resp, err := httpClient.Get(apiURL)
		if err == nil && resp.StatusCode == http.StatusOK {
			c.OK = true
			c.Detail = fmt.Sprintf("reachable at :%d", proxy.DashboardHostPort(cfg))
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

func runDoctorProcess(cmd *cobra.Command, args []string) error {
	dir := "."
	if len(args) > 0 {
		dir = args[0]
	}

	absDir, err := filepath.Abs(dir)
	if err != nil {
		return fmt.Errorf("resolving project directory: %w", err)
	}

	header := color.New(color.FgCyan, color.Bold)
	fmt.Println()
	header.Println("  🩺 Pier Process Doctor")
	fmt.Printf("  %s\n\n", dim(absDir))

	checks, err := processDoctorChecks(absDir, exec.LookPath, os.Stat)
	if err != nil {
		return err
	}
	printDoctorChecks(checks)

	return nil
}

type lookPathFunc func(string) (string, error)
type statFunc func(string) (os.FileInfo, error)

func processDoctorChecks(dir string, lookPath lookPathFunc, stat statFunc) ([]checkResult, error) {
	runPlan, err := planner.PlanProject(dir)
	if err != nil {
		return nil, fmt.Errorf("planning project: %w", err)
	}

	checks := []checkResult{}
	adapter := orchestrator.LocalProcessAdapter{}
	for _, app := range runPlan.Apps {
		spec := appSpecFromPlan(runPlan, app)
		_, port, err := adapter.BuildImage(context.Background(), spec)
		if err != nil {
			checks = append(checks, checkResult{
				Name:   fmt.Sprintf("%s process command", app.Name),
				Detail: "not inferred",
				Fix:    "Set a Pierfile command, add command: to docker-compose.yml, or use --runtime docker",
			})
			continue
		}

		command := orchestrator.LocalProcessCommand(spec, port)
		if command == "" {
			checks = append(checks, checkResult{
				Name:   fmt.Sprintf("%s process command", app.Name),
				Detail: "not inferred",
				Fix:    "Set a Pierfile command, add command: to docker-compose.yml, or use --runtime docker",
			})
			continue
		}

		checks = append(checks, checkResult{
			Name:   fmt.Sprintf("%s process command", app.Name),
			OK:     true,
			Detail: command,
		})
		checks = append(checks, processCommandDependency(command, processBuildDir(spec), lookPath, stat))
	}

	return checks, nil
}

func processCommandDependency(command, buildDir string, lookPath lookPathFunc, stat statFunc) checkResult {
	executable := commandExecutable(command)
	result := checkResult{
		Name: fmt.Sprintf("process dependency '%s'", executable),
	}
	if executable == "" {
		result.Detail = "not found"
		result.Fix = "Set a concrete command executable in Pierfile or docker-compose.yml"
		return result
	}

	if isProjectLocalExecutable(executable) {
		path := executable
		if !filepath.IsAbs(path) {
			path = filepath.Join(buildDir, executable)
		}
		info, err := stat(path)
		if err != nil {
			result.Detail = "not found"
			result.Fix = fmt.Sprintf("Add %s under %s or change the process command", executable, buildDir)
			return result
		}
		if info.Mode()&0111 == 0 {
			result.Detail = "not executable"
			result.Fix = fmt.Sprintf("Run chmod +x %s or change the process command", path)
			return result
		}
		result.OK = true
		result.Detail = path
		return result
	}

	path, err := lookPath(executable)
	if err != nil {
		result.Detail = "not found"
		result.Fix = fmt.Sprintf("Install %s or change the process command", executable)
		return result
	}
	result.OK = true
	result.Detail = path
	return result
}

func commandExecutable(command string) string {
	for _, field := range strings.Fields(command) {
		token := strings.Trim(field, `"'`)
		if strings.Contains(token, "=") && !strings.HasPrefix(token, "./") && !strings.HasPrefix(token, "../") {
			continue
		}
		return token
	}
	return ""
}

func isProjectLocalExecutable(executable string) bool {
	return filepath.IsAbs(executable) || strings.HasPrefix(executable, "./") || strings.HasPrefix(executable, "../")
}

func processBuildDir(spec orchestrator.AppSpec) string {
	buildDir := spec.Dir
	if spec.BuildCtx != "" {
		buildDir = spec.BuildCtx
		if !filepath.IsAbs(buildDir) {
			buildDir = filepath.Join(spec.Dir, buildDir)
		}
	}
	if spec.WorkingDir != "" && !filepath.IsAbs(spec.WorkingDir) {
		buildDir = filepath.Join(buildDir, spec.WorkingDir)
	}
	return filepath.Clean(buildDir)
}

func printDoctorChecks(checks []checkResult) {
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

	fmt.Println()
	fmt.Printf("  %s\n", dim("─────────────────────────────"))
	if failed == 0 {
		fmt.Printf("  %s All %d checks passed!\n", green("✅"), passed)
	} else {
		fmt.Printf("  %s passed, %s failed\n",
			green(fmt.Sprintf("%d", passed)),
			red(fmt.Sprintf("%d", failed)),
		)
	}
	fmt.Println()
}
