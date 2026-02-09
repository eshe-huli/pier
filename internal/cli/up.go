package cli

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/eshe-huli/pier/internal/compose"
	"github.com/eshe-huli/pier/internal/config"
	"github.com/eshe-huli/pier/internal/detect"
	"github.com/eshe-huli/pier/internal/docker"
	"github.com/eshe-huli/pier/internal/infra"
	"github.com/eshe-huli/pier/internal/pierfile"
	"github.com/eshe-huli/pier/internal/runtime"
)

var upDetach bool
var upBuild bool

var upCmd = &cobra.Command{
	Use:   "up",
	Short: "Build and run the current project",
	Long: `Detect services, build the app, and run it on the pier network.

Pier reads from .pier, docker-compose.yml, or auto-detects the framework.
Shared infrastructure (postgres, redis, etc.) is started automatically.

Examples:
  pier up
  pier up --detach
  pier up --build`,
	RunE: runUp,
}

func init() {
	upCmd.Flags().BoolVarP(&upDetach, "detach", "d", true, "Run in background (default true)")
	upCmd.Flags().BoolVar(&upBuild, "build", false, "Force rebuild even if image exists")
	rootCmd.AddCommand(upCmd)
}

func runUp(cmd *cobra.Command, args []string) error {
	dir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting working directory: %w", err)
	}

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	fmt.Println()

	// Step 1: Determine project name
	projectName := filepath.Base(dir)
	var pf *pierfile.Pierfile
	if pierfile.Exists(dir) {
		pf, err = pierfile.Load(dir)
		if err != nil {
			return fmt.Errorf("loading .pier: %w", err)
		}
		if pf.Name != "" {
			projectName = pf.Name
		}
	}

	step(1, fmt.Sprintf("Project: %s", cyan(projectName)))

	// Check for docker-compose project
	cf, composeErr := compose.Parse(dir)
	if composeErr == nil {
		return runUpCompose(dir, projectName, cf, cfg)
	}

	// Step 2: Detect services
	var services []string
	if pf != nil && len(pf.Services) > 0 {
		services = pf.Services
	} else {
		detected, _ := detect.DetectServices(dir)
		for _, d := range detected {
			svc := d.Name
			if d.Version != "" {
				svc += ":" + d.Version
			} else {
				svc += ":" + defaultVersion(d.Name)
			}
			services = append(services, svc)
		}
	}

	// Step 3: Ensure shared infrastructure
	var sharedServices []infra.SharedService
	var dbCreated bool
	if len(services) > 0 {
		step(2, "Starting shared infrastructure...")
		for _, svcSpec := range services {
			parts := strings.SplitN(svcSpec, ":", 2)
			if len(parts) != 2 {
				return fmt.Errorf("invalid service spec '%s' (expected name:version)", svcSpec)
			}
			svcName, svcVersion := parts[0], parts[1]

			fmt.Printf("    → %s:%s ", cyan(svcName), svcVersion)
			if err := infra.EnsureService(svcName, svcVersion); err != nil {
				fmt.Println(red("✗"))
				return fmt.Errorf("starting %s: %w", svcSpec, err)
			}
			fmt.Println(green("✓"))

			svc, _ := infra.ResolveService(svcName, svcVersion)
			if svc != nil {
				sharedServices = append(sharedServices, *svc)
			}

			// Auto-create database for postgres/mysql
			if svcName == "postgres" || svcName == "mysql" {
				if err := infra.CreateDatabase(svcName, svcVersion, projectName); err != nil {
					warn(fmt.Sprintf("Could not create database: %s", err))
				} else {
					dbCreated = true
				}
			}
		}
	}

	// Step 4: Build the app
	port := 0
	if pf != nil && pf.Port > 0 {
		port = pf.Port
	}

	step(3, "Building application...")
	dockerfile := filepath.Join(dir, "Dockerfile")
	if _, err := os.Stat(dockerfile); os.IsNotExist(err) {
		// No Dockerfile — detect framework and generate one
		fw, fwErr := detect.DetectFramework(dir)
		if fwErr != nil {
			return fmt.Errorf("no Dockerfile found and could not detect framework: %w", fwErr)
		}
		if port == 0 {
			port = fw.Port
		}

		tmpl := detect.GenerateDockerfile(fw)
		if tmpl == "" {
			return fmt.Errorf("no Dockerfile template for framework: %s", fw.Name)
		}

		// Write to .pier/Dockerfile (don't touch project root)
		pierDir := filepath.Join(dir, ".pier")
		if err := os.MkdirAll(pierDir, 0755); err != nil {
			return fmt.Errorf("creating .pier directory: %w", err)
		}
		genDockerfile := filepath.Join(pierDir, "Dockerfile")
		if err := os.WriteFile(genDockerfile, []byte(tmpl), 0644); err != nil {
			return fmt.Errorf("writing generated Dockerfile: %w", err)
		}
		info(fmt.Sprintf("Generated Dockerfile for %s → .pier/Dockerfile", cyan(fw.Name)))

		// Build from generated Dockerfile with project dir as context
		buildCmd := exec.Command("docker", "build", "-t", projectName, "-f", genDockerfile, ".")
		buildCmd.Dir = dir
		buildCmd.Stdout = os.Stdout
		buildCmd.Stderr = os.Stderr
		if err := buildCmd.Run(); err != nil {
			return fmt.Errorf("docker build failed: %w", err)
		}
	} else {
		// Dockerfile exists — detect port from framework if not set
		if port == 0 {
			if fw, fwErr := detect.DetectFramework(dir); fwErr == nil {
				port = fw.Port
			}
		}

		buildCmd := exec.Command("docker", "build", "-t", projectName, ".")
		buildCmd.Dir = dir
		buildCmd.Stdout = os.Stdout
		buildCmd.Stderr = os.Stderr
		if err := buildCmd.Run(); err != nil {
			return fmt.Errorf("docker build failed: %w", err)
		}
	}
	success("Image built")

	// Step 5: Stop old container if running
	_ = docker.StopAndRemoveContainer(cmd.Context(), projectName)

	// Step 6: Run the app container
	step(4, fmt.Sprintf("Starting %s...", cyan(projectName)))

	envOverrides := runtime.BuildEnvOverrides(sharedServices)

	dockerArgs := []string{"run", "-d", "--name", projectName, "--network", cfg.Network, "--restart", "unless-stopped"}

	// Env overrides from shared services
	for _, e := range envOverrides {
		dockerArgs = append(dockerArgs, "-e", e)
	}

	// Inject DATABASE_NAME
	dockerArgs = append(dockerArgs, "-e", fmt.Sprintf("DATABASE_NAME=%s", strings.ReplaceAll(projectName, "-", "_")))
	dockerArgs = append(dockerArgs, "-e", fmt.Sprintf("DB_DATABASE=%s", strings.ReplaceAll(projectName, "-", "_")))

	// Pierfile extra env
	if pf != nil {
		for k, v := range pf.Env {
			dockerArgs = append(dockerArgs, "-e", fmt.Sprintf("%s=%s", k, v))
		}
	}

	// Traefik labels
	dockerArgs = append(dockerArgs,
		"-l", "traefik.enable=true",
		"-l", fmt.Sprintf("traefik.http.routers.%s.rule=Host(`%s.%s`)", projectName, projectName, cfg.TLD),
	)
	if port > 0 {
		dockerArgs = append(dockerArgs,
			"-l", fmt.Sprintf("traefik.http.services.%s.loadbalancer.server.port=%d", projectName, port),
		)
	}

	dockerArgs = append(dockerArgs, projectName)

	dockerCmd := exec.Command("docker", dockerArgs...)
	out, err := dockerCmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("docker run failed: %s\n%s", err, string(out))
	}

	// Step 7: Create Traefik route (file proxy as backup)
	if port > 0 {
		_ = createContainerProxy(projectName, port, cfg.TLD)
	}

	// Step 8: Print result
	fmt.Println()
	domain := fmt.Sprintf("%s.%s", projectName, cfg.TLD)
	fmt.Printf("  %s %s\n", green("✅"), bold(domain))
	fmt.Println()

	if len(sharedServices) > 0 {
		fmt.Println("  Services:")
		for _, svc := range sharedServices {
			fmt.Printf("    📦 %s (shared)\n", svc.Container)
		}
		fmt.Println()
	}

	if dbCreated {
		fmt.Printf("  Database: %s (auto-created)\n", projectName)
		fmt.Println()
	}

	return nil
}

