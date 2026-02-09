# Pier v0.2 — Specification

> Laravel Valet for everyone. Any framework. Zero infra pain.

---

## Philosophy

```
Pier = docker run + intelligence

- Don't replace Docker
- Don't touch dev's files
- Don't force team adoption
- Share when possible
- Stay invisible
```

---

## The Problem

**Your MacBook right now:**
```
postgres:5432  → pg14 (some old project?)
postgres:5433  → ntm
postgres:5434  → autopilot  
postgres:5435  → auth service
postgres:5436  → ???

redis:6379    → which cache?
redis:6380    → ntm
redis:6381    → autopilot

apps:
  3000 → ???
  3100 → ntm? or 3200?
  3200 → autopilot?
  3300 → that thing from yesterday
```

**The daily hell:**
- "Connection refused" → wrong port
- "Which port was ntm again?" → grep through .env files
- New project → port conflict → change to 5437
- 4 postgres containers eating 8GB RAM
- `docker ps` → wall of chaos
- New dev joins → 2 hours setting up local env
- "Works on my machine" → doesn't work on yours

**PHP devs have Valet. Everyone else suffers.**

---

## The Solution

```bash
cd my-project
pier up
# ✅ my-project.dock
```

**That's it.** No ports. No config. No suffering.

---

## Who Pier Is For

| Developer | Pain | Pier Solution |
|-----------|------|---------------|
| **Frontend dev** | "I just need the API running" | `pier up` → api.dock works |
| **Mobile dev** | "Backend setup takes forever" | `pier up` → backend.dock works |
| **Backend dev** | "I hate managing postgres/redis" | Pier shares them automatically |
| **PHP dev** | "I miss Valet for my Node projects" | Pier IS Valet for everything |
| **Junior dev** | "2 days to set up local env" | `git clone && pier up` |
| **Anyone** | "I hate Docker complexity" | Pier hides it all |

**If you don't like handling infra, Pier is for you.**

---

## Commands (v0.2)

```bash
pier run <image>    # docker run + pier-net + env override
pier up             # Detect from compose/Dockerfile + build + run
pier down           # Stop
pier ls             # List running projects
pier init           # Detect framework → Generate Dockerfile + .pier (if missing)
```

**5 commands. That's the entire CLI.**

---

## What Pier Does

### 1. Names, Not Ports

```
Before:                          After:
localhost:3100                   ntm.dock
localhost:3200                   autopilot.dock
localhost:5433                   pier-postgres-16
localhost:6380                   pier-redis-7
```

**Never remember a port again.** Type the name, it works.

### 2. Share Services Automatically

```
Project A: needs postgres:16     ─┐
Project B: needs postgres:16      ├→ ONE postgres (2GB RAM)
Project C: needs postgres:16     ─┘

Without Pier: 3 containers = 6GB RAM
With Pier:    1 container  = 2GB RAM
```

**Pier detects versions, shares when they match.**

### 3. Turn Any Project Into Docker

```bash
cd my-express-app        # No Dockerfile, no docker-compose
pier init
pier up
# ✅ my-express-app.dock
```

**Pier detects your framework, generates what's needed.**

Supported:
- Node.js (Express, NestJS, Fastify, Next.js, Nuxt)
- Python (Django, FastAPI, Flask)
- PHP (Laravel, Symfony)
- Ruby (Rails)
- Go (any framework)
- Rust (Actix, Axum)
- Elixir (Phoenix)
- Java (Spring Boot)

### 4. Don't Touch Your Files

```
Your files:           Pier touches?
─────────────────────────────────────
Dockerfile            Never (reads only)
docker-compose.yml    Never (reads only)
.env                  Never (overrides at runtime)
```

**Pier reads everything, changes nothing, overrides at launch.**

### 5. Zero Team Disruption

```gitignore
# .gitignore
.pier
```

```
Dev A (uses Pier):     pier up           → ntm.dock
Dev B (no Pier):       docker-compose up → localhost:3100
CI/CD:                 docker-compose up → works
Production:            normal deploy     → works
```

**Your choice. No forced adoption.**



---

## How It Works

### Simple Case: You Have Nothing

```bash
cd my-project          # Just code, no Docker stuff
pier init              # Detects framework, generates Dockerfile + .pier
pier up                # Builds, runs, routes
# ✅ my-project.dock
```

### Common Case: You Have docker-compose.yml

