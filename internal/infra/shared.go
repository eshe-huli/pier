package infra

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/eshe-huli/pier/internal/config"
	"github.com/eshe-huli/pier/internal/docker"
)

// SharedService represents a managed infrastructure service
type SharedService struct {
	Name      string
	Version   string
	Image     string
	Container string
	Port      int
	DataDir   string
	EnvVars   map[string]string
}

// ServiceDef defines a supported infrastructure service
type ServiceDef struct {
	Image    func(version string) string
	Port     int
	EnvVars  map[string]string
	RunArgs  func(version string) []string // extra docker run args
}

var serviceDefs = map[string]ServiceDef{
	"postgres": {
		Image:   func(v string) string { return fmt.Sprintf("postgres:%s-alpine", v) },
		Port:    5432,
		EnvVars: map[string]string{"POSTGRES_USER": "pier", "POSTGRES_PASSWORD": "pier"},
	},
	"redis": {
		Image:   func(v string) string { return fmt.Sprintf("redis:%s-alpine", v) },
		Port:    6379,
		EnvVars: map[string]string{},
	},
	"mongo": {
		Image:   func(v string) string { return fmt.Sprintf("mongo:%s", v) },
		Port:    27017,
		EnvVars: map[string]string{},
	},
	"mysql": {
		Image:   func(v string) string { return fmt.Sprintf("mysql:%s", v) },
		Port:    3306,
		EnvVars: map[string]string{"MYSQL_ROOT_PASSWORD": "pier"},
	},
	"minio": {
		Image:   func(v string) string { return "minio/minio" },
		Port:    9000,
		EnvVars: map[string]string{},
		RunArgs: func(v string) []string { return []string{"server", "/data", "--console-address", ":9001"} },
	},
}

func containerName(name, version string) string {
	return fmt.Sprintf("pier-%s-%s", name, version)
}

func dataDir(name, version string) string {
	return filepath.Join(config.PierDir(), "data", fmt.Sprintf("%s-%s", name, version))
}

// ResolveService builds a SharedService from name and version
func ResolveService(name, version string) (*SharedService, error) {
	def, ok := serviceDefs[name]
	if !ok {
		return nil, fmt.Errorf("unsupported service: %s (supported: postgres, redis, mongo, mysql, minio)", name)
	}

	return &SharedService{
		Name:      name,
		Version:   version,
		Image:     def.Image(version),
		Container: containerName(name, version),
		Port:      def.Port,
		DataDir:   dataDir(name, version),
		EnvVars:   def.EnvVars,
	}, nil
}

// EnsureService starts a shared service if not already running
func EnsureService(name, version string) error {
	ctx := context.Background()
	cname := containerName(name, version)

	if docker.IsContainerRunning(ctx, cname) {
		return nil
	}

	svc, err := ResolveService(name, version)
	if err != nil {
		return err
	}

	def := serviceDefs[name]
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	// Ensure network
	if _, err := docker.EnsureNetwork(ctx, cfg.Network); err != nil {
		return fmt.Errorf("ensuring network: %w", err)
	}

	// Ensure data dir
	if err := os.MkdirAll(svc.DataDir, 0755); err != nil {
		return fmt.Errorf("creating data dir: %w", err)
	}

	// Remove stopped container if exists
	_ = docker.StopAndRemoveContainer(ctx, cname)

	// Build docker run command
	args := []string{"run", "-d", "--name", cname, "--network", cfg.Network, "--restart", "unless-stopped"}

	// Mount data volume
	var mountTarget string
	switch name {
	case "postgres":
		mountTarget = "/var/lib/postgresql/data"
	case "redis":
		mountTarget = "/data"
	case "mongo":
		mountTarget = "/data/db"
	case "mysql":
		mountTarget = "/var/lib/mysql"
	case "minio":
		mountTarget = "/data"
	}
	if mountTarget != "" {
		args = append(args, "-v", fmt.Sprintf("%s:%s", svc.DataDir, mountTarget))
	}

	// Env vars
	for k, v := range svc.EnvVars {
		args = append(args, "-e", fmt.Sprintf("%s=%s", k, v))
	}

	args = append(args, svc.Image)

	// Extra run args (e.g. minio server command)
	if def.RunArgs != nil {
		args = append(args, def.RunArgs(version)...)
	}

	cmd := exec.Command("docker", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("starting %s: %s\n%s", cname, err, string(out))
	}

	return nil
}

