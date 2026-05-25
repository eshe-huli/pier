package proxy

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/eshe-huli/pier/internal/config"
)

func TestGenerateComposeFileGroupsPierInfrastructure(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	cfg := config.Default()
	cfg.Network = "pier-test"
	cfg.Traefik.Port = 8890

	composePath, err := generateComposeFile(cfg)
	if err != nil {
		t.Fatalf("generateComposeFile returned error: %v", err)
	}

	wantPath := filepath.Join(home, ".pier", "docker-compose.yml")
	if composePath != wantPath {
		t.Fatalf("got compose path %q, want %q", composePath, wantPath)
	}

	data, err := os.ReadFile(composePath)
	if err != nil {
		t.Fatalf("reading compose file: %v", err)
	}
	compose := string(data)

	assertContains(t, compose, "name: pier")
	assertContains(t, compose, "container_name: pier-traefik")
	assertContains(t, compose, `- "8890:80"`)
	assertContains(t, compose, `- "8891:8080"`)
	assertContains(t, compose, "external: true")
	assertContains(t, compose, "pier-test:")
	assertContains(t, compose, filepath.Join(home, ".pier", "traefik", "traefik.yaml"))
	assertContains(t, compose, filepath.Join(home, ".pier", "traefik", "dynamic"))
}

func TestGenerateComposeFileCanBindTraefikDirectlyToPort80(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	cfg := config.Default()
	cfg.Network = "pier-test"
	cfg.Traefik.Port = 8890
	cfg.Nginx.Managed = false

	composePath, err := generateComposeFile(cfg)
	if err != nil {
		t.Fatalf("generateComposeFile returned error: %v", err)
	}

	data, err := os.ReadFile(composePath)
	if err != nil {
		t.Fatalf("reading compose file: %v", err)
	}
	compose := string(data)

	assertContains(t, compose, `- "80:80"`)
	assertContains(t, compose, `- "8891:8080"`)
	if WebHostPort(cfg) != 80 {
		t.Fatalf("got web host port %d, want 80", WebHostPort(cfg))
	}
	if EdgeDescription(cfg) != "Traefik direct :80" {
		t.Fatalf("got edge description %q", EdgeDescription(cfg))
	}
}

func TestComposeManagedLabels(t *testing.T) {
	tests := []struct {
		name   string
		labels map[string]string
		want   bool
	}{
		{
			name: "compose managed pier traefik",
			labels: map[string]string{
				composeProjectLabel: composeProjectName,
				composeServiceLabel: "traefik",
			},
			want: true,
		},
		{
			name: "legacy pier traefik",
			labels: map[string]string{
				"pier.domain": "traefik",
			},
			want: false,
		},
		{
			name: "different compose project",
			labels: map[string]string{
				composeProjectLabel: "other",
				composeServiceLabel: "traefik",
			},
			want: false,
		},
		{
			name:   "nil labels",
			labels: nil,
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isComposeManagedLabels(tt.labels)
			if got != tt.want {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func assertContains(t *testing.T, haystack, needle string) {
	t.Helper()
	if !strings.Contains(haystack, needle) {
		t.Fatalf("expected compose file to contain %q\n\n%s", needle, haystack)
	}
}
