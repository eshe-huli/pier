// Package orchestrator extracts the shared build→run→route pipeline from the CLI layer.
package orchestrator

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
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
	Name        string
	Dir         string
	Image       string            // Pre-built image (skip build if set)
	BuildCtx    string            // Build context path
	Dockerfile  string            // Explicit Dockerfile path (empty = auto-detect)
	Port        int               // Container port
	Env         map[string]string // Extra env vars
	Volumes     []string          // Volume mounts
	Entrypoint  interface{}       // Override entrypoint
	Command     interface{}       // Override CMD
}

// Result holds the outcome of an orchestrated run.
type Result struct {
	Domain         string
	SharedServices []infra.SharedService
	DBCreated      bool
}

// BuildImage builds a Docker image for the app. Returns the image name.
func BuildImage(ctx context.Context, spec AppSpec) (string, int, error) {
	imageName := spec.Name
	port := spec.Port

	if spec.Image != "" {
		return spec.Image, port, nil
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
	if _, err := os.Stat(dockerfile); os.IsNotExist(err) {
		fw, fwErr := detect.DetectFramework(spec.Dir)
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
		dockerfile = filepath.Join(pierDir, "Dockerfile")
		if err := os.WriteFile(dockerfile, []byte(tmpl), 0644); err != nil {
			return "", 0, fmt.Errorf("writing generated Dockerfile: %w", err)
		}
	} else if port == 0 {
		if fw, fwErr := detect.DetectFramework(spec.Dir); fwErr == nil {
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

// RunContainer stops the old container, starts a new one with Traefik labels, and registers it.
func RunContainer(ctx context.Context, spec AppSpec, image string, port int, cfg *config.Config, envOverrides []string) error {
	// Stop old container
	if err := docker.StopAndRemoveContainer(ctx, spec.Name); err != nil {
		// Non-fatal, container might not exist
	}

	dockerArgs := []string{"run", "-d", "--name", spec.Name, "--network", cfg.Network, "--restart", "unless-stopped"}

	// Env overrides from shared services
	for _, e := range envOverrides {
		dockerArgs = append(dockerArgs, "-e", e)
	}

	// App-specific env
	for k, v := range spec.Env {
		dockerArgs = append(dockerArgs, "-e", fmt.Sprintf("%s=%s", k, v))
	}

	// Traefik labels
	dockerArgs = append(dockerArgs,
		"-l", "traefik.enable=true",
		"-l", fmt.Sprintf("traefik.http.routers.%s.rule=Host(`%s.%s`)", spec.Name, spec.Name, cfg.TLD),
	)
	if port > 0 {
		dockerArgs = append(dockerArgs,
			"-l", fmt.Sprintf("traefik.http.services.%s.loadbalancer.server.port=%d", spec.Name, port),
		)
	}

	// Volumes
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

	// Entrypoint override
	if spec.Entrypoint != nil {
		switch ep := spec.Entrypoint.(type) {
		case string:
			dockerArgs = append(dockerArgs, "--entrypoint", ep)
		case []interface{}:
			if len(ep) > 0 {
				dockerArgs = append(dockerArgs, "--entrypoint", fmt.Sprintf("%v", ep[0]))
			}
		}
	}

	dockerArgs = append(dockerArgs, image)

	// Entrypoint args (remaining elements)
	if spec.Entrypoint != nil {
		if ep, ok := spec.Entrypoint.([]interface{}); ok && len(ep) > 1 {
			for _, e := range ep[1:] {
				dockerArgs = append(dockerArgs, fmt.Sprintf("%v", e))
			}
		}
	}

	// Command override
	if spec.Command != nil {
		switch cmd := spec.Command.(type) {
		case string:
			dockerArgs = append(dockerArgs, cmd)
		case []interface{}:
			for _, c := range cmd {
				dockerArgs = append(dockerArgs, fmt.Sprintf("%v", c))
			}
		}
	}

	dockerCmd := exec.CommandContext(ctx, "docker", dockerArgs...)
	out, err := dockerCmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("docker run failed: %s\n%s", err, string(out))
	}

	// File proxy backup — caller should handle this via cli createContainerProxy

	// Register project
	if err := registry.Register(registry.Project{Name: spec.Name, Dir: spec.Dir, Type: "docker"}); err != nil {
		fmt.Fprintf(os.Stderr, "warning: could not register project: %v\n", err)
	}

	return nil
}
