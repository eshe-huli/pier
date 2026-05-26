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
	"github.com/eshe-huli/pier/internal/registry"
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

func TestRunUpComposeProcessRuntimeInfersKnownFrameworkCommand(t *testing.T) {
	dir := t.TempDir()
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeTestFile(t, dir, "api/package.json", `{
  "scripts": { "start:dev": "true" },
  "dependencies": { "@nestjs/core": "10.0.0" }
}`)
	writeTestFile(t, dir, "docker-compose.yml", `services:
  api:
    build: ./api
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
	meta, found, err := orchestrator.ResolveLocalProcessMeta(projectName)
	if err != nil {
		t.Fatalf("ResolveLocalProcessMeta returned error: %v", err)
	}
	if !found {
		t.Fatalf("expected local process metadata for %s", projectName)
	}
	if meta.Command != "npm run start:dev" {
		t.Fatalf("expected inferred NestJS command, got %q", meta.Command)
	}
	if meta.Port != 3000 {
		t.Fatalf("expected inferred NestJS port 3000, got %d", meta.Port)
	}
}

func TestRunUpComposeProcessRuntimeRejectsUnknownCommandlessApp(t *testing.T) {
	dir := t.TempDir()
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeTestFile(t, dir, "docker-compose.yml", `services:
  api:
    image: busybox
    ports:
      - "3000:3000"
`)

	plan, err := planner.PlanProject(dir)
	if err != nil {
		t.Fatalf("PlanProject returned error: %v", err)
	}

	cfg := config.Default()
	err = runUpCompose(
		context.Background(),
		dir,
		plan,
		cfg,
		orchestrator.LocalProcessAdapter{},
	)
	if err == nil {
		t.Fatal("expected commandless process-mode compose app to be rejected")
	}
	if !strings.Contains(err.Error(), "requires a known dev command") {
		t.Fatalf("expected known dev command error, got %v", err)
	}
}

func TestRunUpComposeProcessRuntimeKeepsMultiAppState(t *testing.T) {
	dir := t.TempDir()
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeTestFile(t, dir, "docker-compose.yml", `services:
  api:
    image: node:20
    command: "true"
    ports:
      - "4000:4000"
  worker:
    image: node:20
    command: "true"
    ports:
      - "5000:5000"
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

	for _, name := range []string{"api", "worker"} {
		if _, err := os.Stat(filepath.Join(home, ".pier", "links", name+".json")); err != nil {
			t.Fatalf("expected %s link metadata: %v", name, err)
		}
		if _, err := os.Stat(filepath.Join(home, ".pier", "traefik", "dynamic", name+".yaml")); err != nil {
			t.Fatalf("expected %s proxy config: %v", name, err)
		}
		if _, err := os.Stat(filepath.Join(dir, ".pier", name+".pid")); err != nil {
			t.Fatalf("expected %s app-scoped pid file: %v", name, err)
		}
		if _, err := os.Stat(filepath.Join(dir, ".pier", name+".log")); err != nil {
			t.Fatalf("expected %s app-scoped log file: %v", name, err)
		}
	}

	projects, err := registry.Load()
	if err != nil {
		t.Fatalf("loading registry: %v", err)
	}
	seen := map[string]bool{}
	for _, project := range projects {
		if project.Dir == dir {
			seen[project.Name] = true
		}
	}
	for _, name := range []string{"api", "worker"} {
		if !seen[name] {
			t.Fatalf("registry missing %s app entry: %#v", name, projects)
		}
	}
}

func TestRunUpComposeProcessRuntimeHonorsComposeExecutionDetails(t *testing.T) {
	dir := t.TempDir()
	home := t.TempDir()
	observedCWD := filepath.Join(t.TempDir(), "cwd.txt")
	observedEnv := filepath.Join(t.TempDir(), "env.txt")
	t.Setenv("HOME", home)
	writeTestFile(t, dir, "api/.env.local", "APP_ENV=from-env-file\n")
	writeTestFile(t, dir, "docker-compose.yml", `services:
  api:
    build: .
    working_dir: ./api
    env_file:
      - ./api/.env.local
    command: "pwd > `+observedCWD+` && printf %s \"$APP_ENV\" > `+observedEnv+`"
    expose:
      - "4010"
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

	gotCWD := cleanRealPath(t, waitForTrimmedFile(t, observedCWD))
	wantCWD := cleanRealPath(t, filepath.Join(dir, "api"))
	if gotCWD != wantCWD {
		t.Fatalf("expected process command to run from working_dir %q, got %q", wantCWD, gotCWD)
	}

	if gotEnv := waitForTrimmedFile(t, observedEnv); gotEnv != "from-env-file" {
		t.Fatalf("expected env_file variable %q, got %q", "from-env-file", gotEnv)
	}

	projectName := filepath.Base(filepath.Clean(dir))
	meta, found, err := orchestrator.ResolveLocalProcessMeta(projectName)
	if err != nil {
		t.Fatalf("ResolveLocalProcessMeta returned error: %v", err)
	}
	if !found {
		t.Fatalf("expected local process metadata for %s", projectName)
	}
	if meta.Port != 4010 {
		t.Fatalf("expected exposed port 4010, got %d", meta.Port)
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
