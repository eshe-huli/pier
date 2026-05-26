package cli

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/eshe-huli/pier/internal/config"
	"github.com/eshe-huli/pier/internal/orchestrator"
	"github.com/eshe-huli/pier/internal/planner"
)

func TestRunUpComposeProcessRuntimeAllowsCommandBackedApp(t *testing.T) {
	dir := t.TempDir()
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeTestFile(t, dir, "docker-compose.yml", `services:
  web:
    image: node:20
    command: "true"
    ports:
      - "3000:3000"
`)

	plan, err := planner.PlanProject(dir)
	if err != nil {
		t.Fatalf("PlanProject returned error: %v", err)
	}

	cfg := config.Default()
	if err := runUpCompose(
		context.Background(),
		dir,
		plan,
		cfg,
		orchestrator.LocalProcessAdapter{},
	); err != nil {
		t.Fatalf("runUpCompose returned error: %v", err)
	}

	projectName := filepath.Base(filepath.Clean(dir))
	if _, err := os.Stat(filepath.Join(home, ".pier", "links", projectName+".json")); err != nil {
		t.Fatalf("expected process link metadata: %v", err)
	}
	if _, err := os.Stat(filepath.Join(home, ".pier", "traefik", "dynamic", projectName+".yaml")); err != nil {
		t.Fatalf("expected process proxy config: %v", err)
	}
}
