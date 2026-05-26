// Package orchestrator extracts the shared build→run→route pipeline from the CLI layer.
package orchestrator

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/eshe-huli/pier/internal/config"
	"github.com/eshe-huli/pier/internal/detect"
	"github.com/eshe-huli/pier/internal/docker"
	"github.com/eshe-huli/pier/internal/infra"
	"github.com/eshe-huli/pier/internal/registry"
	"github.com/eshe-huli/pier/internal/runtime"
)

// AppSpec describes what to build and run.
type AppSpec struct {
	Name                    string
	Dir                     string
	Image                   string            // Image tag to build or prebuilt image to run
	Prebuilt                bool              // Skip docker build and use Image directly
	BuildCtx                string            // Build context path
	Dockerfile              string            // Explicit Dockerfile path (empty = auto-detect)
	GeneratedDockerfileName string            // Name under .pier/ when auto-generating
	Framework               *detect.Framework // Framework fallback from the planner
	Port                    int               // Container port
	Env                     map[string]string // Extra env vars
	ExtraEnv                []string          // Ordered extra env vars
	EnvFile                 string            // Optional --env-file path
	RuntimeEnvLast          bool              // Append env overrides after app env
	Volumes                 []string          // Volume mounts
	WorkingDir              string            // Optional container/process working directory
	Entrypoint              interface{}       // Override entrypoint
	Command                 interface{}       // Override CMD
	Route                   bool              // Add Traefik labels
	RegisterType            string            // Registry type, defaults to docker
}

// Result holds the outcome of an orchestrated run.
type Result struct {
	Domain         string
	SharedServices []infra.SharedService
	DBCreated      bool
}

// RuntimeAdapter is the boundary between Pier's plan and a concrete runtime.
type RuntimeAdapter interface {
	Name() string
	BuildImage(ctx context.Context, spec AppSpec) (string, int, error)
	RunApp(ctx context.Context, spec AppSpec, image string, port int, cfg *config.Config, envOverrides []string) error
}

// DockerAdapter executes Pier plans through Docker.
type DockerAdapter struct{}

func (DockerAdapter) Name() string {
	return "docker"
}

// DefaultRuntimeAdapter is used by CLI commands until a command selects another runtime.
var DefaultRuntimeAdapter RuntimeAdapter = DockerAdapter{}

// BuildImage builds an app artifact through the configured runtime adapter.
func BuildImage(ctx context.Context, spec AppSpec) (string, int, error) {
	return DefaultRuntimeAdapter.BuildImage(ctx, spec)
}

// RunContainer executes an app through the configured runtime adapter.
func RunContainer(ctx context.Context, spec AppSpec, image string, port int, cfg *config.Config, envOverrides []string) error {
	return DefaultRuntimeAdapter.RunApp(ctx, spec, image, port, cfg, envOverrides)
}

// BuildImage builds a Docker image for the app. Returns the image name.
func (DockerAdapter) BuildImage(ctx context.Context, spec AppSpec) (string, int, error) {
	imageName := spec.Image
	if imageName == "" {
		imageName = spec.Name
	}
	port := spec.Port

	if spec.Prebuilt {
		return imageName, port, nil
	}

	buildCtx := spec.Dir
	if spec.BuildCtx != "" {
		buildCtx = spec.BuildCtx
		if !filepath.IsAbs(buildCtx) {
			buildCtx = filepath.Join(spec.Dir, buildCtx)
		}
	}

	dockerfile := spec.Dockerfile
	if dockerfile == "" {
		dockerfile = filepath.Join(buildCtx, "Dockerfile")
	}

	// If no Dockerfile, auto-detect framework and generate one
	if _, err := os.Stat(dockerfile); err != nil {
		if !os.IsNotExist(err) {
			return "", 0, fmt.Errorf("checking Dockerfile: %w", err)
		}

		fw, fwErr := detect.DetectFramework(buildCtx)
		if fwErr != nil && spec.Framework != nil && filepath.Clean(buildCtx) == filepath.Clean(spec.Dir) {
			fw = spec.Framework
			fwErr = nil
		}
		if fwErr != nil {
			return "", 0, fmt.Errorf("no Dockerfile found and could not detect framework: %w", fwErr)
		}
		if port == 0 {
			port = fw.Port
		}

		tmpl := detect.GenerateDockerfile(fw)
		if tmpl == "" {
			return "", 0, fmt.Errorf("no Dockerfile template for framework: %s", fw.Name)
		}

		pierDir := filepath.Join(spec.Dir, ".pier")
		if err := os.MkdirAll(pierDir, 0755); err != nil {
			return "", 0, fmt.Errorf("creating .pier directory: %w", err)
		}
		genName := spec.GeneratedDockerfileName
		if genName == "" {
			genName = "Dockerfile"
		}
		dockerfile = filepath.Join(pierDir, genName)
		if err := os.WriteFile(dockerfile, []byte(tmpl), 0644); err != nil {
			return "", 0, fmt.Errorf("writing generated Dockerfile: %w", err)
		}
	} else if port == 0 {
		if fw, fwErr := detect.DetectFramework(buildCtx); fwErr == nil {
			port = fw.Port
		}
	}

	buildArgs := []string{"build", "-t", imageName}
	if dockerfile != filepath.Join(buildCtx, "Dockerfile") {
		buildArgs = append(buildArgs, "-f", dockerfile)
	}
	buildArgs = append(buildArgs, buildCtx)

	cmd := exec.CommandContext(ctx, "docker", buildArgs...)
	cmd.Dir = spec.Dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return "", 0, fmt.Errorf("docker build failed: %w", err)
	}

	return imageName, port, nil
}

