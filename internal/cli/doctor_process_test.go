package cli

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProcessCommandDependencyChecksPathExecutables(t *testing.T) {
	lookup := func(name string) (string, error) {
		if name == "go" {
			return "/usr/bin/go", nil
		}
		return "", errors.New("not found")
	}

	result := processCommandDependency("go run .", t.TempDir(), lookup, os.Stat)

	if !result.OK {
		t.Fatalf("expected go command to be available: %#v", result)
	}
	if result.Name != "process dependency 'go'" {
		t.Fatalf("unexpected check name %q", result.Name)
	}
}

func TestProcessCommandDependencyReportsMissingPathExecutable(t *testing.T) {
	lookup := func(string) (string, error) {
		return "", errors.New("not found")
	}

	result := processCommandDependency("rails server -p 3000", t.TempDir(), lookup, os.Stat)

	if result.OK {
		t.Fatalf("expected missing rails dependency to fail: %#v", result)
	}
	if !strings.Contains(result.Fix, "Install rails") {
		t.Fatalf("expected install hint, got %q", result.Fix)
	}
}

func TestProcessCommandDependencyChecksProjectLocalWrappers(t *testing.T) {
	dir := t.TempDir()
	wrapper := filepath.Join(dir, "gradlew")
	if err := os.WriteFile(wrapper, []byte("#!/bin/sh\n"), 0755); err != nil {
		t.Fatalf("writing wrapper: %v", err)
	}
	lookup := func(string) (string, error) {
		return "", errors.New("not found")
	}

	result := processCommandDependency("./gradlew bootRun", dir, lookup, os.Stat)

	if !result.OK {
		t.Fatalf("expected local wrapper to be available: %#v", result)
	}
}

func TestProcessCommandDependencyReportsMissingProjectLocalWrapper(t *testing.T) {
	lookup := func(string) (string, error) {
		return "", errors.New("not found")
	}

	result := processCommandDependency("./mvnw spring-boot:run", t.TempDir(), lookup, os.Stat)

	if result.OK {
		t.Fatalf("expected missing Maven wrapper to fail: %#v", result)
	}
	if !strings.Contains(result.Fix, "Add ./mvnw") {
		t.Fatalf("expected wrapper hint, got %q", result.Fix)
	}
}

func TestProcessDoctorChecksReportsInferredCommandAndDependency(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "api/build.gradle", `plugins { id "org.springframework.boot" version "3.3.0" }`)
	writeTestFile(t, dir, "docker-compose.yml", `services:
  api:
    build: ./api
`)
	lookup := func(string) (string, error) {
		return "", errors.New("not found")
	}

	checks, err := processDoctorChecks(dir, lookup, os.Stat)
	if err != nil {
		t.Fatalf("processDoctorChecks returned error: %v", err)
	}

	var sawCommand bool
	var sawMissingWrapper bool
	for _, check := range checks {
		if strings.HasSuffix(check.Name, " process command") && check.OK && check.Detail == "./gradlew bootRun" {
			sawCommand = true
		}
		if check.Name == "process dependency './gradlew'" && !check.OK && strings.Contains(check.Fix, "Add ./gradlew") {
			sawMissingWrapper = true
		}
	}
	if !sawCommand {
		t.Fatalf("missing inferred command check: %#v", checks)
	}
	if !sawMissingWrapper {
		t.Fatalf("missing wrapper dependency failure: %#v", checks)
	}
}
