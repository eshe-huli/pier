package orchestrator

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/eshe-huli/pier/internal/config"
	"github.com/eshe-huli/pier/internal/detect"
	"github.com/eshe-huli/pier/internal/registry"
)

type fakeRuntimeAdapter struct {
	buildCalled bool
	runCalled   bool
}

func (a *fakeRuntimeAdapter) Name() string {
	return "fake"
}

func (a *fakeRuntimeAdapter) BuildImage(_ context.Context, _ AppSpec) (string, int, error) {
	a.buildCalled = true
	return "fake-image", 4242, nil
}

func (a *fakeRuntimeAdapter) RunApp(_ context.Context, _ AppSpec, _ string, _ int, _ *config.Config, _ []string) error {
	a.runCalled = true
	return nil
}

func TestDefaultRuntimeAdapterIsDocker(t *testing.T) {
	if DefaultRuntimeAdapter.Name() != "docker" {
		t.Fatalf("default runtime adapter = %q", DefaultRuntimeAdapter.Name())
	}
}

func TestBuildImageUsesDefaultRuntimeAdapter(t *testing.T) {
	previous := DefaultRuntimeAdapter
	fake := &fakeRuntimeAdapter{}
	DefaultRuntimeAdapter = fake
	defer func() { DefaultRuntimeAdapter = previous }()

	image, port, err := BuildImage(context.Background(), AppSpec{Name: "api"})
	if err != nil {
		t.Fatalf("BuildImage returned error: %v", err)
	}
	if !fake.buildCalled {
		t.Fatal("fake runtime adapter was not called")
	}
	if image != "fake-image" || port != 4242 {
		t.Fatalf("image/port = %q/%d", image, port)
	}
}

func TestRunContainerUsesDefaultRuntimeAdapter(t *testing.T) {
	previous := DefaultRuntimeAdapter
	fake := &fakeRuntimeAdapter{}
	DefaultRuntimeAdapter = fake
	defer func() { DefaultRuntimeAdapter = previous }()

	err := RunContainer(
		context.Background(),
		AppSpec{Name: "api"},
		"fake-image",
		4242,
		&config.Config{},
		nil,
	)
	if err != nil {
		t.Fatalf("RunContainer returned error: %v", err)
	}
	if !fake.runCalled {
		t.Fatal("fake runtime adapter was not called")
	}
}

func TestBuildImagePrebuiltSkipsDocker(t *testing.T) {
	image, port, err := BuildImage(context.Background(), AppSpec{
		Name:     "api",
		Image:    "ghcr.io/example/api:latest",
		Prebuilt: true,
		Port:     8080,
	})
	if err != nil {
		t.Fatalf("BuildImage returned error: %v", err)
	}
	if image != "ghcr.io/example/api:latest" {
		t.Fatalf("image = %q", image)
	}
	if port != 8080 {
		t.Fatalf("port = %d", port)
	}
}

func TestDockerRunArgsComposeEnvOrderingAndRoute(t *testing.T) {
	cfg := &config.Config{Network: "pier", TLD: ".dock"}
	spec := AppSpec{
		Name:           "api",
		Dir:            "/workspace/api",
		EnvFile:        "/workspace/api/.pier/env",
		RuntimeEnvLast: true,
		Route:          true,
		Port:           3000,
		Env: map[string]string{
			"APP_ENV": "local",
		},
		ExtraEnv: []string{"FEATURE_FLAG=true"},
	}

	args := DockerRunArgs(spec, "api:latest", spec.Port, cfg, []string{
		"DATABASE_URL=postgres://pier",
		"DATABASE_NAME=api",
	})

	if got := flagValues(args, "--env-file"); !reflect.DeepEqual(got, []string{"/workspace/api/.pier/env"}) {
		t.Fatalf("env file values = %#v", got)
	}

	wantEnv := []string{
		"APP_ENV=local",
		"FEATURE_FLAG=true",
		"DATABASE_URL=postgres://pier",
		"DATABASE_NAME=api",
	}
	if got := flagValues(args, "-e"); !reflect.DeepEqual(got, wantEnv) {
		t.Fatalf("env values = %#v, want %#v", got, wantEnv)
	}

	assertContains(t, args, "traefik.enable=true")
	assertContains(t, args, "traefik.http.routers.api.rule=Host(`api.dock`)")
	assertContains(t, args, "traefik.http.services.api.loadbalancer.server.port=3000")
}

func TestDockerRunArgsEntrypointCommandAndRelativeVolume(t *testing.T) {
	cfg := &config.Config{Network: "pier", TLD: "test"}
	spec := AppSpec{
		Name:       "web",
		Dir:        "/workspace/web",
		Route:      true,
		Port:       8080,
		Volumes:    []string{"storage:/var/www/storage:ro"},
		Entrypoint: []interface{}{"npm", "run"},
		Command:    []interface{}{"dev", "--host", "0.0.0.0"},
	}

	args := DockerRunArgs(spec, "web:latest", spec.Port, cfg, []string{"REDIS_URL=redis://pier"})

	if got := flagValues(args, "-v"); !reflect.DeepEqual(got, []string{"/workspace/web/storage:/var/www/storage:ro"}) {
		t.Fatalf("volume values = %#v", got)
	}

	wantTail := []string{"--entrypoint", "npm", "web:latest", "run", "dev", "--host", "0.0.0.0"}
	if got := args[len(args)-len(wantTail):]; !reflect.DeepEqual(got, wantTail) {
		t.Fatalf("command tail = %#v, want %#v", got, wantTail)
	}
}

