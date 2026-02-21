package compose

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParse_BasicCompose(t *testing.T) {
	dir := t.TempDir()
	content := `version: "3.8"
services:
  api:
    build: ./api
    ports:
      - "3000:3000"
  db:
    image: postgres:16
    environment:
      POSTGRES_PASSWORD: secret
`
	if err := os.WriteFile(filepath.Join(dir, "docker-compose.yml"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	cf, err := Parse(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cf.Services) != 2 {
		t.Fatalf("got %d services, want 2", len(cf.Services))
	}
}

func TestParse_NoComposeFile(t *testing.T) {
	dir := t.TempDir()

	_, err := Parse(dir)
	if err == nil {
		t.Error("expected error for missing compose file, got nil")
	}
}

func TestSeparateServices_InfraVsApp(t *testing.T) {
	cf := &ComposeFile{
		Services: map[string]ComposeService{
			"api": {Build: "./api", Ports: []string{"3000:3000"}},
			"db":  {Image: "postgres:16"},
			"redis": {Image: "redis:7-alpine"},
			"web": {Build: "./web", Ports: []string{"5173:5173"}},
		},
	}

	infra, apps := SeparateServices(cf)

	if len(infra) != 2 {
		t.Errorf("got %d infra services, want 2 (db + redis)", len(infra))
	}
	if len(apps) != 2 {
		t.Errorf("got %d app services, want 2 (api + web)", len(apps))
	}

	// Verify infra services are the image-based ones
	infraNames := make(map[string]bool)
	for _, s := range infra {
		infraNames[s.Name] = true
	}
	if !infraNames["postgres"] && !infraNames["redis"] {
		t.Errorf("expected postgres and redis in infra, got %v", infraNames)
	}
}

func TestParseFirstPort(t *testing.T) {
	tests := []struct {
		ports []string
		want  int
	}{
		{nil, 0},
		{[]string{}, 0},
		{[]string{"3000:3000"}, 3000},
		{[]string{"8080:3000"}, 3000},
		{[]string{"3000:3000/tcp"}, 3000},
		{[]string{"3000"}, 3000},
	}

	for _, tt := range tests {
		got := ParseFirstPort(tt.ports)
		if got != tt.want {
			t.Errorf("ParseFirstPort(%v) = %d, want %d", tt.ports, got, tt.want)
		}
	}
}