// EnsureInfra starts shared infrastructure and returns the services + env overrides.
func EnsureInfra(ctx context.Context, services []string, projectName string) ([]infra.SharedService, []string, bool, error) {
	var shared []infra.SharedService
	var dbCreated bool

	for _, svcSpec := range services {
		parts := strings.SplitN(svcSpec, ":", 2)
		if len(parts) != 2 {
			return nil, nil, false, fmt.Errorf("invalid service spec '%s' (expected name:version)", svcSpec)
		}
		svcName, svcVersion := parts[0], parts[1]

		if err := infra.EnsureService(svcName, svcVersion); err != nil {
			return nil, nil, false, fmt.Errorf("starting %s: %w", svcSpec, err)
		}

		svc, _ := infra.ResolveService(svcName, svcVersion)
		if svc != nil {
			shared = append(shared, *svc)
		}

		if svcName == "postgres" || svcName == "mysql" {
			if err := infra.CreateDatabase(svcName, svcVersion, projectName); err != nil {
				// Non-fatal, just warn
			} else {
				dbCreated = true
			}
		}
	}

	envOverrides := runtime.BuildEnvOverrides(shared)
	dbName := strings.ReplaceAll(projectName, "-", "_")
	envOverrides = append(envOverrides,
		fmt.Sprintf("DATABASE_NAME=%s", dbName),
		fmt.Sprintf("DB_DATABASE=%s", dbName),
	)

	return shared, envOverrides, dbCreated, nil
}

// Domain returns the app domain for the configured Pier TLD.
func (spec AppSpec) Domain(tld string) string {
	tld = strings.TrimPrefix(tld, ".")
	if tld == "" {
		return spec.Name
	}
	return fmt.Sprintf("%s.%s", spec.Name, tld)
}

// DockerRunArgs returns deterministic docker run arguments for an app spec.
func DockerRunArgs(spec AppSpec, image string, port int, cfg *config.Config, envOverrides []string) []string {
	dockerArgs := []string{"run", "-d", "--name", spec.Name, "--network", cfg.Network, "--restart", "unless-stopped"}

	if spec.EnvFile != "" {
		dockerArgs = append(dockerArgs, "--env-file", spec.EnvFile)
	}

	if spec.RuntimeEnvLast {
		dockerArgs = appendEnvMap(dockerArgs, spec.Env)
		dockerArgs = appendExtraEnv(dockerArgs, spec.ExtraEnv)
		dockerArgs = appendEnvList(dockerArgs, envOverrides)
	} else {
		dockerArgs = appendEnvList(dockerArgs, envOverrides)
		dockerArgs = appendEnvMap(dockerArgs, spec.Env)
		dockerArgs = appendExtraEnv(dockerArgs, spec.ExtraEnv)
	}

	if spec.WorkingDir != "" {
		dockerArgs = append(dockerArgs, "-w", spec.WorkingDir)
	}

	if spec.Route {
		dockerArgs = append(dockerArgs,
			"-l", "traefik.enable=true",
			"-l", fmt.Sprintf("traefik.http.routers.%s.rule=Host(`%s`)", spec.Name, spec.Domain(cfg.TLD)),
		)
		if port > 0 {
			dockerArgs = append(dockerArgs,
				"-l", fmt.Sprintf("traefik.http.services.%s.loadbalancer.server.port=%d", spec.Name, port),
			)
		}
	}

	for _, v := range spec.Volumes {
		if strings.Contains(v, ":") {
			parts := strings.SplitN(v, ":", 3)
			hostPath := parts[0]
			if !filepath.IsAbs(hostPath) {
				hostPath = filepath.Join(spec.Dir, hostPath)
			}
			mount := hostPath + ":" + strings.Join(parts[1:], ":")
			dockerArgs = append(dockerArgs, "-v", mount)
		}
	}

	dockerArgs = appendEntrypointOverride(dockerArgs, spec.Entrypoint)
	dockerArgs = append(dockerArgs, image)
	dockerArgs = appendEntrypointArgs(dockerArgs, spec.Entrypoint)
	dockerArgs = appendCommandOverride(dockerArgs, spec.Command)

	return dockerArgs
}

// RunApp stops the old container, starts a new one, and registers it.
func (adapter DockerAdapter) RunApp(ctx context.Context, spec AppSpec, image string, port int, cfg *config.Config, envOverrides []string) error {
	// Stop old container
	if err := docker.StopAndRemoveContainer(ctx, spec.Name); err != nil {
		// Non-fatal, container might not exist
	}

	dockerArgs := DockerRunArgs(spec, image, port, cfg, envOverrides)
	dockerCmd := exec.CommandContext(ctx, "docker", dockerArgs...)
	out, err := dockerCmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("docker run failed: %s\n%s", err, string(out))
	}

	// File proxy backup — caller should handle this via cli createContainerProxy

	// Register project
	registerType := spec.RegisterType
	if registerType == "" {
		registerType = adapter.Name()
	}
	if err := registry.Register(registry.Project{Name: spec.Name, Dir: spec.Dir, Port: port, Type: registerType}); err != nil {
		fmt.Fprintf(os.Stderr, "warning: could not register project: %v\n", err)
	}

	return nil
}

func appendEnvList(args []string, envs []string) []string {
	for _, env := range envs {
		args = append(args, "-e", env)
	}
	return args
}

func appendEnvMap(args []string, env map[string]string) []string {
	keys := make([]string, 0, len(env))
	for key := range env {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	for _, key := range keys {
		args = append(args, "-e", fmt.Sprintf("%s=%s", key, env[key]))
	}
	return args
}

func appendExtraEnv(args []string, envs []string) []string {
	for _, env := range envs {
		args = append(args, "-e", env)
	}
	return args
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
