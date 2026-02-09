# Changelog

All notable changes to Pier will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.2.0] — 2026-02-09

### Added
- `pier up` — Smart project launcher with compose parsing and service sharing
- `pier run` — Docker run with automatic pier-net joining and env injection
- `pier down [name]` — Stop specific projects, `--all` stops everything
- `pier init` — Now detects framework and generates .pier + Dockerfile
- Shared infrastructure: postgres, redis, mongo, mysql, minio (version-aware, auto-shared)
- `.pier` config file (auto-generated, optional)
- Framework detection: NestJS, Express, Next.js, Django, FastAPI, Flask, Go, Rust, Phoenix, Laravel, Rails, Spring Boot, Nuxt, Fastify
- Dockerfile generation for all supported frameworks
- Service detection from docker-compose.yml and dependency files
- Runtime env override (DB_HOST, REDIS_HOST injected automatically)
- Auto database creation for postgres/mysql projects

### Changed
- `pier ls` now shows projects, proxies, and infra services
- `pier down` extended from Traefik-only to per-project + --all
- `pier init` now smart: system init or project init based on context

## [0.1.0] - 2026-02-07

### Added
- `pier init` — one-time setup (Docker network, Traefik, dnsmasq, nginx, resolver)
- `pier ls` — list all active services with domains
- `pier proxy <n> <port>` — route `<n>.dock` → `localhost:<port>`
- `pier unproxy <n>` — remove a bare-metal proxy route
- `pier status` — system health check
- `pier doctor` — diagnose issues with suggested fixes
- `pier dashboard` — open Traefik dashboard in browser
- `pier down` / `pier restart` — manage Pier infrastructure
- `pier config` — view and modify configuration
- `pier version` — print version number
- Configurable TLD (`.dock` default, `.loc`, `.lab`, `.box`, etc.)
- Automatic Docker container discovery via `pier` network
- Laravel Valet compatibility (coexists on different TLD)
- Embedded web dashboard at `pier.dock`

[0.2.0]: https://github.com/eshe-huli/pier/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/eshe-huli/pier/releases/tag/v0.1.0
