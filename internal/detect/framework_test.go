package detect

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDetectFramework_NestJS(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "package.json", `{"dependencies":{"@nestjs/core":"^10.0.0"}}`)

	fw, err := DetectFramework(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fw.Name != "nestjs" {
		t.Errorf("got framework %q, want nestjs", fw.Name)
	}
	if fw.Port != 3000 {
		t.Errorf("got port %d, want 3000", fw.Port)
	}
}

func TestDetectFramework_NextJS(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "package.json", `{"dependencies":{"next":"14.0.0"}}`)

	fw, err := DetectFramework(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fw.Name != "nextjs" {
		t.Errorf("got framework %q, want nextjs", fw.Name)
	}
}

func TestDetectFramework_Laravel(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "composer.json", `{"require":{"laravel/framework":"^11.0"}}`)

	fw, err := DetectFramework(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fw.Name != "laravel" {
		t.Errorf("got framework %q, want laravel", fw.Name)
	}
	if fw.Port != 8000 {
		t.Errorf("got port %d, want 8000", fw.Port)
	}
}

func TestDetectFramework_Go(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "go.mod", `module example.com/myapp

go 1.21`)
	writeFile(t, dir, "main.go", `package main

func main() {}`)

	fw, err := DetectFramework(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fw.Name != "go" {
		t.Errorf("got framework %q, want go", fw.Name)
	}
	if fw.Port != 8080 {
		t.Errorf("got port %d, want 8080", fw.Port)
	}
}

func TestDetectFramework_NoFramework(t *testing.T) {
	dir := t.TempDir()

	_, err := DetectFramework(dir)
	if err == nil {
		t.Error("expected error for empty directory, got nil")
	}
}

func TestDetectFramework_Priority(t *testing.T) {
	// NestJS should take priority over Express
	dir := t.TempDir()
	writeFile(t, dir, "package.json", `{"dependencies":{"@nestjs/core":"^10.0.0","express":"^4.0.0"}}`)

	fw, err := DetectFramework(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fw.Name != "nestjs" {
		t.Errorf("got framework %q, want nestjs (should take priority over express)", fw.Name)
	}
}

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("writing %s: %v", name, err)
	}
}
