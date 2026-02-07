package docker

import (
	"context"
	"fmt"
	"strings"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
)

// ContainerInfo holds information about a container on the pier network
type ContainerInfo struct {
	ID          string
	Name        string
	Domain      string
	Image       string
	State       string
	Status      string
	ComposeProject string
	ComposeService string
	PierDomain  string
}

// ListContainers returns all containers on the pier network
func ListContainers(ctx context.Context, networkName string, tld string) ([]ContainerInfo, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, fmt.Errorf("connecting to Docker: %w", err)
	}
	defer cli.Close()

	containers, err := cli.ContainerList(ctx, container.ListOptions{All: true})
	if err != nil {
		return nil, fmt.Errorf("listing containers: %w", err)
	}

	var result []ContainerInfo
	for _, c := range containers {
		// Check if container is on the pier network
		if !isOnNetwork(c, networkName) {
			continue
		}

		info := ContainerInfo{
			ID:    c.ID[:12],
			Name:  cleanName(c.Names),
			Image: c.Image,
			State: c.State,
			Status: c.Status,
		}

		// Extract compose labels
		info.ComposeProject = c.Labels["com.docker.compose.project"]
		info.ComposeService = c.Labels["com.docker.compose.service"]
		info.PierDomain = c.Labels["pier.domain"]

		// Determine domain: pier.domain > compose service > container name
		info.Domain = resolveDomain(info, tld)

		result = append(result, info)
	}

	return result, nil
}

// GetContainer returns info about a specific container
func GetContainer(ctx context.Context, nameOrID string) (*types.ContainerJSON, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, fmt.Errorf("connecting to Docker: %w", err)
	}
	defer cli.Close()

	info, err := cli.ContainerInspect(ctx, nameOrID)
	if err != nil {
		return nil, fmt.Errorf("inspecting container: %w", err)
	}

	return &info, nil
}

// IsContainerRunning checks if a specific container is running
func IsContainerRunning(ctx context.Context, name string) bool {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return false
	}
	defer cli.Close()

	info, err := cli.ContainerInspect(ctx, name)
	if err != nil {
		return false
	}

	return info.State.Running
}

// StopAndRemoveContainer stops and removes a container by name
func StopAndRemoveContainer(ctx context.Context, name string) error {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return fmt.Errorf("connecting to Docker: %w", err)
	}
	defer cli.Close()

	// Stop the container
	if err := cli.ContainerStop(ctx, name, container.StopOptions{}); err != nil {
		// Ignore "not found" or "not running" errors
		if !strings.Contains(err.Error(), "No such container") &&
			!strings.Contains(err.Error(), "is not running") {
			return fmt.Errorf("stopping container: %w", err)
		}
	}

	// Remove the container
	if err := cli.ContainerRemove(ctx, name, container.RemoveOptions{Force: true}); err != nil {
		if !strings.Contains(err.Error(), "No such container") {
			return fmt.Errorf("removing container: %w", err)
		}
	}

	return nil
}

func isOnNetwork(c types.Container, networkName string) bool {
	if c.NetworkSettings == nil {
		return false
	}
	for name := range c.NetworkSettings.Networks {
		if name == networkName {
			return true
		}
	}
	return false
}

func cleanName(names []string) string {
	if len(names) == 0 {
		return "unknown"
	}
	name := names[0]
	return strings.TrimPrefix(name, "/")
}

func resolveDomain(info ContainerInfo, tld string) string {
	var name string
	switch {
	case info.PierDomain != "":
		name = info.PierDomain
	case info.ComposeService != "":
		name = info.ComposeService
	default:
		name = info.Name
	}
	return name + "." + tld
}
