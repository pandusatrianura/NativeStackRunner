# Native Stack Runner (`nsr` / `stack`)

## 1. Problem Statement
**How Might We** deliver an instant (< 200ms warm-start), battery-efficient, and per-project isolated backend dependency runner (PostgreSQL, Redis, MinIO) for developers without the RAM bloat, thermal issues, and enterprise licensing hurdles of Docker Desktop?

---

## 2. Recommended Direction
We are building **Native Stack Runner** (`stack` / `nsr`) as a single-binary CLI written in **Go**. The tool serves as a lightweight, zero-daemon dependency supervisor. Rather than managing virtual machines or container overlays, it dynamically downloads precompiled, platform-specific static binaries and runtime bundles (e.g., Zonky/embedded-postgres for PostgreSQL, Redis/Dragonfly, MinIO) into a shared user-level cache (`~/.stack/cache/`).

When a developer runs `stack up` in a project directory:
1. It resolves services defined in `stack.yaml` (with seamless auto-fallback to standard `docker-compose.yml`).
2. It provisions an isolated per-project state inside `./.stack/data/{service}` and writes process identifiers to `./.stack/pids/{service}.pid`.
3. It launches services as standard local child processes bound to project-scoped ports or Unix domain sockets, injecting required runtime paths (`LD_LIBRARY_PATH` / `DYLD_LIBRARY_PATH`).
4. It multiplexes and color-codes log output in the foreground and executes a graceful shutdown (`SIGINT`/`SIGTERM`) sequence upon exit.

---

## 3. Key Assumptions to Validate

### Must Be True (Dealbreakers)
- [ ] **Binary Portability across Host OSes:** Precompiled PostgreSQL bundles with bundled shared libraries (`libssl`, `libcrypto`, `libicudata`) can run cleanly across major Linux distributions (Ubuntu, Fedora, Arch) and macOS (Apple Silicon & Intel) without runtime link failures.  
  *Validation Strategy:* Build a smoke-test CLI spike that downloads and launches PostgreSQL 16 on macOS arm64, Ubuntu 22.04 x86_64, and Arch Linux.
- [ ] **Clean Process Lifecycle & Orphan Elimination:** Abrupt developer terminal exits or `kill -9` will leave PID artifacts that the CLI can reliably detect and reap on subsequent runs without port locking.  
  *Validation Strategy:* Stress-test process termination scenarios (SIGINT, SIGTERM, crash recovery) verifying PID files and port release.

### Should Be True (Important)
- [ ] **Docker Compose Compatibility:** A lightweight YAML parser can interpret typical `postgres` and `redis` service blocks in existing `docker-compose.yml` files (ports, environment variables, volumes) without requiring full specification compliance.  
  *Validation Strategy:* Test parsing on 10 popular open-source project `docker-compose.yml` files.
- [ ] **Resource & Battery Savings:** Native process execution consumes < 20% of the idle RAM and zero background VM CPU overhead compared to Docker Desktop running the same services.  
  *Validation Strategy:* Benchmark idle RAM/CPU footprint of Docker Desktop (Postgres + Redis) vs `stack up`.

### Might Be True (Nice to Have)
- [ ] Developers prefer using Unix domain sockets over localhost TCP ports to completely avoid port collision across concurrent projects.

---

## 4. MVP Scope (v0.1)

### What's In
- **Target OS:** macOS (Apple Silicon arm64, Intel x86_64) and Linux (x86_64).
- **Core Services:**
  - **PostgreSQL** (v15, v16 via embedded-postgres curated binary archive)
  - **Redis** (v7 / Dragonfly static binary)
  - **MinIO** (S3 compatible local object store)
- **CLI Commands:**
  - `stack up`: Download missing binaries, initialize isolated data dirs, start services, stream prefixed logs.
  - `stack down`: Graceful stop of running project services.
  - `stack status`: Check health, ports, and PIDs of project services.
  - `stack clean`: Remove local `./.stack/data` directory for a fresh state.
- **Config Support:**
  - Native `stack.yaml`.
  - Fallback parsing of existing `docker-compose.yml` for postgres/redis services.

### What's Out (Not Doing & Why)
- ❌ **Custom Dockerfile Image Building:** This is not a container engine; it does not build arbitrary container images.
- ❌ **Container Overlay / Virtual Bridge Networking:** All services bind to `localhost` or project-scoped ports/sockets; no virtual network bridges or iptables manipulation.
- ❌ **Support for 10+ Databases on Day One:** Strict focus on PostgreSQL, Redis, and MinIO first to guarantee 100% stability.
- ❌ **GUI / Desktop Tray App:** Keeping it strictly a developer-first CLI for terminal velocity and scriptability.
- ❌ **Background Persistent Daemon:** Avoiding the anti-pattern of running a global background daemon that drains system resources.

---

## 5. Configuration Specification

### Native `stack.yaml`
```yaml
version: "1"
name: myapp-stack

services:
  postgres:
    version: "16"
    port: 5432
    database: app_dev
    user: postgres
    password: password

  redis:
    version: "7"
    port: 6379

  minio:
    version: "latest"
    port: 9000
    console_port: 9001
    access_key: minioadmin
    secret_key: minioadmin
    buckets:
      - uploads
```

---

## 6. Architecture & Directory Layout

```
~/.stack/                         # Global User Cache
  └── cache/
      ├── bin/
      │   ├── postgres/16.1-arm64/...
      │   ├── redis/7.2-arm64/...
      │   └── minio/latest-arm64/...
      └── manifests.json          # Verified release signatures & checksums

<project-root>/
  ├── stack.yaml                  # Project dependency spec (or docker-compose.yml)
  └── .stack/                     # Per-project isolated runtime (gitignored)
      ├── data/
      │   ├── postgres/           # PGDATA directory
      │   └── minio/              # S3 data buckets
      ├── pids/                   # Service PID tracking files
      └── logs/                   # Service rotating log files
```

---

## 7. Open Questions for Phase 1 Implementation
1. **Windows Support Priority:** Should Windows support be bundled in v0.1 via official Windows PostgreSQL/Redis port binaries, or focused on macOS/Linux first?
2. **Binary CDN Hosting:** Should assets be fetched directly from official GitHub releases (e.g., Zonky `embedded-postgres-binaries` and MinIO releases) or mirrored through a dedicated project GitHub Releases repository with SHA256 verification?
3. **Naming & Identity:** Project naming choice between `stack` (concise, familiar) vs `nsr` (distinctive binary name to prevent PATH collisions).
