package proxy

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/eshe-huli/pier/internal/config"
	"gopkg.in/yaml.v3"
)

// FileProxy represents a bare-metal proxy entry
type FileProxy struct {
	Name   string
	Port   int
	Domain string
}

// CreateFileProxy creates a Traefik dynamic config file for a bare-metal proxy
func CreateFileProxy(name string, port int, tld string) error {
	domain := fmt.Sprintf("%s.%s", name, tld)

	// Build the YAML structure
	cfg := map[string]interface{}{
		"http": map[string]interface{}{
			"routers": map[string]interface{}{
				name: map[string]interface{}{
					"rule":    fmt.Sprintf("Host(`%s`)", domain),
					"service": name,
					"entryPoints": []string{
						"web",
					},
				},
			},
			"services": map[string]interface{}{
				name: map[string]interface{}{
					"loadBalancer": map[string]interface{}{
						"servers": []map[string]interface{}{
							{
								"url": fmt.Sprintf("http://host.docker.internal:%d", port),
							},
						},
					},
				},
			},
		},
	}

	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshaling proxy config: %w", err)
	}

	filePath := filepath.Join(config.TraefikDynamicDir(), name+".yaml")
	if err := os.WriteFile(filePath, data, 0644); err != nil {
		return fmt.Errorf("writing proxy config: %w", err)
	}

	return nil
}

// RemoveFileProxy removes a Traefik dynamic config file
func RemoveFileProxy(name string) error {
	filePath := filepath.Join(config.TraefikDynamicDir(), name+".yaml")

	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return fmt.Errorf("proxy '%s' not found", name)
	}

	if err := os.Remove(filePath); err != nil {
		return fmt.Errorf("removing proxy config: %w", err)
	}

	return nil
}

// ListFileProxies reads all file-based proxy configurations
func ListFileProxies(tld string) ([]FileProxy, error) {
	dynamicDir := config.TraefikDynamicDir()

	entries, err := os.ReadDir(dynamicDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading dynamic config directory: %w", err)
	}

	var proxies []FileProxy
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".yaml") {
			continue
		}

		name := strings.TrimSuffix(entry.Name(), ".yaml")
		proxy := FileProxy{
			Name:   name,
			Domain: fmt.Sprintf("%s.%s", name, tld),
		}

		// Try to parse the port from the file
		data, err := os.ReadFile(filepath.Join(dynamicDir, entry.Name()))
		if err == nil {
			proxy.Port = extractPort(data)
		}

		proxies = append(proxies, proxy)
	}

	return proxies, nil
}

// FileProxyExists checks if a file proxy exists
func FileProxyExists(name string) bool {
	filePath := filepath.Join(config.TraefikDynamicDir(), name+".yaml")
	_, err := os.Stat(filePath)
	return err == nil
}

// IsProxyBackendAlive checks if a port is listening on localhost
func IsProxyBackendAlive(port int) bool {
	if port == 0 {
		return true // can't check, assume alive
	}
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 500*time.Millisecond)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

func extractPort(data []byte) int {
	// Simple extraction — look for host.docker.internal:<port>
	content := string(data)
	idx := strings.Index(content, "host.docker.internal:")
	if idx == -1 {
		return 0
	}

	portStr := ""
	for i := idx + len("host.docker.internal:"); i < len(content); i++ {
		c := content[i]
		if c >= '0' && c <= '9' {
			portStr += string(c)
		} else {
			break
		}
	}

	var port int
	fmt.Sscanf(portStr, "%d", &port)
	return port
}
