package proxy

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"

	"github.com/eshe-huli/pier/internal/config"
)

const (
	traefikContainerName = "pier-traefik"
	composeProjectName   = "pier"
)

// TraefikRouter represents a route from the Traefik API
type TraefikRouter struct {
	Name        string   `json:"name"`
	Rule        string   `json:"rule"`
	Service     string   `json:"service"`
	Status      string   `json:"status"`
	Provider    string   `json:"provider"`
	EntryPoints []string `json:"entryPoints"`
}

func WebHostPort(cfg *config.Config) int {
	if cfg.Nginx.Managed {
		return cfg.Traefik.Port
	}
	return 80
}

func DashboardHostPort(cfg *config.Config) int {
	return cfg.Traefik.Port + 1
}

func EdgeDescription(cfg *config.Config) string {
	if cfg.Nginx.Managed {
		return fmt.Sprintf("nginx :80 -> Traefik :%d", WebHostPort(cfg))
	}
	return "Traefik direct :80"
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
    defaultRule: "Host(`+"`"+`{{ trimPrefix `+"`"+`/`+"`"+` .Name }}.%s`+"`"+`)"
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

// generateComposeFile writes the Pier infra docker-compose.yml to ~/.pier/
// This ensures all Pier services appear grouped in Docker Desktop.
func generateComposeFile(cfg *config.Config) (string, error) {
	pierDir := config.PierDir()
	if err := os.MkdirAll(pierDir, 0755); err != nil {
		return "", fmt.Errorf("creating pier directory: %w", err)
	}

	traefikYaml := config.TraefikConfigPath()
	dynamicDir := config.TraefikDynamicDir()
	composePath := filepath.Join(pierDir, "docker-compose.yml")

	compose := fmt.Sprintf(`# Pier Infrastructure - managed by pier CLI
# This file groups all Pier services in Docker Desktop
name: pier

services:
  traefik:
    image: %s
    container_name: %s
    restart: unless-stopped
    labels:
      - pier.domain=traefik
      - traefik.enable=true
    ports:
      - "%d:80"
      - "%d:8080"
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock
      - %s:/etc/traefik/traefik.yaml
      - %s:/etc/traefik/dynamic
    networks:
      - %s

networks:
  %s:
    external: true
`, cfg.Traefik.Image, traefikContainerName,
		WebHostPort(cfg), DashboardHostPort(cfg),
		traefikYaml, dynamicDir,
		cfg.Network, cfg.Network)

	if err := os.WriteFile(composePath, []byte(compose), 0644); err != nil {
		return "", fmt.Errorf("writing compose file: %w", err)
	}

	return composePath, nil
}

// StartTraefik starts the Traefik container via docker compose
func StartTraefik(ctx context.Context, cfg *config.Config) error {
	// Check if already running
	if IsTraefikRunning(ctx) {
		return nil
	}

	// Generate/update the compose file
	composePath, err := generateComposeFile(cfg)
	if err != nil {
		return err
	}

	// Run docker compose up -d
	cmd := exec.CommandContext(ctx, "docker", "compose", "-f", composePath, "-p", composeProjectName, "up", "-d")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("starting Pier infrastructure: %w", err)
	}

	return nil
}

// StopTraefik stops and removes the Traefik container via docker compose
func StopTraefik(ctx context.Context) error {
	composePath := filepath.Join(config.PierDir(), "docker-compose.yml")

	// Try compose down first (if compose file exists)
	if _, err := os.Stat(composePath); err == nil {
		cmd := exec.CommandContext(ctx, "docker", "compose", "-f", composePath, "-p", composeProjectName, "down")
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err == nil {
			return nil
		}
	}

	// Fallback: stop via Docker API (for containers created before compose migration)
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return fmt.Errorf("connecting to Docker: %w", err)
	}
	defer cli.Close()

	_ = cli.ContainerStop(ctx, traefikContainerName, container.StopOptions{})
	_ = cli.ContainerRemove(ctx, traefikContainerName, container.RemoveOptions{Force: true})

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
