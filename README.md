<p align="center">
  <img src="https://em-content.zobj.net/source/apple/391/anchor_2693.png" width="80" alt="Pier">
</p>

<h1 align="center">Pier</h1>

<p align="center">
  <strong>Pier gives every Docker container and local process a clean <code>.dock</code> domain — no more port numbers.</strong>
</p>

<p align="center">
  <a href="#quick-start">Quick Start</a> •
  <a href="#usage">Usage</a> •
  <a href="#how-it-works">How It Works</a> •
  <a href="#configuration">Configuration</a> •
  <a href="#roadmap">Roadmap</a> •
  <a href="#contributing">Contributing</a>
</p>

<p align="center">
  <a href="https://github.com/eshe-huli/pier/releases"><img src="https://img.shields.io/github/v/release/eshe-huli/pier?style=flat-square" alt="Release"></a>
  <a href="LICENSE"><img src="https://img.shields.io/badge/license-MIT-blue?style=flat-square" alt="License"></a>
  <a href="https://github.com/eshe-huli/pier/actions"><img src="https://img.shields.io/github/actions/workflow/status/eshe-huli/pier/ci.yml?style=flat-square&label=build" alt="Build"></a>
</p>

---

What [Laravel Valet](https://laravel.com/docs/valet) did for PHP, Pier does for **everything else**. Spin up a Docker container or run a local process — it gets a clean `.dock` domain instantly. No port numbers, no `/etc/hosts` editing, no YAML gymnastics.

```bash
pier init                    # one-time setup
pier proxy myapp 3000        # http://myapp.dock → localhost:3000
docker compose up            # http://app.dock → container (automatic)
```

## Quick Start

```bash
# Install via Homebrew
brew install eshe-huli/tap/pier

# Or download the binary
curl -fsSL https://github.com/eshe-huli/pier/releases/latest/download/pier-darwin-arm64 -o /usr/local/bin/pier
chmod +x /usr/local/bin/pier

# One-time setup (creates Docker network, configures dnsmasq + nginx + Traefik)
pier init

# Done. Every container on the 'pier' network gets a domain automatically.
```

### Prerequisites

- **macOS** (Linux support planned)
- **Docker** — Docker Desktop or [OrbStack](https://orbstack.dev)
- **Homebrew** — for dnsmasq and nginx dependencies

`pier init` handles the rest.

## Usage

```bash
pier ls                      # List all active services
pier proxy myapp 3000        # Route myapp.dock → localhost:3000
pier unproxy myapp           # Remove the route
pier status                  # System health check
pier doctor                  # Diagnose issues with fix suggestions
pier dashboard               # Open Traefik dashboard in browser
pier down                    # Stop Pier infrastructure
pier restart                 # Restart everything
pier config                  # View configuration
pier config set tld loc      # Change TLD from .dock to .loc
```

### Docker Containers

Add your service to the `pier` network — that's it:

```yaml
# docker-compose.yml
services:
  app:
    build: .
    networks: [default, pier]

networks:
  pier:
    external: true
```

```bash
docker compose up
# → http://app.dock ✅
```

Override the domain with a label:

```yaml
labels:
  - "pier.domain=myapp"      # → http://myapp.dock
```

### Bare-Metal Processes

```bash
npm run dev                   # Running on port 3000
pier proxy myapp 3000         # → http://myapp.dock

go run main.go                # Running on port 8080
pier proxy api 8080           # → http://api.dock
```

## How It Works

```
  Browser → myapp.dock
      │
      ▼
┌──────────────┐
│   dnsmasq    │  *.dock → 127.0.0.1
└──────┬───────┘
       ▼
┌──────────────┐
│    nginx     │  *.dock:80 → proxy_pass :8880
└──────┬───────┘
       ▼
┌──────────────┐
│   Traefik    │  Routes by container name (Docker provider)
│   :8880      │  Routes by config (file provider for bare-metal)
└──────┬───────┘
       ▼
┌──────────────┐
│  Container   │  On 'pier' Docker network — no port mapping needed
│  or Process  │  Or localhost process on any port
└──────────────┘
```

**dnsmasq** resolves all `*.dock` domains to `127.0.0.1`. **nginx** catches port 80 traffic and forwards it to **Traefik**, which auto-discovers Docker containers on the `pier` network and reads file-based configs for bare-metal proxies. Zero configuration per service.

## Comparison

| | **Pier** | **Laravel Valet** | **Docker Compose ports** | **/etc/hosts** |
|---|---|---|---|---|
| Docker containers | ✅ Automatic | ❌ PHP only | ⚠️ Manual port mapping | ⚠️ Manual per-host |
| Local processes | ✅ `pier proxy` | ✅ `valet proxy` | ❌ | ⚠️ Manual |
| Setup | One command | One command | Per-project YAML | Manual editing |
| Conflicts | None (unique names) | None (.test TLD) | Port collisions | Stale entries |
| Multiple services | All get domains | PHP apps only | Port juggling | Works but painful |
| Configurable TLD | ✅ `.dock`, `.loc`, `.lab`… | `.test` only | N/A | N/A |
| Cleanup | `pier unproxy` / stop container | `valet unlink` | Remove port mapping | Edit file again |

**tl;dr** — Pier is Valet for containers. If you're tired of `localhost:3000`, `localhost:3001`, `localhost:8080`... this is for you.

## Configuration

Config lives at `~/.pier/config.yaml`:

```yaml
tld: dock                     # Default TLD (.dock, .loc, .lab, .box)
traefik:
  port: 8880                  # Internal Traefik port
  dashboard: true
  image: traefik:v3.3
network: pier                 # Docker network name
nginx:
  managed: true
  valet_compatible: true      # Coexists with Laravel Valet
```

`nginx.managed: true` is the default because it keeps Pier Valet-compatible:
nginx owns host port 80 and forwards `*.dock` to Traefik on `traefik.port`.
Set `nginx.managed: false` and run `pier restart` if you want Traefik to bind
host port 80 directly instead.

When upgrading older Pier installs, `pier init` and `pier restart` replace a
legacy standalone `pier-traefik` container with the compose-managed Pier stack.

### Valet Compatibility

Pier coexists with Laravel Valet out of the box:
- `.test` → Valet (PHP apps)
- `.dock` → Pier (containers & everything else)

Different TLDs, different nginx server blocks. No conflicts.

## Commands

| Command | Description |
|---|---|
| `pier init` | One-time setup (Docker network, Traefik, dnsmasq, nginx) |
| `pier ls` | List all active services with their domains |
| `pier proxy <name> <port>` | Route `<name>.dock` → `localhost:<port>` |
| `pier unproxy <name>` | Remove a bare-metal proxy route |
| `pier status` | System health check |
| `pier doctor` | Diagnose issues with suggested fixes |
| `pier dashboard` | Open Traefik dashboard in browser |
| `pier down` | Stop Pier infrastructure |
| `pier restart` | Restart Pier infrastructure |
| `pier config` | View current configuration |
| `pier config get <key>` | Get a config value |
| `pier config set <key> <val>` | Set a config value |
| `pier version` | Print version |

## Roadmap

Pier is open core — the CLI is free and open source. Pro features are coming:

### 🔓 Pier Pro (planned)
- **`pier secure`** — Automatic HTTPS with locally-trusted certificates (mkcert)
- **Auto-discovery** — Detect running processes and suggest proxies
- **Blueprints** — Project templates (`pier blueprint rails` → Redis + Postgres + app)
- **Team sync** — Share Pier configs across a team

### ☁️ Pier Cloud (planned)
- **`pier tunnel`** — Expose local services to the internet (like ngrok, built-in)
- **Persistent URLs** — Stable tunnel URLs for webhooks and demos
- **Access control** — Password-protect tunnels

## FAQ

<details>
<summary><strong>Does it work with OrbStack?</strong></summary>
Yes. Pier uses the standard Docker API — any Docker-compatible runtime works.
</details>

<details>
<summary><strong>Does it conflict with Laravel Valet?</strong></summary>
No. Pier uses <code>.dock</code> by default, Valet uses <code>.test</code>. They share nginx peacefully.
</details>

<details>
<summary><strong>Can I use a different TLD?</strong></summary>
Yes: <code>pier config set tld loc</code> then <code>pier restart</code>.
</details>

<details>
<summary><strong>What about HTTPS?</strong></summary>
Coming in Pier Pro with <code>pier secure</code> — auto-trusted local certs via mkcert.
</details>

<details>
<summary><strong>Does it need Kubernetes?</strong></summary>
No. Pure Docker networking. Lightweight and fast.
</details>

## Contributing

Contributions welcome! Pier is built with Go 1.25+ and cobra.

```bash
git clone https://github.com/eshe-huli/pier.git
cd pier
make build                    # builds to bin/pier
make test                     # run tests
```

### Project Structure

```
cmd/pier/          CLI entrypoint
internal/
  cli/             Command definitions (cobra)
  config/          Configuration management
  dns/             dnsmasq + resolver setup
  docker/          Docker network + container discovery
  proxy/           Traefik + nginx + file provider
  dashboard/       Embedded web dashboard
```

Please open an issue before submitting large PRs. Bug fixes and documentation improvements are always welcome.

## License

[MIT](LICENSE) — do whatever you want.

---

<p align="center">
  <strong>Port hell is over.</strong> 🔩
  <br>
  Built by <a href="https://github.com/eshe-huli">eshe-huli</a>
</p>
