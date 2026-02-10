package docker

import (
	"context"
	"fmt"
	"time"

	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
)

// EnsureNetwork creates the pier Docker network if it doesn't exist.
// Returns true if the network was created, false if it already existed.
func EnsureNetwork(ctx context.Context, networkName string) (bool, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return false, fmt.Errorf("connecting to Docker: %w", err)
	}
	defer cli.Close()

	// Check if network already exists
	networks, err := cli.NetworkList(ctx, network.ListOptions{})
	if err != nil {
		return false, fmt.Errorf("listing networks: %w", err)
	}

	for _, n := range networks {
		if n.Name == networkName {
			return false, nil
		}
	}

	// Create the network
	_, err = cli.NetworkCreate(ctx, networkName, network.CreateOptions{
		Driver: "bridge",
	})
	if err != nil {
		return false, fmt.Errorf("creating network '%s': %w", networkName, err)
	}

	return true, nil
}

// NetworkExists checks if the pier Docker network exists
func NetworkExists(ctx context.Context, networkName string) (bool, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return false, fmt.Errorf("connecting to Docker: %w", err)
	}
	defer cli.Close()

	networks, err := cli.NetworkList(ctx, network.ListOptions{})
	if err != nil {
		return false, fmt.Errorf("listing networks: %w", err)
	}

	for _, n := range networks {
		if n.Name == networkName {
			return true, nil
		}
	}

	return false, nil
}

// IsDockerRunning checks if the Docker daemon is reachable.
// Retries briefly to handle Docker/OrbStack still starting up.
func IsDockerRunning() bool {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return false
	}
	defer cli.Close()

	// Retry up to 3 times with 1s delay — handles daemon still starting
	for i := 0; i < 3; i++ {
		_, err = cli.Ping(context.Background())
		if err == nil {
			return true
		}
		if i < 2 {
			time.Sleep(time.Second)
		}
	}
	return false
}
