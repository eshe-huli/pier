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

## Next Low-Level Closures

1. Extend process runtime lifecycle coverage into `pier down`, `pier logs`, and
   dashboard restart flows so process-mode projects are as operable as Docker
   projects after launch.
2. Decide how far process mode should go for docker-compose projects: keep app
   services on Docker by default, or map explicitly declared commands to host
   processes while reusing compose infra.

## Verification Gates

- `go test ./...`
- `make build`
- `git diff --check`
