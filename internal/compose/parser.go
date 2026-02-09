package compose

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// ComposeFile represents a docker-compose.yml
type ComposeFile struct {
	Services map[string]ComposeService `yaml:"services"`
}

// ComposeService represents a service in docker-compose.yml
type ComposeService struct {
	Image       string      `yaml:"image"`
	Build       interface{} `yaml:"build"`
	Ports       []string    `yaml:"ports"`
	Environment interface{} `yaml:"environment"`
	DependsOn   interface{} `yaml:"depends_on"`
	Volumes     []string    `yaml:"volumes"`
	Command     interface{} `yaml:"command"`
}

// InfraService is a detected infrastructure service from compose
type InfraService struct {
	ComposeName string
	Image       string
	Name        string // e.g. "postgres"
	Version     string // e.g. "16"
}

// AppService is an application service from compose
type AppService struct {
	ComposeName string
	Build       string // build context path
	Image       string
	Ports       []string
	Environment map[string]string
}

var knownInfra = map[string]bool{
	"postgres": true, "redis": true, "mongo": true, "mysql": true,
	"minio": true, "kafka": true, "rabbitmq": true, "elasticsearch": true,
	"mariadb": true, "memcached": true,
}

// Parse reads and parses a docker-compose file from the given directory
func Parse(dir string) (*ComposeFile, error) {
	var data []byte
	var err error
	for _, name := range []string{"docker-compose.yml", "docker-compose.yaml", "compose.yml", "compose.yaml"} {
		data, err = os.ReadFile(filepath.Join(dir, name))
		if err == nil {
			break
		}
	}
	if err != nil {
		return nil, fmt.Errorf("no compose file found: %w", err)
	}

	var cf ComposeFile
	if err := yaml.Unmarshal(data, &cf); err != nil {
		return nil, fmt.Errorf("parsing compose file: %w", err)
	}
	return &cf, nil
}

// SeparateServices splits compose services into infra and app services
func SeparateServices(cf *ComposeFile) (infra []InfraService, apps []AppService) {
	for name, svc := range cf.Services {
		if svc.Image != "" && isInfraImage(svc.Image) {
			imgName, imgVersion := parseImageTag(svc.Image)
			infra = append(infra, InfraService{
				ComposeName: name,
				Image:       svc.Image,
				Name:        imgName,
				Version:     imgVersion,
			})
		} else {
			app := AppService{
				ComposeName: name,
				Image:       svc.Image,
				Ports:       svc.Ports,
				Environment: parseEnvironment(svc.Environment),
			}
			app.Build = parseBuildContext(svc.Build)
			apps = append(apps, app)
		}
	}
	return
}

func isInfraImage(image string) bool {
	parts := strings.Split(image, "/")
	nameTag := parts[len(parts)-1]
	name := strings.SplitN(nameTag, ":", 2)[0]
	return knownInfra[name]
}

func parseImageTag(image string) (name, version string) {
	parts := strings.Split(image, "/")
	nameTag := parts[len(parts)-1]
	split := strings.SplitN(nameTag, ":", 2)
	name = split[0]
	if len(split) > 1 {
		version = split[1]
		// Strip suffixes like "-alpine"
		if i := strings.IndexByte(version, '-'); i > 0 {
			version = version[:i]
		}
	}
	return
}

func parseBuildContext(build interface{}) string {
	if build == nil {
		return ""
	}
	switch v := build.(type) {
	case string:
		return v
	case map[string]interface{}:
		if ctx, ok := v["context"]; ok {
			if s, ok := ctx.(string); ok {
				return s
			}
		}
	}
	return "."
}

func parseEnvironment(env interface{}) map[string]string {
	result := make(map[string]string)
	if env == nil {
		return result
	}
	switch v := env.(type) {
	case []interface{}:
		for _, item := range v {
			if s, ok := item.(string); ok {
				parts := strings.SplitN(s, "=", 2)
				if len(parts) == 2 {
					result[parts[0]] = parts[1]
				}
			}
		}
	case map[string]interface{}:
		for k, val := range v {
			result[k] = fmt.Sprintf("%v", val)
		}
	}
	return result
}
