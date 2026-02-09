package runtime

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// GenerateEnvFile reads the project's .env file, applies pier overrides,
// and writes to .pier/env. Returns the path to the generated file.
// If no .env exists, creates .pier/env with just the overrides.
func GenerateEnvFile(projectDir string, overrides []string) (string, error) {
	pierDir := filepath.Join(projectDir, ".pier")
	if err := os.MkdirAll(pierDir, 0755); err != nil {
		return "", fmt.Errorf("creating .pier directory: %w", err)
	}

	outPath := filepath.Join(pierDir, "env")

	// Parse overrides into map
	overrideMap := make(map[string]string)
	for _, o := range overrides {
		parts := strings.SplitN(o, "=", 2)
		if len(parts) == 2 {
			overrideMap[parts[0]] = parts[1]
		}
	}

	// Read project .env if it exists
	var lines []string
	seen := make(map[string]bool)

	envFile := filepath.Join(projectDir, ".env")
	if f, err := os.Open(envFile); err == nil {
		defer f.Close()
		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			line := scanner.Text()
			trimmed := strings.TrimSpace(line)

			// Pass through comments and blank lines
			if trimmed == "" || strings.HasPrefix(trimmed, "#") {
				lines = append(lines, line)
				continue
			}

			parts := strings.SplitN(trimmed, "=", 2)
			if len(parts) != 2 {
				lines = append(lines, line)
				continue
			}

			key := strings.TrimSpace(parts[0])
			seen[key] = true

			// If pier has an override, use it
			if val, ok := overrideMap[key]; ok {
				lines = append(lines, fmt.Sprintf("%s=%s", key, val))
			} else {
				// Resolve ${VAR:-default} patterns to just the default
				val := strings.TrimSpace(parts[1])
				val = resolveDefault(val)
				lines = append(lines, fmt.Sprintf("%s=%s", key, val))
			}
		}
	}

	// Append any overrides not already in the file
	lines = append(lines, "", "# Pier infrastructure overrides")
	for _, o := range overrides {
		parts := strings.SplitN(o, "=", 2)
		if len(parts) == 2 && !seen[parts[0]] {
			lines = append(lines, o)
		}
	}

	content := strings.Join(lines, "\n") + "\n"
	if err := os.WriteFile(outPath, []byte(content), 0644); err != nil {
		return "", fmt.Errorf("writing .pier/env: %w", err)
	}

	return outPath, nil
}

// resolveDefault resolves ${VAR:-default} to just "default"
func resolveDefault(val string) string {
	if !strings.Contains(val, "${") {
		return val
	}
	// Handle ${VAR:-default} pattern
	result := val
	for {
		start := strings.Index(result, "${")
		if start < 0 {
			break
		}
		end := strings.Index(result[start:], "}")
		if end < 0 {
			break
		}
		end += start

		inner := result[start+2 : end]
		replacement := ""

		if idx := strings.Index(inner, ":-"); idx >= 0 {
			replacement = inner[idx+2:]
		}

		result = result[:start] + replacement + result[end+1:]
	}
	return result
}
