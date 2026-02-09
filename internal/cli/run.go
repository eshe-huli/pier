package cli

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"

	"github.com/eshe-huli/pier/internal/config"
	"github.com/eshe-huli/pier/internal/infra"
	"github.com/eshe-huli/pier/internal/proxy"
	"github.com/eshe-huli/pier/internal/runtime"
)

var runImage string
var runPort int
var runEnvs []string
var runBuild bool
var runServices []string

var runCmd = &cobra.Command{
	Use:   "run <name>",
	Short: "Run a Docker container with shared infrastructure",
	Long: `Run a Docker container on the pier network with automatic service
discovery and environment injection.

Examples:
  pier run myapp --image myapp:latest
  pier run myapp --build --port 3000
  pier run myapp --image node:20 --services postgres:16,redis:7
  pier run myapp --build --services postgres:16 --env SECRET=abc`,
	Args: cobra.ExactArgs(1),
	RunE: runRunCmd,
}

func init() {
	runCmd.Flags().StringVar(&runImage, "image", "", "Docker image to run")
	runCmd.Flags().IntVar(&runPort, "port", 0, "Internal port (for Traefik routing)")
	runCmd.Flags().StringArrayVarP(&runEnvs, "env", "e", nil, "Extra env vars (repeatable, KEY=VAL)")
	runCmd.Flags().BoolVar(&runBuild, "build", false, "Build from Dockerfile in current dir first")
	runCmd.Flags().StringSliceVar(&runServices, "services", nil, "Required services (e.g. postgres:16,redis:7)")
	rootCmd.AddCommand(runCmd)
}

func runRunCmd(cmd *cobra.Command, args []string) error {
	name := args[0]

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	// Step 1: Build if requested
	image := runImage
	if runBuild {
		step(1, fmt.Sprintf("Building image %s...", cyan(name)))
		buildCmd := exec.Command("docker", "build", "-t", name, ".")
		buildCmd.Stdout = os.Stdout
		buildCmd.Stderr = os.Stderr
		if err := buildCmd.Run(); err != nil {
			return fmt.Errorf("docker build failed: %w", err)
		}
		image = name
		success("Image built")
	}

	if image == "" {
		return fmt.Errorf("either --image or --build is required")
	}

	// Step 2: Parse and load .pier services if no --services flag
	services := runServices
	if len(services) == 0 {
		services = loadPierFileServices()
	}

	// Step 3: Ensure shared services
	var sharedServices []infra.SharedService
	if len(services) > 0 {
		step(2, "Starting shared infrastructure...")
		for _, svcSpec := range services {
			parts := strings.SplitN(svcSpec, ":", 2)
			if len(parts) != 2 {
				return fmt.Errorf("invalid service spec '%s' (expected name:version, e.g. postgres:16)", svcSpec)
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
		}
	}

	// Step 4: Build env overrides
	envOverrides := runtime.BuildEnvOverrides(sharedServices)

	// Step 5: Run the container
	step(3, fmt.Sprintf("Running %s...", cyan(name)))

	dockerArgs := []string{"run", "-d", "--name", name, "--network", cfg.Network, "--restart", "unless-stopped"}

	// Add env overrides from shared services
	for _, e := range envOverrides {
		dockerArgs = append(dockerArgs, "-e", e)
	}

	// Add user-specified env vars
	for _, e := range runEnvs {
		dockerArgs = append(dockerArgs, "-e", e)
	}

	dockerArgs = append(dockerArgs, image)

	dockerCmd := exec.Command("docker", dockerArgs...)
	out, err := dockerCmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("docker run failed: %s\n%s", err, string(out))
	}

	success(fmt.Sprintf("Container %s started", bold(name)))

	// Step 6: Create Traefik route if port specified
	if runPort > 0 {
		step(4, "Creating route...")
		if err := createContainerProxy(name, runPort, cfg.TLD); err != nil {
			warn(fmt.Sprintf("Could not create route: %s", err))
		} else {
			domain := fmt.Sprintf("%s.%s", name, cfg.TLD)
			success(fmt.Sprintf("Routed → %s", cyan(domain)))
		}
	}

	fmt.Println()
	return nil
}

// createContainerProxy creates a Traefik route for a Docker container (uses container name, not host.docker.internal)
func createContainerProxy(name string, port int, tld string) error {
	domain := fmt.Sprintf("%s.%s", name, tld)
	url := fmt.Sprintf("http://%s:%d", name, port)

	cfg := map[string]interface{}{
		"http": map[string]interface{}{
			"routers": map[string]interface{}{
				name: map[string]interface{}{
					"rule":    fmt.Sprintf("Host(`%s`)", domain),
					"service": name,
					"entryPoints": []string{"web"},
				},
			},
			"services": map[string]interface{}{
				name: map[string]interface{}{
					"loadBalancer": map[string]interface{}{
						"servers": []map[string]interface{}{
							{"url": url},
						},
					},
				},
			},
		},
	}

	return proxy.WriteTraefikConfig(name, cfg)
}

// loadPierFileServices reads services from .pier file in current directory
func loadPierFileServices() []string {
	data, err := os.ReadFile("Pierfile")
	if err != nil {
		return nil
	}

	var services []string
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "services:") {
			continue
		}
		line = strings.TrimPrefix(line, "- ")
		line = strings.TrimSpace(line)
		if line != "" && strings.Contains(line, ":") {
			services = append(services, line)
		}
	}
	return services
}
