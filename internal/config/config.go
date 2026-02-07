package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Config represents the Pier configuration
type Config struct {
	TLD     string        `yaml:"tld"`
	Traefik TraefikConfig `yaml:"traefik"`
	Network string        `yaml:"network"`
	Nginx   NginxConfig   `yaml:"nginx"`
}

// TraefikConfig holds Traefik-specific settings
type TraefikConfig struct {
	Port      int    `yaml:"port"`
	Dashboard bool   `yaml:"dashboard"`
	Image     string `yaml:"image"`
}

// NginxConfig holds nginx-specific settings
type NginxConfig struct {
	Managed         bool `yaml:"managed"`
	ValetCompatible bool `yaml:"valet_compatible"`
}

// PierDir returns the Pier home directory (~/.pier)
func PierDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".", ".pier")
	}
	return filepath.Join(home, ".pier")
}

// ConfigPath returns the path to config.yaml
func ConfigPath() string {
	return filepath.Join(PierDir(), "config.yaml")
}

// TraefikDir returns the Traefik config directory
func TraefikDir() string {
	return filepath.Join(PierDir(), "traefik")
}

// TraefikDynamicDir returns the Traefik dynamic config directory
func TraefikDynamicDir() string {
	return filepath.Join(TraefikDir(), "dynamic")
}

// TraefikConfigPath returns the Traefik static config path
func TraefikConfigPath() string {
	return filepath.Join(TraefikDir(), "traefik.yaml")
}

// NginxDir returns the nginx config directory
func NginxDir() string {
	return filepath.Join(PierDir(), "nginx")
}

// NginxConfigPath returns the nginx config path
func NginxConfigPath() string {
	return filepath.Join(NginxDir(), "pier.conf")
}

// Default returns the default configuration
func Default() *Config {
	return &Config{
		TLD: "dock",
		Traefik: TraefikConfig{
			Port:      8880,
			Dashboard: true,
			Image:     "traefik:v3.3",
		},
		Network: "pier",
		Nginx: NginxConfig{
			Managed:         true,
			ValetCompatible: true,
		},
	}
}

// Load reads the config from disk, falling back to defaults
func Load() (*Config, error) {
	cfg := Default()

	data, err := os.ReadFile(ConfigPath())
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return nil, fmt.Errorf("reading config: %w", err)
	}

	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}

	return cfg, nil
}

// Save writes the config to disk
func Save(cfg *Config) error {
	if err := os.MkdirAll(PierDir(), 0755); err != nil {
		return fmt.Errorf("creating pier directory: %w", err)
	}

	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshaling config: %w", err)
	}

	if err := os.WriteFile(ConfigPath(), data, 0644); err != nil {
		return fmt.Errorf("writing config: %w", err)
	}

	return nil
}

// Get retrieves a config value by dot-separated key
func (c *Config) Get(key string) (string, error) {
	switch strings.ToLower(key) {
	case "tld":
		return c.TLD, nil
	case "network":
		return c.Network, nil
	case "traefik.port":
		return fmt.Sprintf("%d", c.Traefik.Port), nil
	case "traefik.dashboard":
		return fmt.Sprintf("%t", c.Traefik.Dashboard), nil
	case "traefik.image":
		return c.Traefik.Image, nil
	case "nginx.managed":
		return fmt.Sprintf("%t", c.Nginx.Managed), nil
	case "nginx.valet_compatible":
		return fmt.Sprintf("%t", c.Nginx.ValetCompatible), nil
	default:
		return "", fmt.Errorf("unknown config key: %s", key)
	}
}

// Set updates a config value by dot-separated key
func (c *Config) Set(key, value string) error {
	switch strings.ToLower(key) {
	case "tld":
		c.TLD = value
	case "network":
		c.Network = value
	case "traefik.port":
		var port int
		if _, err := fmt.Sscanf(value, "%d", &port); err != nil {
			return fmt.Errorf("invalid port: %s", value)
		}
		c.Traefik.Port = port
	case "traefik.dashboard":
		c.Traefik.Dashboard = value == "true"
	case "traefik.image":
		c.Traefik.Image = value
	case "nginx.managed":
		c.Nginx.Managed = value == "true"
	case "nginx.valet_compatible":
		c.Nginx.ValetCompatible = value == "true"
	default:
		return fmt.Errorf("unknown config key: %s", key)
	}
	return nil
}

// EnsureDirectories creates all required directories
func EnsureDirectories() error {
	dirs := []string{
		PierDir(),
		TraefikDir(),
		TraefikDynamicDir(),
		NginxDir(),
		filepath.Join(PierDir(), "certs"),
		filepath.Join(PierDir(), "logs"),
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("creating directory %s: %w", dir, err)
		}
	}

	return nil
}
