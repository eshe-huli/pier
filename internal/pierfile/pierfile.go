package pierfile

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

const FileName = ".pierfile"

type Pierfile struct {
	Name     string            `yaml:"name"`
	Services []string          `yaml:"services,omitempty"`
	Port     int               `yaml:"port,omitempty"`
	Build    bool              `yaml:"build,omitempty"`
	Env      map[string]string `yaml:"env,omitempty"`
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

func Save(dir string, pf *Pierfile) error {
	data, err := yaml.Marshal(pf)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, FileName), data, 0644)
}
