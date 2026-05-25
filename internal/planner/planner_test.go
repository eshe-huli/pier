package planner

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/eshe-huli/pier/internal/manifest"
)

func TestPlanProject_ComposeSplitsInfraAndApps(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "docker-compose.yml", `services:
  api:
    build: .
    ports:
      - "3000:3000"
  postgres:
    image: postgres:16-alpine
  redis:
    image: redis:7
`)

	plan, err := PlanProject(dir)
	if err != nil {
		t.Fatalf("PlanProject returned error: %v", err)
	}

	if plan.Source != SourceCompose {
		t.Fatalf("got source %q, want %q", plan.Source, SourceCompose)
	}
	if len(plan.AppServices) != 1 {
		t.Fatalf("got %d app services, want 1", len(plan.AppServices))
	}
	if plan.AppServices[0].ComposeName != "api" {
		t.Fatalf("got app service %q, want api", plan.AppServices[0].ComposeName)
	}
	assertServiceSpec(t, plan.ServiceSpecs, "postgres:16")
	assertServiceSpec(t, plan.ServiceSpecs, "redis:7")
}

func TestPlanProject_ComposeRendersAppPlan(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "docker-compose.yml", `services:
  api:
    build:
      context: ./api
      dockerfile: Dockerfile.dev
    ports:
      - "8080:3000/tcp"
    environment:
      APP_ENV: local
    volumes:
      - ./api:/srv/api:cached
      - named-cache:/cache
  postgres:
    image: postgres:16-alpine
`)

	plan, err := PlanProject(dir)
	if err != nil {
		t.Fatalf("PlanProject returned error: %v", err)
	}

	if len(plan.Apps) != 1 {
		t.Fatalf("got %d app plans, want 1", len(plan.Apps))
	}

	app := plan.Apps[0]
	if app.Name != filepath.Base(filepath.Clean(dir)) {
		t.Fatalf("got app name %q, want project directory name", app.Name)
	}
	if app.ComposeName != "api" {
		t.Fatalf("got compose name %q, want api", app.ComposeName)
	}
	if app.BuildContext != filepath.Join(dir, "api") {
		t.Fatalf("got build context %q, want %q", app.BuildContext, filepath.Join(dir, "api"))
	}
	if app.Dockerfile != filepath.Join(dir, "api", "Dockerfile.dev") {
		t.Fatalf("got dockerfile %q, want %q", app.Dockerfile, filepath.Join(dir, "api", "Dockerfile.dev"))
	}
	if app.Port != 3000 {
		t.Fatalf("got port %d, want 3000", app.Port)
	}
	if app.Env["APP_ENV"] != "local" {
		t.Fatalf("got APP_ENV %q, want local", app.Env["APP_ENV"])
	}
	if !app.UseEnvFile {
		t.Fatal("expected compose build app to use generated env file")
	}
	wantVolume := filepath.Join(dir, "api") + ":/srv/api:cached"
	if len(app.Volumes) != 1 || app.Volumes[0] != wantVolume {
		t.Fatalf("got volumes %#v, want [%q]", app.Volumes, wantVolume)
	}
}

func TestPlanProject_ComposeInfraOnlyRendersDefaultAppPlan(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "Dockerfile", "FROM alpine\n")
	writeFile(t, dir, "Pierfile", `name: api
port: 8080
`)
	writeFile(t, dir, "docker-compose.yml", `services:
  postgres:
    image: postgres:16-alpine
`)

	plan, err := PlanProject(dir)
	if err != nil {
		t.Fatalf("PlanProject returned error: %v", err)
	}

	if len(plan.AppServices) != 0 {
		t.Fatalf("got %d compose app services, want 0", len(plan.AppServices))
	}
	if len(plan.Apps) != 1 {
		t.Fatalf("got %d app plans, want 1", len(plan.Apps))
	}

	app := plan.Apps[0]
	if app.Name != "api" {
		t.Fatalf("got app name %q, want api", app.Name)
	}
	if app.Port != 8080 {
		t.Fatalf("got port %d, want 8080", app.Port)
	}
}

func TestPlanProject_PierfileDefaultsMissingVersions(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "Pierfile", `name: api
services:
  - redis
  - postgres:15
`)

	plan, err := PlanProject(dir)
	if err != nil {
		t.Fatalf("PlanProject returned error: %v", err)
	}

	if plan.ProjectName != "api" {
		t.Fatalf("got project name %q, want api", plan.ProjectName)
	}
	if plan.Source != SourcePierfile {
		t.Fatalf("got source %q, want %q", plan.Source, SourcePierfile)
	}
	assertServiceSpec(t, plan.ServiceSpecs, "redis:7")
	assertServiceSpec(t, plan.ServiceSpecs, "postgres:15")
}

func TestPlanProject_PierfileRendersDefaultAppPlan(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "Dockerfile", "FROM alpine\n")
	writeFile(t, dir, "Pierfile", `name: api
port: 8088
env:
  APP_ENV: local
`)

	plan, err := PlanProject(dir)
	if err != nil {
		t.Fatalf("PlanProject returned error: %v", err)
	}

	if len(plan.Apps) != 1 {
		t.Fatalf("got %d app plans, want 1", len(plan.Apps))
	}

	app := plan.Apps[0]
	if app.Name != "api" {
		t.Fatalf("got app name %q, want api", app.Name)
	}
	if app.BuildContext != dir {
		t.Fatalf("got build context %q, want %q", app.BuildContext, dir)
	}
	if app.Dockerfile != filepath.Join(dir, "Dockerfile") {
		t.Fatalf("got dockerfile %q, want %q", app.Dockerfile, filepath.Join(dir, "Dockerfile"))
	}
	if app.Port != 8088 {
		t.Fatalf("got port %d, want 8088", app.Port)
	}
	if app.Env["APP_ENV"] != "local" {
		t.Fatalf("got APP_ENV %q, want local", app.Env["APP_ENV"])
	}
}