func TestLocalProcessAdapterBuildImageResolvesFrameworkPort(t *testing.T) {
	adapter := LocalProcessAdapter{}

	image, port, err := adapter.BuildImage(context.Background(), AppSpec{
		Name: "web",
		Dir:  t.TempDir(),
		Framework: &detect.Framework{
			Name:     "nextjs",
			Language: "node",
			Port:     3000,
		},
	})
	if err != nil {
		t.Fatalf("BuildImage returned error: %v", err)
	}
	if image != "web" {
		t.Fatalf("image = %q, want web", image)
	}
	if port != 3000 {
		t.Fatalf("port = %d, want 3000", port)
	}
}

func TestDevCommandForFramework(t *testing.T) {
	tests := []struct {
		name string
		fw   *detect.Framework
		port int
		want string
	}{
		{
			name: "nextjs",
			fw:   &detect.Framework{Name: "nextjs"},
			port: 3001,
			want: "npx next dev -p 3001",
		},
		{
			name: "laravel",
			fw:   &detect.Framework{Name: "laravel"},
			port: 8000,
			want: "php artisan serve --port=8000",
		},
		{
			name: "unknown",
			fw:   &detect.Framework{Name: "unknown"},
			port: 9000,
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := DevCommandForFramework(tt.fw, tt.port); got != tt.want {
				t.Fatalf("DevCommandForFramework() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestLocalProcessCommandPrefersPierfileCommand(t *testing.T) {
	dir := t.TempDir()
	writeOrchestratorTestFile(t, dir, "Pierfile", "name: api\nport: 4173\nservices:\n  - name: app\n    command: npm run dev -- --host 0.0.0.0\n")

	got := LocalProcessCommand(AppSpec{
		Name: "api",
		Dir:  dir,
		Framework: &detect.Framework{
			Name: "nextjs",
			Port: 3000,
		},
	}, 3000)

	if got != "npm run dev -- --host 0.0.0.0" {
		t.Fatalf("LocalProcessCommand() = %q", got)
	}
}

func TestLocalProcessRunAppCreatesProxyAndRegistryWithoutCommand(t *testing.T) {
	home := t.TempDir()
	dir := t.TempDir()
	t.Setenv("HOME", home)

	adapter := LocalProcessAdapter{}
	err := adapter.RunApp(
		context.Background(),
		AppSpec{Name: "api", Dir: dir},
		"",
		4242,
		&config.Config{TLD: "dock"},
		nil,
	)
	if err != nil {
		t.Fatalf("RunApp returned error: %v", err)
	}

	proxyFile := filepath.Join(home, ".pier", "traefik", "dynamic", "api.yaml")
	data, err := os.ReadFile(proxyFile)
	if err != nil {
		t.Fatalf("reading proxy file: %v", err)
	}
	if !strings.Contains(string(data), "host.docker.internal:4242") {
		t.Fatalf("proxy file missing host process target:\n%s", string(data))
	}

	projects, err := registry.Load()
	if err != nil {
		t.Fatalf("loading registry: %v", err)
	}
	if len(projects) != 1 {
		t.Fatalf("registry length = %d, want 1", len(projects))
	}
	if projects[0].Name != "api" || projects[0].Port != 4242 || projects[0].Type != "proxy" {
		t.Fatalf("registry project = %#v", projects[0])
	}
}

func TestLocalProcessEnvMergesRuntimeAndAppEnv(t *testing.T) {
	got := localProcessEnv(
		[]string{"PATH=/bin", "PORT=1000", "DATABASE_URL=old"},
		AppSpec{
			Env: map[string]string{
				"APP_ENV":      "local",
				"DATABASE_URL": "app",
			},
			ExtraEnv: []string{"FEATURE=true"},
		},
		[]string{"DATABASE_URL=runtime", "REDIS_URL=redis://pier"},
		5000,
	)

	want := []string{
		"PATH=/bin",
		"PORT=5000",
		"DATABASE_URL=app",
		"REDIS_URL=redis://pier",
		"APP_ENV=local",
		"FEATURE=true",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("localProcessEnv() = %#v, want %#v", got, want)
	}
}

func flagValues(args []string, flag string) []string {
	var values []string
	for i := 0; i < len(args)-1; i++ {
		if args[i] == flag {
			values = append(values, args[i+1])
			i++
		}
	}
	return values
}

func assertContains(t *testing.T, args []string, want string) {
	t.Helper()
	for _, arg := range args {
		if arg == want {
			return
		}
	}
	t.Fatalf("args missing %q: %#v", want, args)
}

func writeOrchestratorTestFile(t *testing.T, dir, name, content string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("creating test directory: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("writing test file: %v", err)
	}
}
