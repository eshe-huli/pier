# Changelog

All notable changes to Pier will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.1.0] - 2026-02-07

### Added
- `pier init` — one-time setup (Docker network, Traefik, dnsmasq, nginx, resolver)
- `pier ls` — list all active services with domains
- `pier proxy <name> <port>` — route `<name>.dock` → `localhost:<port>`
- `pier unproxy <name>` — remove a bare-metal proxy route
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

[0.1.0]: https://github.com/eshe-huli/pier/releases/tag/v0.1.0
