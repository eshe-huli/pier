// Package manifest reads and writes Pier's project-local runtime manifest.
package manifest

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

const (
	DirName  = ".pier"
	FileName = "manifest.yaml"
	Version  = 1
)

type Manifest struct {
	Version  int       `yaml:"version"`
	Source   string    `yaml:"source,omitempty"`
	Project  Project   `yaml:"project,omitempty"`
	Services []Service `yaml:"services,omitempty"`
	Apps     []App     `yaml:"apps,omitempty"`
}

type Project struct {
	Name string `yaml:"name,omitempty"`
}

type Service struct {
	Name    string `yaml:"name"`
	Version string `yaml:"version,omitempty"`
}

type App struct {
	Name      string    `yaml:"name,omitempty"`
	Port      int       `yaml:"port,omitempty"`
	Overrides Overrides `yaml:"overrides,omitempty"`
}

type Overrides struct {
	Env map[string]string `yaml:"env,omitempty"`
}

func Path(dir string) string {
	return filepath.Join(dir, DirName, FileName)
}

func Exists(dir string) bool {
	_, err := os.Stat(Path(dir))
	return err == nil
}

func Load(dir string) (*Manifest, error) {
	data, err := os.ReadFile(Path(dir))
	if err != nil {
		return nil, err
	}

	var mf Manifest
	if err := yaml.Unmarshal(data, &mf); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", FileName, err)
	}
	if mf.Version == 0 {
		mf.Version = Version
	}
	return &mf, nil
}

func Save(dir string, mf *Manifest) error {
	pierDir := filepath.Join(dir, DirName)
	if err := os.MkdirAll(pierDir, 0755); err != nil {
		return fmt.Errorf("creating %s directory: %w", DirName, err)
	}

	if mf.Version == 0 {
		mf.Version = Version
	}

	data, err := yaml.Marshal(mf)
	if err != nil {
		return fmt.Errorf("marshalling %s: %w", FileName, err)
	}

	if err := os.WriteFile(Path(dir), data, 0644); err != nil {
		return fmt.Errorf("writing %s: %w", FileName, err)
	}
	return nil
}
