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

## Next Low-Level Closures

1. Add a `.pier` manifest writer/reader for detected services, ports, and
   overrides so teams can opt in without forcing adoption.
2. Add integration tests using fixture projects under an ignored examples
   directory to prove Next, Nest, Astro, Remix, Rails, Go, Django/FastAPI, and
   Laravel all route to `*.dock`.
3. Decide whether Nginx remains required or whether Traefik can bind directly
   to port 80 with a clearer sudo/manual step.
4. Add upgrade handling for existing non-compose Pier infrastructure so users
   can move from v0.1 to v0.2 without orphaned containers.

## Verification Gates

- `go test ./...`
- `make build`
- `git diff --check`
