package proxy

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"

	"github.com/eshe-huli/pier/internal/config"
)

const (
	traefikContainerName = "pier-traefik"
)

// TraefikRouter represents a route from the Traefik API
type TraefikRouter struct {
	Name     string `json:"name"`
	Rule     string `json:"rule"`
	Service  string `json:"service"`
	Status   string `json:"status"`
	Provider string `json:"provider"`
	EntryPoints []string `json:"entryPoints"`
}

// GenerateTraefikConfig generates the Traefik static configuration
func GenerateTraefikConfig(cfg *config.Config) error {
	// Use simple defaultRule — just container name + TLD
	// Traefik Go template syntax: {{ .Name }} gives the container name
	conf := fmt.Sprintf(`# Pier — managed by pier CLI
# Traefik v3 static configuration

api:
  dashboard: true
  insecure: true

entryPoints:
  web:
    address: ":80"

providers:
  docker:
    endpoint: "unix:///var/run/docker.sock"
    exposedByDefault: false
    network: %s
    defaultRule: "Host(` + "`" + `{{ trimPrefix ` + "`" + `/` + "`" + ` .Name }}.%s` + "`" + `)"
  file:
    directory: "/etc/traefik/dynamic"
    watch: true
`, cfg.Network, cfg.TLD)

	if err := os.MkdirAll(config.TraefikDir(), 0755); err != nil {
		return fmt.Errorf("creating traefik directory: %w", err)
	}

	if err := os.MkdirAll(config.TraefikDynamicDir(), 0755); err != nil {
		return fmt.Errorf("creating traefik dynamic directory: %w", err)
	}

	if err := os.WriteFile(config.TraefikConfigPath(), []byte(conf), 0644); err != nil {
		return fmt.Errorf("writing traefik config: %w", err)
	}

	return nil
}

// StartTraefik starts the Traefik container
func StartTraefik(ctx context.Context, cfg *config.Config) error {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return fmt.Errorf("connecting to Docker: %w", err)
	}
	defer cli.Close()

	// Check if container already exists and is running
	info, err := cli.ContainerInspect(ctx, traefikContainerName)
	if err == nil {
		if info.State.Running {
			return nil // Already running
		}
		// Container exists but stopped — remove and recreate
		_ = cli.ContainerRemove(ctx, traefikContainerName, container.RemoveOptions{Force: true})
	}

	// Pull image if needed
	_, _, err = cli.ImageInspectWithRaw(ctx, cfg.Traefik.Image)
	if err != nil {
		reader, pullErr := cli.ImagePull(ctx, cfg.Traefik.Image, image.PullOptions{})
		if pullErr != nil {
			return fmt.Errorf("pulling Traefik image: %w", pullErr)
		}
		defer reader.Close()
		_, _ = io.Copy(io.Discard, reader) // Wait for pull to complete
	}

	// Resolve paths
	home, _ := os.UserHomeDir()
	traefikYaml := filepath.Join(home, ".pier", "traefik", "traefik.yaml")
	dynamicDir := filepath.Join(home, ".pier", "traefik", "dynamic")

	// Container config
	containerCfg := &container.Config{
		Image: cfg.Traefik.Image,
		Labels: map[string]string{
			"pier.domain":    "traefik",
			"traefik.enable": "true",
		},
		ExposedPorts: nat.PortSet{
			"80/tcp":   {},
			"8080/tcp": {},
		},
	}

	hostCfg := &container.HostConfig{
		RestartPolicy: container.RestartPolicy{Name: "unless-stopped"},
		PortBindings: nat.PortMap{
			"80/tcp": []nat.PortBinding{
				{HostIP: "0.0.0.0", HostPort: fmt.Sprintf("%d", cfg.Traefik.Port)},
			},
			"8080/tcp": []nat.PortBinding{
				{HostIP: "0.0.0.0", HostPort: fmt.Sprintf("%d", cfg.Traefik.Port+1)},
			},
		},
		Mounts: []mount.Mount{
			{
				Type:   mount.TypeBind,
				Source: "/var/run/docker.sock",
				Target: "/var/run/docker.sock",
			},
			{
				Type:   mount.TypeBind,
				Source: traefikYaml,
				Target: "/etc/traefik/traefik.yaml",
			},
			{
				Type:   mount.TypeBind,
				Source: dynamicDir,
				Target: "/etc/traefik/dynamic",
			},
		},
	}

	networkCfg := &network.NetworkingConfig{
		EndpointsConfig: map[string]*network.EndpointSettings{
			cfg.Network: {},
		},
	}

	resp, err := cli.ContainerCreate(ctx, containerCfg, hostCfg, networkCfg, nil, traefikContainerName)
	if err != nil {
		return fmt.Errorf("creating Traefik container: %w", err)
	}

	if err := cli.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		return fmt.Errorf("starting Traefik container: %w", err)
	}

	return nil
}

// StopTraefik stops and removes the Traefik container
func StopTraefik(ctx context.Context) error {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return fmt.Errorf("connecting to Docker: %w", err)
	}
	defer cli.Close()

	if err := cli.ContainerStop(ctx, traefikContainerName, container.StopOptions{}); err != nil {
		if !strings.Contains(err.Error(), "No such container") &&
			!strings.Contains(err.Error(), "is not running") {
			return fmt.Errorf("stopping Traefik: %w", err)
		}
	}

	if err := cli.ContainerRemove(ctx, traefikContainerName, container.RemoveOptions{Force: true}); err != nil {
		if !strings.Contains(err.Error(), "No such container") {
			return fmt.Errorf("removing Traefik: %w", err)
		}
	}

	return nil
}

// IsTraefikRunning checks if the Traefik container is running
func IsTraefikRunning(ctx context.Context) bool {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return false
	}
	defer cli.Close()

	info, err := cli.ContainerInspect(ctx, traefikContainerName)
	if err != nil {
		return false
	}

	return info.State.Running
}

// GetTraefikRouters fetches active routes from the Traefik API
func GetTraefikRouters(dashboardPort int) ([]TraefikRouter, error) {
	url := fmt.Sprintf("http://127.0.0.1:%d/api/http/routers", dashboardPort)

	httpClient := &http.Client{Timeout: 5 * time.Second}
	resp, err := httpClient.Get(url)
	if err != nil {
		return nil, fmt.Errorf("connecting to Traefik API: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Traefik API returned status %d", resp.StatusCode)
	}

	var routers []TraefikRouter
	if err := json.NewDecoder(resp.Body).Decode(&routers); err != nil {
		return nil, fmt.Errorf("parsing Traefik API response: %w", err)
	}

	return routers, nil
}

// GetTraefikRouteCount returns the number of active routes
func GetTraefikRouteCount(dashboardPort int) int {
	routers, err := GetTraefikRouters(dashboardPort)
	if err != nil {
		return 0
	}
	return len(routers)
}
