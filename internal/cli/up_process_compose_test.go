package cli

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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

func TestRunUpComposeProcessRuntimeRunsCommandFromBuildContext(t *testing.T) {
	dir := t.TempDir()
	home := t.TempDir()
	observedCWD := filepath.Join(t.TempDir(), "cwd.txt")
	t.Setenv("HOME", home)
	writeTestFile(t, dir, "api/package.json", `{"scripts":{"dev":"node server.js"}}`)
	writeTestFile(t, dir, "docker-compose.yml", `services:
  api:
    build: ./api
    command: "pwd > `+observedCWD+`"
    ports:
      - "4000:4000"
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

	got := cleanRealPath(t, waitForTrimmedFile(t, observedCWD))
	want := cleanRealPath(t, filepath.Join(dir, "api"))
	if got != want {
		t.Fatalf("expected process command to run from build context %q, got %q", want, got)
	}
}

func cleanRealPath(t *testing.T, path string) string {
	t.Helper()

	realPath, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatalf("resolving real path %s: %v", path, err)
	}
	return filepath.Clean(realPath)
}

func waitForTrimmedFile(t *testing.T, path string) string {
	t.Helper()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		data, err := os.ReadFile(path)
		if err == nil {
			return strings.TrimSpace(string(data))
		}
		time.Sleep(10 * time.Millisecond)
	}

	t.Fatalf("timed out waiting for %s", path)
	return ""
}