func TestPlanProject_ManifestOverridesDetection(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "package.json", `{
  "dependencies": {
    "pg": "^8.0.0",
    "ioredis": "^5.0.0"
  }
}`)
	writeFile(t, dir, ".pier/manifest.yaml", `version: 1
source: detection
project:
  name: api
services:
  - name: postgres
    version: "15"
apps:
  - name: api
    port: 8088
    overrides:
      env:
        APP_ENV: local
`)

	plan, err := PlanProject(dir)
	if err != nil {
		t.Fatalf("PlanProject returned error: %v", err)
	}

	if plan.Source != SourceManifest {
		t.Fatalf("got source %q, want %q", plan.Source, SourceManifest)
	}
	if plan.ProjectName != "api" {
		t.Fatalf("got project name %q, want api", plan.ProjectName)
	}
	assertServiceSpec(t, plan.ServiceSpecs, "postgres:15")
	assertNoServiceSpec(t, plan.ServiceSpecs, "redis:7")

	if len(plan.Apps) != 1 {
		t.Fatalf("got %d app plans, want 1", len(plan.Apps))
	}
	app := plan.Apps[0]
	if app.Name != "api" {
		t.Fatalf("got app name %q, want api", app.Name)
	}
	if app.Port != 8088 {
		t.Fatalf("got port %d, want 8088", app.Port)
	}
	if app.Env["APP_ENV"] != "local" {
		t.Fatalf("got APP_ENV %q, want local", app.Env["APP_ENV"])
	}
}

func TestPlanProject_ManifestAppOverridesDoNotSuppressDetection(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "package.json", `{
  "dependencies": {
    "pg": "^8.0.0"
  }
}`)
	writeFile(t, dir, ".pier/manifest.yaml", `version: 1
project:
  name: api
apps:
  - name: api
    port: 8088
`)

	plan, err := PlanProject(dir)
	if err != nil {
		t.Fatalf("PlanProject returned error: %v", err)
	}

	if plan.Source != SourceDetection {
		t.Fatalf("got source %q, want %q", plan.Source, SourceDetection)
	}
	assertServiceSpec(t, plan.ServiceSpecs, "postgres:16")
	if len(plan.Apps) != 1 || plan.Apps[0].Port != 8088 {
		t.Fatalf("got app plans %#v, want port 8088", plan.Apps)
	}
}

func TestPlanProject_DetectsFrameworkAndServiceFallback(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "package.json", `{
  "dependencies": {
    "@nestjs/core": "^11.0.0",
    "pg": "^8.0.0",
    "ioredis": "^5.0.0"
  }
}`)

	plan, err := PlanProject(dir)
	if err != nil {
		t.Fatalf("PlanProject returned error: %v", err)
	}

	if plan.Source != SourceDetection {
		t.Fatalf("got source %q, want %q", plan.Source, SourceDetection)
	}
	if plan.Framework == nil || plan.Framework.Name != "nestjs" {
		t.Fatalf("got framework %#v, want nestjs", plan.Framework)
	}
	assertServiceSpec(t, plan.ServiceSpecs, "postgres:16")
	assertServiceSpec(t, plan.ServiceSpecs, "redis:7")
}

func TestSaveManifestWritesPlanSummary(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "Pierfile", `name: api
services:
  - postgres:15
port: 8088
env:
  APP_ENV: local
`)

	plan, err := PlanProject(dir)
	if err != nil {
		t.Fatalf("PlanProject returned error: %v", err)
	}
	if err := SaveManifest(plan); err != nil {
		t.Fatalf("SaveManifest returned error: %v", err)
	}

	got, err := manifest.Load(dir)
	if err != nil {
		t.Fatalf("manifest.Load returned error: %v", err)
	}

	if got.Source != "pierfile" {
		t.Fatalf("got source %q, want pierfile", got.Source)
	}
	if got.Project.Name != "api" {
		t.Fatalf("got project name %q, want api", got.Project.Name)
	}
	if len(got.Services) != 1 || got.Services[0].Name != "postgres" || got.Services[0].Version != "15" {
		t.Fatalf("got services %#v, want postgres:15", got.Services)
	}
	if len(got.Apps) != 1 || got.Apps[0].Port != 8088 {
		t.Fatalf("got apps %#v, want port 8088", got.Apps)
	}
	if got.Apps[0].Overrides.Env["APP_ENV"] != "local" {
		t.Fatalf("got APP_ENV %q, want local", got.Apps[0].Overrides.Env["APP_ENV"])
	}
}

func TestRuntimeEnvAddsDatabaseIdentity(t *testing.T) {
	got := RuntimeEnv("my-api", []string{"REDIS_URL=redis://localhost:6379"})
	want := []string{
		"REDIS_URL=redis://localhost:6379",
		"DATABASE_NAME=my_api",
		"DB_DATABASE=my_api",
	}
	if len(got) != len(want) {
		t.Fatalf("got %d env vars, want %d: %v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got env[%d] %q, want %q", i, got[i], want[i])
		}
	}
}

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("creating parent directory for %s: %v", name, err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("writing %s: %v", name, err)
	}
}

func assertServiceSpec(t *testing.T, specs []string, want string) {
	t.Helper()
	for _, spec := range specs {
		if spec == want {
			return
		}
	}
	t.Fatalf("expected service spec %q in %v", want, specs)
}

func assertNoServiceSpec(t *testing.T, specs []string, want string) {
	t.Helper()
	for _, spec := range specs {
		if spec == want {
			t.Fatalf("did not expect service spec %q in %v", want, specs)
		}
	}
}
