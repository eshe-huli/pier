# Changelog

All notable changes to Pier will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased] - v0.2.0

### Vision
Pier v0.2 evolves from a routing tool to a **full dev environment helper** — Laravel Valet for everyone.

### Planned
- `pier up` — Smart build + run from compose/Dockerfile with service sharing
- `pier run <image>` — docker run + pier-net + automatic env override
- `pier init` (enhanced) — Framework detection, Dockerfile generation if missing
- **Service sharing** — Detect postgres/redis versions, share when match, run new when mismatch
- **Runtime interception** — Override DB_HOST, REDIS_HOST at launch without touching files
- **Framework detection** — Auto-detect NestJS, Express, Laravel, Rails, Django, FastAPI, Go, Rust, Phoenix, Spring
- **Version-aware infra** — pier-postgres-14, pier-postgres-16 can coexist
- **Zero file changes** — Pier reads compose/Dockerfile/.env, never writes to them
- **Optional .pier file** — Auto-generated, editable, gitignore-able

### Philosophy
- Don't replace Docker — intercept it
- Don't touch dev's files — override at runtime  
- Don't force team adoption — .pier in .gitignore
- Share when possible — version match = share
- Stay invisible — same codebase works without Pier

See [docs/SPEC-v0.2.md](docs/SPEC-v0.2.md) for full specification.

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

[Unreleased]: https://github.com/eshe-huli/pier/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/eshe-huli/pier/releases/tag/v0.1.0
