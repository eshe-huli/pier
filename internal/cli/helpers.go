package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/eshe-huli/pier/internal/pierfile"
)

// resolveProjectName returns the project name from args, Pierfile, or directory name.
func resolveProjectName(args []string) (string, error) {
	if len(args) > 0 {
		return args[0], nil
	}

	dir, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("getting working directory: %w", err)
	}

	name := filepath.Base(dir)
	if pierfile.Exists(dir) {
		pf, err := pierfile.Load(dir)
		if err == nil && pf.Name != "" {
			name = pf.Name
		}
	}

	return name, nil
}
