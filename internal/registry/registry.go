package registry

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/eshe-huli/pier/internal/config"
)

// Project represents a known project in the registry
type Project struct {
	Name      string `json:"name"`
	Dir       string `json:"dir"`
	Port      int    `json:"port"`
	Command   string `json:"command,omitempty"`
	Type      string `json:"type"` // "linked", "proxy", "docker", "run"
	Framework string `json:"framework,omitempty"`
	LastUsed  string `json:"lastUsed"`
}

var mu sync.Mutex

func registryPath() string {
	return filepath.Join(config.PierDir(), "registry.json")
}

// Load returns all registered projects
func Load() ([]Project, error) {
	data, err := os.ReadFile(registryPath())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var projects []Project
	if err := json.Unmarshal(data, &projects); err != nil {
		return nil, err
	}
	return projects, nil
}

func save(projects []Project) error {
	_ = os.MkdirAll(config.PierDir(), 0755)
	data, err := json.MarshalIndent(projects, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(registryPath(), data, 0644)
}

// Register adds or updates a project in the registry
func Register(p Project) error {
	mu.Lock()
	defer mu.Unlock()

	projects, _ := Load()
	p.LastUsed = time.Now().UTC().Format(time.RFC3339)

	// Update existing or append
	found := false
	for i, existing := range projects {
		if sameProject(existing, p) {
			projects[i] = p
			found = true
			break
		}
	}
	if !found {
		projects = append(projects, p)
	}

	return save(projects)
}

func sameProject(existing Project, next Project) bool {
	if existing.Name != "" && next.Name != "" && existing.Name == next.Name {
		return true
	}
	if existing.Dir == "" || next.Dir == "" || existing.Dir != next.Dir {
		return false
	}
	if supportsMultipleProjectsPerDir(existing.Type) || supportsMultipleProjectsPerDir(next.Type) {
		return existing.Name == "" || next.Name == ""
	}
	return true
}

func supportsMultipleProjectsPerDir(projectType string) bool {
	switch projectType {
	case "linked", "proxy", "process":
		return true
	default:
		return false
	}
}

// Remove removes a project from the registry by name
func Remove(name string) error {
	mu.Lock()
	defer mu.Unlock()

	projects, _ := Load()
	filtered := make([]Project, 0, len(projects))
	for _, p := range projects {
		if p.Name != name {
			filtered = append(filtered, p)
		}
	}
	return save(filtered)
}

// Touch updates the lastUsed timestamp for a project
func Touch(name string) error {
	mu.Lock()
	defer mu.Unlock()

	projects, _ := Load()
	for i, p := range projects {
		if p.Name == name {
			projects[i].LastUsed = time.Now().UTC().Format(time.RFC3339)
			return save(projects)
		}
	}
	return nil
}