```yaml
# Your docker-compose.yml (UNTOUCHED)
services:
  postgres:
    image: postgres:16
    ports:
      - '5433:5432'     # Your weird port
  redis:
    image: redis:7
    ports:
      - '6380:6379'     # Another weird port
  app:
    build: .
    depends_on:
      - postgres
      - redis
```

```bash
pier up
```

```
🧠 Pier thinks:
   → postgres:16 needed
   → I have pier-postgres-16 running already
   → Skip this service, share mine
   
   → redis:7 needed  
   → I have pier-redis-7 running already
   → Skip this service, share mine
   
   → app needs building
   → Build it
   → Run on pier-net
   → Inject DB_HOST=pier-postgres-16
   → Inject REDIS_HOST=pier-redis-7

✅ my-project.dock
```

**Your compose file unchanged. Pier shares at runtime.**

### Advanced Case: Version Mismatch

```
Project A needs postgres:14
Project B needs postgres:16
```

```
Pier runs both:
  pier-postgres-14  ← Project A connects here
  pier-postgres-16  ← Project B connects here
```

**Different versions coexist. Pier routes correctly.**

---

## The .pier File

**Auto-generated. Editable. Optional.**

```yaml
# .pier
services:
  - postgres:16
  - redis:7
```

**Only needed when:**
- No docker-compose to detect from
- You want to specify versions explicitly
- Override auto-detection

**Most projects: Pier figures it out.**

---

## What `pier run` Does

```bash
# Dev types:
pier run myapp

# Pier executes:
docker run \
  --network pier-net \
  -e DB_HOST=pier-postgres-16 \
  -e DB_PORT=5432 \
  -e REDIS_HOST=pier-redis-7 \
  -e REDIS_PORT=6379 \
  myapp
```

Just added flags. That's all.



---

## What `pier up` Does

```
1. Check for docker-compose.yml
   → Found? Parse it.

2. Check for Dockerfile
   → Found? Use it.
   → Missing? Generate minimal one.

3. Check for .pier
   → Found? Use it.
   → Missing? Detect services from compose.

4. For each service in compose:
   → postgres/redis/mysql/mongo?
     → Version match with running? Share it.
     → No match? Start new instance.
   → App service?
     → Build and run on pier-net.

5. Inject environment at runtime:
   → DB_HOST=pier-postgres-16
   → REDIS_HOST=pier-redis-7

6. Route:
   → folder-name.dock (via Traefik)
```

---

## Service Sharing Logic

```
Project needs postgres:16

Pier checks: Do I have postgres running?
  → Yes, pier-postgres-16 exists    → Share it
  → Yes, but it's pier-postgres-14  → Start pier-postgres-16
  → No postgres running             → Start pier-postgres-16, share it
```

Multiple versions coexist:
```
pier-postgres-14   # For projects needing :14
pier-postgres-16   # For projects needing :16
pier-redis-7       # Shared by all redis users
```

---

## The Magic: Runtime Interception

**Your .env file (untouched):**
```env
DB_HOST=localhost
DB_PORT=5433
REDIS_HOST=localhost
REDIS_PORT=6380
```

**What Pier does at `pier up`:**
```bash
docker run \
  --network pier-net \
  -e DB_HOST=pier-postgres-16 \
  -e DB_PORT=5432 \
  -e REDIS_HOST=pier-redis-7 \
  -e REDIS_PORT=6379 \
  my-project
```

**Your files say localhost:5433. Container sees pier-postgres-16:5432.**

**No file changes. Runtime override.**

---

## File Ownership

| File | Owner | Pier touches? |
|------|-------|---------------|
| Dockerfile | Dev | Never (reads only, generates if missing) |
| docker-compose.yml | Dev | Never (reads only) |
| .env | Dev | Never (overrides at runtime) |
| .pier | Pier | Creates if needed, dev can edit |



---

## Framework Detection

Pier auto-detects and generates appropriate Dockerfiles:

| Detected | How | Dockerfile Template |
|----------|-----|---------------------|
| NestJS | `@nestjs/core` in package.json | Node + npm + build |
| Express | `express` in package.json | Node + npm |
| Next.js | `next` in package.json | Node + npm + next build |
| Laravel | `laravel/framework` in composer.json | PHP + composer + artisan |
| Rails | `rails` in Gemfile | Ruby + bundle |
| Django | `django` in requirements.txt | Python + pip + gunicorn |
| FastAPI | `fastapi` in requirements.txt | Python + pip + uvicorn |
| Go | go.mod exists | Go + build |
| Rust | Cargo.toml exists | Rust + cargo build |
| Phoenix | `phoenix` in mix.exs | Elixir + mix |
| Spring | pom.xml with spring-boot | Java + maven |

