package cli

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/eshe-huli/pier/internal/config"
	"github.com/eshe-huli/pier/internal/gitignore"
	"github.com/eshe-huli/pier/internal/infra"
	"github.com/eshe-huli/pier/internal/orchestrator"
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
		spec := appSpecFromPlan(runPlan, app)
		spec.RuntimeEnvLast = true
		spec.Route = true
		spec.RegisterType = "docker"
		if app.UseEnvFile && pierEnvFile != "" {
			spec.EnvFile = pierEnvFile
		}

		image := spec.Image
		port := spec.Port
		if app.BuildContext != "" {
			step(3+i, fmt.Sprintf("Building %s...", cyan(app.Name)))
			var err error
			image, port, err = orchestrator.BuildImage(ctx, spec)
			if err != nil {
				return fmt.Errorf("building %s: %w", app.Name, err)
			}
			success("Image built")
		}
		if image == "" {
			image = app.Name
		}

		// Run
		if err := orchestrator.RunContainer(ctx, spec, image, port, cfg, envOverrides); err != nil {
			return fmt.Errorf("running %s: %w", app.Name, err)
		}

		// File proxy backup
		if port > 0 {
			_ = createContainerProxy(app.Name, port, cfg.TLD)
		}
	}

	// Print result
	fmt.Println()
	for _, app := range apps {
		fmt.Printf("  %s %s\n", green("✅"), bold(app.Domain(cfg.TLD)))
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
	spec := appSpecFromPlan(runPlan, app)
	step(3, "Building application...")
	image, port, err := orchestrator.BuildImage(ctx, spec)
	if err != nil {
		return err
	}
	success("Image built")

	step(4, fmt.Sprintf("Starting %s...", cyan(app.Name)))
	envOverrides := planner.RuntimeEnv(projectName, runtime.BuildEnvOverrides(sharedServices))
	spec.Route = true
	spec.RegisterType = "docker"
	if err := orchestrator.RunContainer(ctx, spec, image, port, cfg, envOverrides); err != nil {
		return err
	}

	if port > 0 {
		_ = createContainerProxy(app.Name, port, cfg.TLD)
	}

	domain := spec.Domain(cfg.TLD)
	fmt.Println()
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

func appSpecFromPlan(runPlan *planner.Plan, app planner.AppPlan) orchestrator.AppSpec {
	if app.Name == "" {
		app.Name = runPlan.ProjectName
	}
	if app.BuildContext == "" {
		app.BuildContext = runPlan.Dir
	}
	if app.Image == "" {
		app.Image = app.Name
	}

	genName := ""
	if app.ComposeName != "" {
		genName = app.Name + ".Dockerfile"
	}

	return orchestrator.AppSpec{
		Name:                    app.Name,
		Dir:                     runPlan.Dir,
		Image:                   app.Image,
		BuildCtx:                app.BuildContext,
		Dockerfile:              app.Dockerfile,
		GeneratedDockerfileName: genName,
		Framework:               runPlan.Framework,
		Port:                    app.Port,
		Env:                     app.Env,
		Volumes:                 app.Volumes,
		Entrypoint:              app.Entrypoint,
		Command:                 app.Command,
	}
}