// runUpCompose handles docker-compose.yml projects
func runUpCompose(dir, projectName string, cf *compose.ComposeFile, cfg *config.Config) error {
	infraSvcs, appSvcs := compose.SeparateServices(cf)

	// Ensure shared infra
	var sharedServices []infra.SharedService
	var dbCreated bool
	if len(infraSvcs) > 0 {
		step(2, "Starting shared infrastructure (from compose)...")
		for _, is := range infraSvcs {
			version := is.Version
			if version == "" {
				version = defaultVersion(is.Name)
			}

			fmt.Printf("    → %s:%s (replaces compose '%s') ", cyan(is.Name), version, is.ComposeName)
			if err := infra.EnsureService(is.Name, version); err != nil {
				fmt.Println(red("✗"))
				return fmt.Errorf("starting %s: %w", is.Name, err)
			}
			fmt.Println(green("✓"))

			svc, _ := infra.ResolveService(is.Name, version)
			if svc != nil {
				sharedServices = append(sharedServices, *svc)
			}

			if is.Name == "postgres" || is.Name == "mysql" {
				if err := infra.CreateDatabase(is.Name, version, projectName); err != nil {
					warn(fmt.Sprintf("Could not create database: %s", err))
				} else {
					dbCreated = true
				}
			}
		}
	}

	envOverrides := runtime.BuildEnvOverrides(sharedServices)

	// If no app services in compose, fall through to normal build (Dockerfile + .pier)
	if len(appSvcs) == 0 {
		return runUpBuild(dir, projectName, cfg, sharedServices, dbCreated)
	}

	// Build and run app services
	for i, app := range appSvcs {
		appName := projectName
		if len(appSvcs) > 1 {
			appName = app.ComposeName
		}

		step(3+i, fmt.Sprintf("Building %s...", cyan(appName)))

		// Build
		if app.Build != "" {
			buildCtx := app.Build
			if !filepath.IsAbs(buildCtx) {
				buildCtx = filepath.Join(dir, buildCtx)
			}
			buildCmd := exec.Command("docker", "build", "-t", appName, buildCtx)
			buildCmd.Stdout = os.Stdout
			buildCmd.Stderr = os.Stderr
			if err := buildCmd.Run(); err != nil {
				return fmt.Errorf("building %s: %w", appName, err)
			}
		}

		image := appName
		if app.Build == "" && app.Image != "" {
			image = app.Image
		}

		// Stop old
		_ = docker.StopAndRemoveContainer(context.Background(), appName)

		// Determine port
		port := parseFirstPort(app.Ports)

		// Run
		dockerArgs := []string{"run", "-d", "--name", appName, "--network", cfg.Network, "--restart", "unless-stopped"}

		// Compose environment vars first (lower priority)
		for k, v := range app.Environment {
			dockerArgs = append(dockerArgs, "-e", fmt.Sprintf("%s=%s", k, v))
		}

		// Pier env overrides last (higher priority — override compose)
		for _, e := range envOverrides {
			dockerArgs = append(dockerArgs, "-e", e)
		}
		dockerArgs = append(dockerArgs, "-e", fmt.Sprintf("DATABASE_NAME=%s", strings.ReplaceAll(projectName, "-", "_")))
		dockerArgs = append(dockerArgs, "-e", fmt.Sprintf("DB_DATABASE=%s", strings.ReplaceAll(projectName, "-", "_")))

		// Traefik labels
		dockerArgs = append(dockerArgs,
			"-l", "traefik.enable=true",
			"-l", fmt.Sprintf("traefik.http.routers.%s.rule=Host(`%s.%s`)", appName, appName, cfg.TLD),
		)
		if port > 0 {
			dockerArgs = append(dockerArgs,
				"-l", fmt.Sprintf("traefik.http.services.%s.loadbalancer.server.port=%d", appName, port),
			)
		}

		dockerArgs = append(dockerArgs, image)

		dockerCmd := exec.Command("docker", dockerArgs...)
		out, err := dockerCmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("running %s: %s\n%s", appName, err, string(out))
		}

		// File proxy backup
		if port > 0 {
			_ = createContainerProxy(appName, port, cfg.TLD)
		}
	}

	// Print result
	fmt.Println()
	for _, app := range appSvcs {
		appName := projectName
		if len(appSvcs) > 1 {
			appName = app.ComposeName
		}
		domain := fmt.Sprintf("%s.%s", appName, cfg.TLD)
		fmt.Printf("  %s %s\n", green("✅"), bold(domain))
	}
	fmt.Println()

	if len(sharedServices) > 0 {
		fmt.Println("  Services:")
		for _, svc := range sharedServices {
			fmt.Printf("    📦 %s (shared)\n", svc.Container)
		}
		fmt.Println()
	}

	if dbCreated {
		fmt.Printf("  Database: %s (auto-created)\n", projectName)
		fmt.Println()
	}

	return nil
}