**Don't have a Dockerfile? Pier generates one. Have one? Pier uses yours.**

---

## Service Detection

From docker-compose.yml:
```yaml
services:
  db:
    image: postgres:16      # → Pier shares pier-postgres-16
  cache:
    image: redis:7          # → Pier shares pier-redis-7
  mongo:
    image: mongo:6          # → Pier shares pier-mongo-6
```

From dependencies:
```json
// package.json
"dependencies": {
  "pg": "^8.0.0",           // → Needs postgres
  "redis": "^4.0.0",        // → Needs redis
  "typeorm": "^0.3.0"       // → Confirms postgres
}
```

---

## Infrastructure

```
pier-net              # Docker network (all containers join)
pier-traefik          # Routes *.dock automatically
pier-postgres-16      # Shared, started on demand
pier-redis-7          # Shared, started on demand
```

**You don't manage this. Pier does.**

Data persistence:
```
~/.pier/data/
├── postgres-14/     # Survives restarts
├── postgres-16/     # Survives restarts
├── redis-7/         # Survives restarts
└── mongo-6/         # Survives restarts
```

---

## Real Numbers

| Metric | Before Pier | After Pier |
|--------|-------------|------------|
| Postgres instances | 4 | 1 |
| Redis instances | 3 | 1 |
| RAM for databases | 8GB | 2GB |
| Ports to remember | 12+ | 0 |
| Time to onboard new dev | 2 hours | 2 minutes |
| "Works on my machine" bugs | Weekly | Never |



---

## Comparison

| Feature | Docker Compose | Laravel Valet | Pier |
|---------|----------------|---------------|------|
| Any framework | ✅ | ❌ PHP only | ✅ |
| No port management | ❌ | ✅ | ✅ |
| Service sharing | ❌ | ✅ | ✅ |
| Auto-detection | ❌ | ✅ | ✅ |
| Name-based routing | ❌ | ✅ | ✅ |
| Containerized | ✅ | ❌ | ✅ |
| Works with existing Docker | ✅ | ❌ | ✅ |
| Zero config | ❌ | ✅ | ✅ |

**Pier = Valet's simplicity + Docker's power**

---

## Workflows

### Frontend Dev
```bash
git clone company/api
cd api
pier up
# ✅ api.dock — call it from your React app
```

### Mobile Dev
```bash
git clone company/backend
cd backend
pier up
# ✅ backend.dock — point your app here
```

### New Team Member
```bash
# Day 1 at new job
git clone company/monorepo
cd monorepo/api
pier up
# ✅ Working in 2 minutes, not 2 hours
```

### Multi-Project Work
```bash
# Terminal 1
cd api && pier up        # ✅ api.dock

# Terminal 2
cd admin && pier up      # ✅ admin.dock

# Terminal 3  
cd worker && pier up     # ✅ worker.dock

# All share same postgres, same redis
```

### Convert Legacy Project
```bash
cd old-express-app       # No Docker, runs on localhost:3000
pier init                # Generates Dockerfile + .pier
pier up                  # ✅ old-express-app.dock

# Now it's containerized
git add Dockerfile .pier
git commit -m "Add Pier support"
```

---

## Migration from v0.1

v0.1 commands still work:
```bash
pier proxy myapp 3000    # Still works (bare-metal routing)
pier unproxy myapp       # Still works
```

New v0.2 commands:
```bash
pier run <image>         # docker run + smart flags
pier up                  # Smart compose/Dockerfile handling
pier init                # Framework detection + generation
```

---

## Summary

```
5 commands:     pier init, pier run, pier up, pier down, pier ls
1 file:         .pier (optional, auto-generated)
1 network:      pier-net (automatic)
1 router:       pier-traefik (automatic)

Share services. Route by name. Stay invisible.
```

---

## The Pitch

> **Pier is Laravel Valet for everyone.**
>
> Frontend dev? `pier up` — your API works.
> Mobile dev? `pier up` — your backend works.
> Backend dev? `pier up` — no more port hell.
> New to the team? `pier up` — you're running in 2 minutes.
>
> Pier detects your framework, shares services, routes by name.
> Your files stay untouched. Your team doesn't need to adopt it.
>
> **Stop managing ports. Start building.**
