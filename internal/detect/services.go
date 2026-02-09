package detect

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

type ServiceDep struct {
	Name    string
	Version string
}

func (s ServiceDep) String() string {
	if s.Version == "" {
		return s.Name
	}
	return s.Name + ":" + s.Version
}

func DetectServices(dir string) ([]ServiceDep, error) {
	// Compose is most reliable
	deps, err := DetectFromCompose(dir)
	if err == nil && len(deps) > 0 {
		return deps, nil
	}

	// Fall back to dependency file detection
	var all []ServiceDep
	for _, fn := range []func(string) ([]ServiceDep, error){
		DetectFromPackageJSON,
		DetectFromPython,
		DetectFromRuby,
	} {
		if found, err := fn(dir); err == nil {
			all = append(all, found...)
		}
	}
	return dedup(all), nil
}

func DetectFromCompose(dir string) ([]ServiceDep, error) {
	var data []byte
	var err error
	for _, name := range []string{"docker-compose.yml", "docker-compose.yaml", "compose.yml", "compose.yaml"} {
		data, err = os.ReadFile(filepath.Join(dir, name))
		if err == nil {
			break
		}
	}
	if err != nil {
		return nil, err
	}

	var compose struct {
		Services map[string]struct {
			Image string `yaml:"image"`
			Build any    `yaml:"build"`
		} `yaml:"services"`
	}
	if err := yaml.Unmarshal(data, &compose); err != nil {
		return nil, err
	}

	var deps []ServiceDep
	for _, svc := range compose.Services {
		if svc.Build != nil || svc.Image == "" {
			continue
		}
		dep := parseImage(svc.Image)
		if dep != nil {
			deps = append(deps, *dep)
		}
	}
	return deps, nil
}

func parseImage(image string) *ServiceDep {
	known := map[string]bool{"postgres": true, "redis": true, "mongo": true, "mysql": true, "mariadb": true, "rabbitmq": true, "elasticsearch": true, "memcached": true}
	// Strip registry prefix
	parts := strings.Split(image, "/")
	nameTag := parts[len(parts)-1]
	split := strings.SplitN(nameTag, ":", 2)
	name := split[0]
	if !known[name] {
		return nil
	}
	version := ""
	if len(split) > 1 {
		// Strip suffixes like "-alpine"
		v := split[1]
		if i := strings.IndexByte(v, '-'); i > 0 {
			v = v[:i]
		}
		version = v
	}
	return &ServiceDep{Name: name, Version: version}
}

func DetectFromPackageJSON(dir string) ([]ServiceDep, error) {
	data, err := os.ReadFile(filepath.Join(dir, "package.json"))
	if err != nil {
		return nil, err
	}
	var pkg struct {
		Dependencies    map[string]string `json:"dependencies"`
		DevDependencies map[string]string `json:"devDependencies"`
	}
	if err := json.Unmarshal(data, &pkg); err != nil {
		return nil, err
	}
	allDeps := merge(pkg.Dependencies, pkg.DevDependencies)
	return matchDeps(allDeps, map[string]string{
		"pg": "postgres", "typeorm": "postgres", "prisma": "postgres", "@prisma/client": "postgres",
		"redis": "redis", "ioredis": "redis", "bullmq": "redis",
		"mongoose": "mongo", "mongodb": "mongo",
	}), nil
}

func DetectFromPython(dir string) ([]ServiceDep, error) {
	content := ""
	for _, name := range []string{"requirements.txt", "Pipfile"} {
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err == nil {
			content += "\n" + string(data)
		}
	}
	if content == "" {
		return nil, os.ErrNotExist
	}
	lower := strings.ToLower(content)
	return matchContent(lower, map[string]string{
		"psycopg2": "postgres", "sqlalchemy": "postgres",
		"redis": "redis", "celery": "redis",
		"pymongo": "mongo", "djongo": "mongo",
	}), nil
}

func DetectFromRuby(dir string) ([]ServiceDep, error) {
	data, err := os.ReadFile(filepath.Join(dir, "Gemfile"))
	if err != nil {
		return nil, err
	}
	content := strings.ToLower(string(data))
	return matchContent(content, map[string]string{
		"'pg'": "postgres", "\"pg\"": "postgres",
		"'redis'": "redis", "\"redis\"": "redis",
		"'mongoid'": "mongo", "\"mongoid\"": "mongo",
	}), nil
}

func matchDeps(deps map[string]string, mapping map[string]string) []ServiceDep {
	seen := map[string]bool{}
	var result []ServiceDep
	for dep := range deps {
		if svc, ok := mapping[dep]; ok && !seen[svc] {
			seen[svc] = true
			result = append(result, ServiceDep{Name: svc})
		}
	}
	return result
}

func matchContent(content string, mapping map[string]string) []ServiceDep {
	seen := map[string]bool{}
	var result []ServiceDep
	for keyword, svc := range mapping {
		if strings.Contains(content, keyword) && !seen[svc] {
			seen[svc] = true
			result = append(result, ServiceDep{Name: svc})
		}
	}
	return result
}

func merge(a, b map[string]string) map[string]string {
	m := make(map[string]string)
	for k, v := range a {
		m[k] = v
	}
	for k, v := range b {
		m[k] = v
	}
	return m
}

func dedup(deps []ServiceDep) []ServiceDep {
	seen := map[string]bool{}
	var result []ServiceDep
	for _, d := range deps {
		if !seen[d.Name] {
			seen[d.Name] = true
			result = append(result, d)
		}
	}
	return result
}