// StopService stops a shared service
func StopService(name, version string) error {
	ctx := context.Background()
	return docker.StopAndRemoveContainer(ctx, containerName(name, version))
}

// ListRunning returns all running pier-* infrastructure containers
func ListRunning() []SharedService {
	ctx := context.Background()
	cfg, _ := config.Load()
	if cfg == nil {
		return nil
	}

	containers, err := docker.ListContainers(ctx, cfg.Network, cfg.TLD)
	if err != nil {
		return nil
	}

	var services []SharedService
	for _, c := range containers {
		if !strings.HasPrefix(c.Name, "pier-") || c.Name == "pier-traefik" {
			continue
		}

		// Parse pier-{name}-{version}
		parts := strings.SplitN(strings.TrimPrefix(c.Name, "pier-"), "-", 2)
		if len(parts) != 2 {
			continue
		}

		svcName := parts[0]
		svcVersion := parts[1]

		if _, ok := serviceDefs[svcName]; !ok {
			continue
		}

		svc, err := ResolveService(svcName, svcVersion)
		if err != nil {
			continue
		}

		if c.State == "running" {
			services = append(services, *svc)
		}
	}

	return services
}

// IsInfraContainer checks if a container name matches a known pier infra service
func IsInfraContainer(name string) bool {
	if !strings.HasPrefix(name, "pier-") {
		return false
	}
	trimmed := strings.TrimPrefix(name, "pier-")
	for svcName := range serviceDefs {
		if strings.HasPrefix(trimmed, svcName+"-") {
			return true
		}
	}
	return false
}

// GetConnectionEnv returns env vars for connecting to a shared service
func GetConnectionEnv(name, version string) map[string]string {
	cname := containerName(name, version)
	def, ok := serviceDefs[name]
	if !ok {
		return nil
	}

	env := map[string]string{}

	switch name {
	case "postgres":
		env["DB_HOST"] = cname
		env["DB_PORT"] = fmt.Sprintf("%d", def.Port)
		env["DB_USER"] = "pier"
		env["DB_USERNAME"] = "pier"
		env["DB_PASSWORD"] = "pier"
		env["DB_DATABASE"] = "" // set by caller with project name
		env["DB_SYNC"] = "true"
		env["DATABASE_HOST"] = cname
		env["DATABASE_PORT"] = fmt.Sprintf("%d", def.Port)
		env["DATABASE_URL"] = fmt.Sprintf("postgres://pier:pier@%s:%d", cname, def.Port)
		env["POSTGRES_HOST"] = cname
		env["POSTGRES_PORT"] = fmt.Sprintf("%d", def.Port)
		env["POSTGRES_USER"] = "pier"
		env["POSTGRES_PASSWORD"] = "pier"
	case "redis":
		env["REDIS_HOST"] = cname
		env["REDIS_PORT"] = fmt.Sprintf("%d", def.Port)
		env["CACHE_HOST"] = cname
		env["CACHE_PORT"] = fmt.Sprintf("%d", def.Port)
	case "mongo":
		env["MONGO_HOST"] = cname
		env["MONGO_PORT"] = fmt.Sprintf("%d", def.Port)
	case "mysql":
		env["DB_HOST"] = cname
		env["DB_PORT"] = fmt.Sprintf("%d", def.Port)
		env["DB_USER"] = "root"
		env["DB_PASSWORD"] = "pier"
		env["MYSQL_HOST"] = cname
		env["MYSQL_PORT"] = fmt.Sprintf("%d", def.Port)
	case "minio":
		env["MINIO_ENDPOINT"] = fmt.Sprintf("%s:9000", cname)
		env["S3_ENDPOINT"] = fmt.Sprintf("http://%s:9000", cname)
	}

	return env
}
