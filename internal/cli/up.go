package cli

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/eshe-huli/pier/internal/config"
	"github.com/eshe-huli/pier/internal/detect"
	"github.com/eshe-huli/pier/internal/docker"
	"github.com/eshe-huli/pier/internal/gitignore"
	"github.com/eshe-huli/pier/internal/infra"
	"github.com/eshe-huli/pier/internal/planner"
	"github.com/eshe-huli/pier/internal/registry"
	"github.com/eshe-huli/pier/internal/runtime"
)

var upDetach bool
var upBuild bool

var upCmd = &cobra.Command{
	Use:   "up",
	Short: "Build and run the current project",
	Long: `Detect services, build the app, and run it on the pier network.

Pier reads from Pierfile, docker-compose.yml, or auto-detects the framework.
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

	// Ensure .pier/ is in .gitignore
	_ = gitignore.EnsurePierIgnored(dir)

	fmt.Println()

	runPlan, err := planner.PlanProject(dir)
	if err != nil {
		return fmt.Errorf("planning project: %w", err)
	}
	savePlanManifest(runPlan)
	projectName := runPlan.ProjectName

	step(1, fmt.Sprintf("Project: %s", cyan(projectName)))

	// Check for docker-compose project
	if runPlan.ComposeFile != nil {
		return runUpCompose(cmd.Context(), dir, runPlan, cfg)
	}

	// Step 2: Detect services
	services := runPlan.ServiceSpecs

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

	return runUpBuild(cmd.Context(), runPlan, cfg, sharedServices, dbCreated)
}

// runUpCompose handles docker-compose.yml projects
func runUpCompose(ctx context.Context, dir string, runPlan *planner.Plan, cfg *config.Config) error {
	projectName := runPlan.ProjectName
	infraSvcs := runPlan.InfraServices
	apps := runPlan.Apps

	// Ensure shared infra
	var sharedServices []infra.SharedService
	var dbCreated bool
	if len(infraSvcs) > 0 {
		step(2, "Starting shared infrastructure (from compose)...")
		for _, is := range infraSvcs {
			version := is.Version
			if version == "" {
				version = planner.DefaultVersion(is.Name)
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

	// Generate .pier/env from project .env + pier overrides
	envOverrides := planner.RuntimeEnv(projectName, runtime.BuildEnvOverrides(sharedServices))
	pierEnvFile, envErr := runtime.GenerateEnvFile(dir, envOverrides)
	if envErr != nil {
		warn(fmt.Sprintf("Could not generate .pier/env: %s", envErr))
	}

	// If no app services in compose, fall through to normal build (Dockerfile + .pier)
	if len(runPlan.AppServices) == 0 {
		return runUpBuild(ctx, runPlan, cfg, sharedServices, dbCreated)
	}

	// Build and run app services
	for i, app := range apps {
		if app.BuildContext != "" {
			step(3+i, fmt.Sprintf("Building %s...", cyan(app.Name)))
			if err := buildAppImage(runPlan, app); err != nil {
				return fmt.Errorf("building %s: %w", app.Name, err)
			}
			success("Image built")
		}

		image := app.Image
		if image == "" {
			image = app.Name
		}

		// Stop old
		_ = docker.StopAndRemoveContainer(ctx, app.Name)

		// Run
		dockerArgs := []string{"run", "-d", "--name", app.Name, "--network", cfg.Network, "--restart", "unless-stopped"}

		// For built apps: use .pier/env file (clean, no baked-in env)
		// For sidecars (image-only): pass env vars individually
		if app.UseEnvFile && pierEnvFile != "" {
			dockerArgs = append(dockerArgs, "--env-file", pierEnvFile)
		}

		// Compose environment vars (lower priority)
		for k, v := range app.Env {
			dockerArgs = append(dockerArgs, "-e", fmt.Sprintf("%s=%s", k, v))
		}

		// Pier env overrides (highest priority)
		for _, e := range envOverrides {
			dockerArgs = append(dockerArgs, "-e", e)
		}

		// Traefik labels
		dockerArgs = append(dockerArgs,
			"-l", "traefik.enable=true",
			"-l", fmt.Sprintf("traefik.http.routers.%s.rule=Host(`%s.%s`)", app.Name, app.Name, cfg.TLD),
		)
		if app.Port > 0 {
			dockerArgs = append(dockerArgs,
				"-l", fmt.Sprintf("traefik.http.services.%s.loadbalancer.server.port=%d", app.Name, app.Port),
			)
		}

		// Volumes are resolved by the planner.
		for _, v := range app.Volumes {
			dockerArgs = append(dockerArgs, "-v", v)
		}

		dockerArgs = appendEntrypointOverride(dockerArgs, app.Entrypoint)

		dockerArgs = append(dockerArgs, image)
		dockerArgs = appendEntrypointArgs(dockerArgs, app.Entrypoint)
		dockerArgs = appendCommandOverride(dockerArgs, app.Command)

		dockerCmd := exec.Command("docker", dockerArgs...)
		out, err := dockerCmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("running %s: %s\n%s", app.Name, err, string(out))
		}

		// File proxy backup
		if app.Port > 0 {
			_ = createContainerProxy(app.Name, app.Port, cfg.TLD)
		}
	}

	// Print result
	fmt.Println()
	for _, app := range apps {
		domain := fmt.Sprintf("%s.%s", app.Name, cfg.TLD)
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
	// Register in project registry
	_ = registry.Register(registry.Project{Name: projectName, Dir: dir, Type: "docker"})

	return nil
}

// runUpBuild handles the default build+run path from a planner app plan.
func runUpBuild(ctx context.Context, runPlan *planner.Plan, cfg *config.Config, sharedServices []infra.SharedService, dbCreated bool) error {
	if len(runPlan.Apps) == 0 {
		return fmt.Errorf("no app plan available for %s", runPlan.ProjectName)
	}

	app := runPlan.Apps[0]
	if app.Name == "" {
		app.Name = runPlan.ProjectName
	}
	if app.BuildContext == "" {
		app.BuildContext = runPlan.Dir
	}
	if app.Image == "" {
		app.Image = app.Name
	}

	projectName := runPlan.ProjectName
	step(3, "Building application...")
	if err := buildAppImage(runPlan, app); err != nil {
		return err
	}
	success("Image built")

	_ = docker.StopAndRemoveContainer(ctx, app.Name)

	step(4, fmt.Sprintf("Starting %s...", cyan(app.Name)))
	envOverrides := planner.RuntimeEnv(projectName, runtime.BuildEnvOverrides(sharedServices))
	dockerArgs := []string{"run", "-d", "--name", app.Name, "--network", cfg.Network, "--restart", "unless-stopped"}
	for _, e := range envOverrides {
		dockerArgs = append(dockerArgs, "-e", e)
	}
	for k, v := range app.Env {
		dockerArgs = append(dockerArgs, "-e", fmt.Sprintf("%s=%s", k, v))
	}
	dockerArgs = append(dockerArgs,
		"-l", "traefik.enable=true",
		"-l", fmt.Sprintf("traefik.http.routers.%s.rule=Host(`%s.%s`)", app.Name, app.Name, cfg.TLD),
	)
	if app.Port > 0 {
		dockerArgs = append(dockerArgs,
			"-l", fmt.Sprintf("traefik.http.services.%s.loadbalancer.server.port=%d", app.Name, app.Port),
		)
	}
	dockerArgs = append(dockerArgs, app.Image)

	dockerCmd := exec.Command("docker", dockerArgs...)
	out, err := dockerCmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("docker run failed: %s\n%s", err, string(out))
	}

	if app.Port > 0 {
		_ = createContainerProxy(app.Name, app.Port, cfg.TLD)
	}

	fmt.Println()
	domain := fmt.Sprintf("%s.%s", app.Name, cfg.TLD)
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
	// Register in project registry
	_ = registry.Register(registry.Project{Name: projectName, Dir: runPlan.Dir, Type: "docker"})

	return nil
}

func savePlanManifest(runPlan *planner.Plan) {
	if err := planner.SaveManifest(runPlan); err != nil {
		warn(fmt.Sprintf("Could not write .pier/manifest.yaml: %s", err))
	}
}

func buildAppImage(runPlan *planner.Plan, app planner.AppPlan) error {
	buildContext := app.BuildContext
	if buildContext == "" {
		buildContext = runPlan.Dir
	}

	dockerfile := app.Dockerfile
	if dockerfile == "" {
		defaultDockerfile := filepath.Join(buildContext, "Dockerfile")
		if _, err := os.Stat(defaultDockerfile); err != nil {
			if !os.IsNotExist(err) {
				return fmt.Errorf("checking Dockerfile: %w", err)
			}

			fw, fwErr := detect.DetectFramework(buildContext)
			if fwErr != nil && runPlan.Framework != nil && filepath.Clean(buildContext) == filepath.Clean(runPlan.Dir) {
				fw = runPlan.Framework
				fwErr = nil
			}
			if fwErr != nil {
				return fmt.Errorf("no Dockerfile found and could not detect framework: %w", fwErr)
			}

			tmpl := detect.GenerateDockerfile(fw)
			if tmpl == "" {
				return fmt.Errorf("no Dockerfile template for framework: %s", fw.Name)
			}

			pierDir := filepath.Join(runPlan.Dir, ".pier")
			if err := os.MkdirAll(pierDir, 0755); err != nil {
				return fmt.Errorf("creating .pier directory: %w", err)
			}

			genName := "Dockerfile"
			if app.ComposeName != "" {
				genName = app.Name + ".Dockerfile"
			}
			dockerfile = filepath.Join(pierDir, genName)
			if err := os.WriteFile(dockerfile, []byte(tmpl), 0644); err != nil {
				return fmt.Errorf("writing generated Dockerfile: %w", err)
			}
			info(fmt.Sprintf("Generated Dockerfile for %s → %s", cyan(fw.Name), filepath.ToSlash(filepath.Join(".pier", genName))))
		}
	}

	buildArgs := []string{"build", "-t", app.Name}
	if dockerfile != "" {
		buildArgs = append(buildArgs, "-f", dockerfile)
	}
	buildArgs = append(buildArgs, buildContext)

	buildCmd := exec.Command("docker", buildArgs...)
	buildCmd.Dir = runPlan.Dir
	buildCmd.Stdout = os.Stdout
	buildCmd.Stderr = os.Stderr
	if err := buildCmd.Run(); err != nil {
		return fmt.Errorf("docker build failed: %w", err)
	}

	return nil
}

func appendEntrypointOverride(args []string, entrypoint interface{}) []string {
	if entrypoint == nil {
		return args
	}

	switch ep := entrypoint.(type) {
	case string:
		return append(args, "--entrypoint", ep)
	case []interface{}:
		if len(ep) > 0 {
			return append(args, "--entrypoint", fmt.Sprintf("%v", ep[0]))
		}
	}
	return args
}

func appendEntrypointArgs(args []string, entrypoint interface{}) []string {
	ep, ok := entrypoint.([]interface{})
	if !ok || len(ep) <= 1 {
		return args
	}

	for _, item := range ep[1:] {
		args = append(args, fmt.Sprintf("%v", item))
	}
	return args
}

func appendCommandOverride(args []string, command interface{}) []string {
	if command == nil {
		return args
	}

	switch cmd := command.(type) {
	case string:
		args = append(args, cmd)
	case []interface{}:
		for _, item := range cmd {
			args = append(args, fmt.Sprintf("%v", item))
		}
	}
	return args
}
