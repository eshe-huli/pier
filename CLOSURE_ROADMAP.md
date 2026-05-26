# Pier Closure Roadmap

## Product Decision

Pier should feel like Laravel Valet for any language: a developer types
`pier up` and gets a stable local domain without memorizing ports or editing
project files. Docker is the runtime adapter, not the product surface.

## Closed In This Pass

- Pier infrastructure is grouped under a generated `~/.pier/docker-compose.yml`
  so Docker Desktop/OrbStack shows one coherent Pier stack.
- Traefik startup now uses Docker Compose while preserving Docker API fallback
  shutdown for older Pier-created containers.
- `pier init` recognizes Astro and Remix alongside the existing Node, PHP,
  Ruby, Go, Java, Python, Rust, and Elixir paths.
- Local process links keep their log file open until the child process exits,
  preventing detached dev servers from losing stdout/stderr.
- dnsmasq PID checks now verify that the process actually exists on macOS
  instead of trusting a stale PID file.
- Tests cover Astro/Remix detection and the generated compose file contract.
- `pier up` now starts from a tested internal planner that normalizes compose
  app/infra splits, Pierfile project names, default service versions, and
  dependency-file service detection before Docker commands run.
- `pier up` app execution now consumes planner-rendered app plans for compose
  build contexts, Dockerfile paths, route names, ports, env, and bind mounts.
- `.pier/manifest.yaml` now records project-local plan details and can feed
  planner service, port, and env overrides without changing committed files.
- Planner fixture tests now cover Next, Nest, Astro, Remix, Rails, Go, Django,
  FastAPI, and Laravel app domains, ports, languages, and shared service specs.
- HTTP edge behavior is explicit: nginx remains the default Valet-compatible
  port 80 owner, while `nginx.managed=false` lets Traefik bind port 80 directly.
- Startup now detects legacy standalone `pier-traefik` containers and replaces
  them with the compose-managed Pier stack during init/restart.
- `pier up` and `pier run` now share the internal orchestrator for Docker image
  builds, container runs, env injection, route labels, volumes, entrypoints, and
  registry updates.
- `pier up --dry-run` and `pier run --dry-run` now provide command-level smoke
  paths that verify detected apps, shared services, domains, ports, env, and
  images without touching Docker, proxy files, registry state, manifests, or
  `.gitignore`.
- `pier up` and `pier run` now execute through an explicit `RuntimeAdapter`
  boundary, with Docker registered as the default adapter and tests proving the
  CLI-facing orchestrator can swap adapters without changing the plan shape.
- `pier up --runtime process` now runs the first host-process runtime adapter:
  it resolves the same planner app shape, starts a framework/Pierfile command
  when known, creates a file-provider `.dock` route, writes link metadata, and
  registers the project without app Docker.
- Process-mode lifecycle now shares PID/log/proxy metadata across CLI and
  dashboard surfaces: `pier down` stops host processes and removes routes,
  `pier down --all` includes registered process-mode projects, `pier logs`
  tails `.pier/dev.log`, and the dashboard can start, stop, and restart
  command-backed local processes through the same runtime path.
- `pier up --runtime process` now supports docker-compose projects whose app
  service declares an explicit `command`, reusing Pier shared infra while
  running the app itself as a host process.
- Compose process commands now execute from the service build context, so
  monorepo services such as `build: ./api` run their dev command in the
  expected working directory.
- Compose parsing now accepts long-form and numeric `ports`/`expose` entries
  before normalizing them back into Pier's existing port-selection path.
- `pier up` now auto-selects the process runtime for detected non-compose app
  projects with known framework/Pierfile commands, while `--runtime docker`
  remains available as an explicit escape hatch.
- Compose process mode can now infer a safe host command from a service build
  context with a known framework, while still refusing commandless services that
  do not have a detectable framework or Pierfile command.

## Next Low-Level Closures

1. Expand framework-specific process-mode command inference with driver tests
   for Rails, Django/FastAPI, Laravel, Go, Rust, Phoenix, and Spring Boot
   compose build contexts.
2. Add a `pier doctor process` check that explains missing host dependencies
   for inferred commands before users hit runtime shell errors.

## Verification Gates

- `go test ./...`
- `make build`
- `git diff --check`
