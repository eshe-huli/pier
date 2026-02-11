package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	"github.com/eshe-huli/pier/internal/config"
	"github.com/eshe-huli/pier/internal/detect"
	"github.com/eshe-huli/pier/internal/gitignore"
	"github.com/eshe-huli/pier/internal/dns"
	"github.com/eshe-huli/pier/internal/docker"
	"github.com/eshe-huli/pier/internal/pierfile"
	"github.com/eshe-huli/pier/internal/proxy"
)

var (
	initSystemFlag bool
	initForceFlag  bool
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize Pier or a project",
	Long: `Smart init: detects context and does the right thing.

If Pier is not set up yet (no Traefik, no pier network):
  → System init: Docker network, Traefik, DNS configuration

If Pier is already running AND you're in a project directory:
  → Project init: detect framework, generate Pierfile + Dockerfile

Flags:
  --system    Force system init even inside a project
  --force     Overwrite existing Dockerfile during project init`,
	RunE: runInit,
}

func init() {
	initCmd.Flags().BoolVar(&initSystemFlag, "system", false, "Force system initialization")
	initCmd.Flags().BoolVar(&initForceFlag, "force", false, "Overwrite existing Dockerfile")
	rootCmd.AddCommand(initCmd)
}

// isProjectDir checks if the current directory looks like a project
func isProjectDir(dir string) bool {
	markers := []string{
		"package.json", "go.mod", "Cargo.toml", "requirements.txt",
		"Pipfile", "pyproject.toml", "composer.json", "Gemfile",
		"mix.exs", "pom.xml", "build.gradle", "docker-compose.yml",
		"docker-compose.yaml", "Dockerfile",
	}
	for _, m := range markers {
		if _, err := os.Stat(filepath.Join(dir, m)); err == nil {
			return true
		}
	}
	return false
}

// isPierSystemReady checks if Traefik is running and pier network exists
func isPierSystemReady() bool {
	ctx := context.Background()
	if !docker.IsDockerRunning() {
		return false
	}
	if !proxy.IsTraefikRunning(ctx) {
		return false
	}
	exists, err := docker.NetworkExists(ctx, "pier")
	return err == nil && exists
}

func runInit(cmd *cobra.Command, args []string) error {
	if initSystemFlag {
		return runSystemInit(cmd, args)
	}

	// Smart detection
	dir, _ := os.Getwd()
	systemReady := isPierSystemReady()

	if systemReady && isProjectDir(dir) {
		return runProjectInitLogic(dir)
	}

	// Default to system init
	return runSystemInit(cmd, args)
}

func runProjectInitLogic(dir string) error {
	header := color.New(color.FgCyan, color.Bold)
	header.Println("\n🔩 Pier — Project Init\n")

	// 1. Detect framework
	fw, fwErr := detect.DetectFramework(dir)
	if fwErr == nil {
		fmt.Printf("  🔍 Detected: %s (%s)\n", bold(initTitleCase(fw.Name)), fw.Language)
	} else {
		fmt.Printf("  🔍 No framework detected\n")
	}

	// 2. Detect services
	services, _ := detect.DetectServices(dir)
	if len(services) > 0 {
		names := make([]string, len(services))
		for i, s := range services {
			names[i] = s.String()
		}
		fmt.Printf("  📦 Services: %s\n", strings.Join(names, ", "))
	} else {
		fmt.Printf("  📦 No services detected\n")
	}

	// 3. Generate Dockerfile if needed (into .pier/ — never touch project root)
	pierDir := filepath.Join(dir, ".pier")
	_ = os.MkdirAll(pierDir, 0755)
	projectDockerfile := filepath.Join(dir, "Dockerfile")
	pierDockerfile := filepath.Join(pierDir, "Dockerfile")
	if fw != nil {
		// Only generate if project doesn't have its own Dockerfile
		if _, err := os.Stat(projectDockerfile); os.IsNotExist(err) {
			if _, err := os.Stat(pierDockerfile); os.IsNotExist(err) || initForceFlag {
				content := detect.GenerateDockerfile(fw)
				if content != "" {
					if err := os.WriteFile(pierDockerfile, []byte(content), 0644); err != nil {
						return fmt.Errorf("writing Dockerfile: %w", err)
					}
					fmt.Printf("  📄 Generated: .pier/Dockerfile\n")
				}
			} else {
				fmt.Printf("  📄 .pier/Dockerfile already exists %s\n", dim("(use --force to overwrite)"))
			}
		} else {
			fmt.Printf("  📄 Project Dockerfile exists %s\n", dim("(pier won't override)"))
		}
	}

	// 4. Generate .pier file (no port — pier handles routing automatically)
	projectName := filepath.Base(dir)
	pf := &pierfile.Pierfile{
		Name:  projectName,
		Build: true,
	}
	// Port is NOT stored in config — pier detects it from framework
	// and injects PORT env automatically. Users never touch ports.
	for _, s := range services {
		pf.Services = append(pf.Services, pierfile.ServiceEntry{
			Name:    s.Name,
			Version: s.Version,
		})
	}
	if err := pierfile.Save(dir, pf); err != nil {
		return fmt.Errorf("writing .pier: %w", err)
	}
	fmt.Printf("  📄 Generated: app.pier\n")

	// Ensure .pier/ is in .gitignore
	if err := gitignore.EnsurePierIgnored(dir); err != nil {
		fmt.Printf("  ⚠️  Could not update .gitignore: %s\n", err)
	} else {
		fmt.Printf("  📄 Updated: .gitignore (.pier/ added)\n")
	}

	// Summary
	fmt.Println()
	color.New(color.FgGreen, color.Bold).Println("  Ready! Run `pier up` to start.")
	fmt.Println()
	return nil
}

func runSystemInit(cmd *cobra.Command, args []string) error {
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

func initTitleCase(s string) string {
	if s == "" {
		return s
	}
	r := []rune(s)
	r[0] = unicode.ToUpper(r[0])
	return string(r)
}
