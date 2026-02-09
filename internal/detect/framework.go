package detect

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

type Framework struct {
	Name     string
	Language string
	Port     int
}

func DetectFramework(dir string) (*Framework, error) {
	// Node.js frameworks (package.json)
	if hasPkgDep(dir, "@nestjs/core") {
		return &Framework{"nestjs", "node", 3000}, nil
	}
	if hasPkgDep(dir, "next") {
		return &Framework{"nextjs", "node", 3000}, nil
	}
	if hasPkgDep(dir, "nuxt") {
		return &Framework{"nuxt", "node", 3000}, nil
	}
	if hasPkgDep(dir, "express") {
		return &Framework{"express", "node", 3000}, nil
	}
	if hasPkgDep(dir, "fastify") {
		return &Framework{"fastify", "node", 3000}, nil
	}

	// PHP
	if fileContains(dir, "composer.json", "laravel/framework") {
		return &Framework{"laravel", "php", 8000}, nil
	}

	// Python
	if fileContains(dir, "requirements.txt", "django") {
		return &Framework{"django", "python", 8000}, nil
	}
	if fileContains(dir, "requirements.txt", "fastapi") {
		return &Framework{"fastapi", "python", 8000}, nil
	}
	if fileContains(dir, "requirements.txt", "flask") {
		return &Framework{"flask", "python", 5000}, nil
	}

	// Ruby
	if fileContains(dir, "Gemfile", "rails") {
		return &Framework{"rails", "ruby", 3000}, nil
	}

	// Go
	if fileExists(dir, "go.mod") {
		return &Framework{"go", "go", 8080}, nil
	}

	// Rust
	if fileExists(dir, "Cargo.toml") {
		return &Framework{"rust", "rust", 8080}, nil
	}

	// Elixir
	if fileContains(dir, "mix.exs", "phoenix") {
		return &Framework{"phoenix", "elixir", 4000}, nil
	}

	// Java
	if fileContains(dir, "pom.xml", "spring-boot") {
		return &Framework{"spring-boot", "java", 8080}, nil
	}

	return nil, os.ErrNotExist
}

func hasPkgDep(dir, dep string) bool {
	data, err := os.ReadFile(filepath.Join(dir, "package.json"))
	if err != nil {
		return false
	}
	var pkg struct {
		Dependencies    map[string]string `json:"dependencies"`
		DevDependencies map[string]string `json:"devDependencies"`
	}
	if json.Unmarshal(data, &pkg) != nil {
		return false
	}
	if _, ok := pkg.Dependencies[dep]; ok {
		return true
	}
	if _, ok := pkg.DevDependencies[dep]; ok {
		return true
	}
	return false
}

func fileContains(dir, filename, substr string) bool {
	data, err := os.ReadFile(filepath.Join(dir, filename))
	if err != nil {
		return false
	}
	return strings.Contains(strings.ToLower(string(data)), strings.ToLower(substr))
}

func fileExists(dir, filename string) bool {
	_, err := os.Stat(filepath.Join(dir, filename))
	return err == nil
}
