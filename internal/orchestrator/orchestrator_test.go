package orchestrator

import (
	"context"
	"reflect"
	"testing"

	"github.com/eshe-huli/pier/internal/config"
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
