package pierfile

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

const FileName = "Pierfile"

// ServiceEntry supports both simple string ("postgres:16") and map form:
//
//	services:
//	  - postgres:16          # string
//	  - name: redis          # map with optional fields
//	    version: "7"
//	    port: 6379
//	    env:
//	      REDIS_PASSWORD: secret
type ServiceEntry struct {
	Name    string            `yaml:"name,omitempty" json:"name,omitempty"`
	Version string            `yaml:"version,omitempty" json:"version,omitempty"`
	Port    int               `yaml:"port,omitempty" json:"port,omitempty"`
	Command string            `yaml:"command,omitempty" json:"command,omitempty"`
	Env     map[string]string `yaml:"env,omitempty" json:"env,omitempty"`
}

// UnmarshalYAML allows ServiceEntry to be either a plain string or a map.
func (s *ServiceEntry) UnmarshalYAML(value *yaml.Node) error {
	// String form: "postgres:16" or "redis"
	if value.Kind == yaml.ScalarNode {
		parts := strings.SplitN(value.Value, ":", 2)
		s.Name = parts[0]
		if len(parts) > 1 {
			s.Version = parts[1]
		}
		return nil
	}

	// Map form
	if value.Kind == yaml.MappingNode {
		type raw ServiceEntry // avoid recursion
		var r raw
		if err := value.Decode(&r); err != nil {
			return err
		}
		*s = ServiceEntry(r)
		return nil
	}

	return fmt.Errorf("service entry must be a string or map, got %v", value.Kind)
}

// MarshalYAML writes back as string if only name/version are set, map otherwise.
func (s ServiceEntry) MarshalYAML() (interface{}, error) {
	if s.Port == 0 && s.Command == "" && len(s.Env) == 0 {
		return FormatService(s.Name, s.Version), nil
	}
	type raw ServiceEntry
	return raw(s), nil
}

// String returns the "name:version" form.
func (s ServiceEntry) String() string {
	return FormatService(s.Name, s.Version)
}

type Pierfile struct {
	Name     string            `yaml:"name"`
	Services []ServiceEntry    `yaml:"services,omitempty"`
	Port     int               `yaml:"port,omitempty"`
	Build    bool              `yaml:"build,omitempty"`
	Env      map[string]string `yaml:"env,omitempty"`
}

// ServiceNames returns just the service names (for backwards compat).
func (pf *Pierfile) ServiceNames() []string {
	names := make([]string, len(pf.Services))
	for i, s := range pf.Services {
		names[i] = s.String()
	}
	return names
}

type ServiceDep struct {
	Name    string
	Version string
}

func ParseService(s string) ServiceDep {
	parts := strings.SplitN(s, ":", 2)
	dep := ServiceDep{Name: parts[0]}
	if len(parts) > 1 {
		dep.Version = parts[1]
	}
	return dep
}

func FormatService(name, version string) string {
	if version == "" {
		return name
	}
	return fmt.Sprintf("%s:%s", name, version)
}

func Exists(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, FileName))
	return err == nil
}

func Load(dir string) (*Pierfile, error) {
	data, err := os.ReadFile(filepath.Join(dir, FileName))
	if err != nil {
		return nil, err
	}
	var pf Pierfile
	if err := yaml.Unmarshal(data, &pf); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", FileName, err)
	}
	return &pf, nil
}

func (pf *Pierfile) ToJSON() (string, error) {
	data, err := json.MarshalIndent(pf, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func Save(dir string, pf *Pierfile) error {
	data, err := yaml.Marshal(pf)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, FileName), data, 0644)
}
