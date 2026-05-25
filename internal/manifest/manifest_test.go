package manifest

import (
	"path/filepath"
	"testing"
)

func TestSaveAndLoad(t *testing.T) {
	dir := t.TempDir()
	input := &Manifest{
		Source: "detection",
		Project: Project{
			Name: "api",
		},
		Services: []Service{
			{Name: "postgres", Version: "16"},
			{Name: "redis", Version: "7"},
		},
		Apps: []App{
			{
				Name: "api",
				Port: 3000,
				Overrides: Overrides{
					Env: map[string]string{
						"APP_ENV": "local",
					},
				},
			},
		},
	}

	if err := Save(dir, input); err != nil {
		t.Fatalf("Save returned error: %v", err)
	}

	if !Exists(dir) {
		t.Fatal("expected manifest to exist")
	}
	if Path(dir) != filepath.Join(dir, ".pier", "manifest.yaml") {
		t.Fatalf("got path %q", Path(dir))
	}

	got, err := Load(dir)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	if got.Version != Version {
		t.Fatalf("got version %d, want %d", got.Version, Version)
	}
	if got.Project.Name != "api" {
		t.Fatalf("got project name %q, want api", got.Project.Name)
	}
	if len(got.Services) != 2 || got.Services[0].Name != "postgres" {
		t.Fatalf("got services %#v", got.Services)
	}
	if len(got.Apps) != 1 || got.Apps[0].Port != 3000 {
		t.Fatalf("got apps %#v", got.Apps)
	}
	if got.Apps[0].Overrides.Env["APP_ENV"] != "local" {
		t.Fatalf("got APP_ENV %q, want local", got.Apps[0].Overrides.Env["APP_ENV"])
	}
}