// runUpBuild handles the build+run path when compose has no app services
func runUpBuild(dir, projectName string, cfg *config.Config, sharedServices []infra.SharedService, dbCreated bool) error {
	var pf *pierfile.Pierfile
	if pierfile.Exists(dir) {
		pf, _ = pierfile.Load(dir)
	}

	port := 0
	if pf != nil && pf.Port > 0 {
		port = pf.Port
	}

	step(3, "Building application...")
	dockerfile := filepath.Join(dir, "Dockerfile")
	if _, err := os.Stat(dockerfile); os.IsNotExist(err) {
		fw, fwErr := detect.DetectFramework(dir)
		if fwErr != nil {
			return fmt.Errorf("no Dockerfile found and could not detect framework: %w", fwErr)
		}
		if port == 0 {
			port = fw.Port
		}
		tmpl := detect.GenerateDockerfile(fw)
		if tmpl == "" {
			return fmt.Errorf("no Dockerfile template for framework: %s", fw.Name)
		}
		pierDir := filepath.Join(dir, ".pier")
		_ = os.MkdirAll(pierDir, 0755)
		genDockerfile := filepath.Join(pierDir, "Dockerfile")
		_ = os.WriteFile(genDockerfile, []byte(tmpl), 0644)
		info(fmt.Sprintf("Generated Dockerfile for %s → .pier/Dockerfile", cyan(fw.Name)))

		buildCmd := exec.Command("docker", "build", "-t", projectName, "-f", genDockerfile, ".")
		buildCmd.Dir = dir
		buildCmd.Stdout = os.Stdout
		buildCmd.Stderr = os.Stderr
		if err := buildCmd.Run(); err != nil {
			return fmt.Errorf("docker build failed: %w", err)
		}
	} else {
		if port == 0 {
			if fw, fwErr := detect.DetectFramework(dir); fwErr == nil {
				port = fw.Port
			}
		}
		buildCmd := exec.Command("docker", "build", "-t", projectName, ".")
		buildCmd.Dir = dir
		buildCmd.Stdout = os.Stdout
		buildCmd.Stderr = os.Stderr
		if err := buildCmd.Run(); err != nil {
			return fmt.Errorf("docker build failed: %w", err)
		}
	}
	success("Image built")

	_ = docker.StopAndRemoveContainer(context.Background(), projectName)

	step(4, fmt.Sprintf("Starting %s...", cyan(projectName)))
	envOverrides := runtime.BuildEnvOverrides(sharedServices)
	dockerArgs := []string{"run", "-d", "--name", projectName, "--network", cfg.Network, "--restart", "unless-stopped"}
	for _, e := range envOverrides {
		dockerArgs = append(dockerArgs, "-e", e)
	}
	dockerArgs = append(dockerArgs, "-e", fmt.Sprintf("DATABASE_NAME=%s", strings.ReplaceAll(projectName, "-", "_")))
	dockerArgs = append(dockerArgs, "-e", fmt.Sprintf("DB_DATABASE=%s", strings.ReplaceAll(projectName, "-", "_")))
	if pf != nil {
		for k, v := range pf.Env {
			dockerArgs = append(dockerArgs, "-e", fmt.Sprintf("%s=%s", k, v))
		}
	}
	dockerArgs = append(dockerArgs,
		"-l", "traefik.enable=true",
		"-l", fmt.Sprintf("traefik.http.routers.%s.rule=Host(`%s.%s`)", projectName, projectName, cfg.TLD),
	)
	if port > 0 {
		dockerArgs = append(dockerArgs,
			"-l", fmt.Sprintf("traefik.http.services.%s.loadbalancer.server.port=%d", projectName, port),
		)
	}
	dockerArgs = append(dockerArgs, projectName)

	dockerCmd := exec.Command("docker", dockerArgs...)
	out, err := dockerCmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("docker run failed: %s\n%s", err, string(out))
	}

	if port > 0 {
		_ = createContainerProxy(projectName, port, cfg.TLD)
	}

	fmt.Println()
	domain := fmt.Sprintf("%s.%s", projectName, cfg.TLD)
	fmt.Printf("  %s %s\n", green("✅"), bold(domain))
	fmt.Println()
	if len(sharedServices) > 0 {
		fmt.Println("  Services:")
		for _, svc := range sharedServices {
			fmt.Printf("    📦 %s (shared)\n", svc.Container)
		}
		fmt.Println()
	}
	if dbCreated {
		fmt.Printf("  Database: %s (auto-created)\n", projectName)
		fmt.Println()
	}
	return nil
}

func defaultVersion(name string) string {
	defaults := map[string]string{
		"postgres": "16", "redis": "7", "mongo": "7", "mysql": "8",
		"minio": "latest", "kafka": "latest", "rabbitmq": "3", "elasticsearch": "8",
	}
	if v, ok := defaults[name]; ok {
		return v
	}
	return "latest"
}

func parseFirstPort(ports []string) int {
	if len(ports) == 0 {
		return 0
	}
	p := ports[0]
	p = strings.Split(p, "/")[0]
	parts := strings.Split(p, ":")
	var port int
	fmt.Sscanf(parts[len(parts)-1], "%d", &port)
	return port
}
