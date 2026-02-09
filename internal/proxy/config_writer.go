package proxy

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/eshe-huli/pier/internal/config"
	"gopkg.in/yaml.v3"
)

// WriteTraefikConfig writes a Traefik dynamic config file
func WriteTraefikConfig(name string, cfg map[string]interface{}) error {
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshaling config: %w", err)
	}

	filePath := filepath.Join(config.TraefikDynamicDir(), name+".yaml")
	if err := os.WriteFile(filePath, data, 0644); err != nil {
		return fmt.Errorf("writing config: %w", err)
	}

	return nil
}
