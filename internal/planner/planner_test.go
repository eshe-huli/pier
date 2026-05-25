package planner

import (
	"os"
	"path/filepath"
	"testing"
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

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	path := filepath.Join(dir, name)
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
