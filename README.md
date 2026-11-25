# Native Stack Runner (`stack`)

> Lightweight, containerless backend dependency runner for local development and CI.

[![CI](https://github.com/pandusatrianura/NativeStackRunner/actions/workflows/ci.yml/badge.svg)](https://github.com/pandusatrianura/NativeStackRunner/actions/workflows/ci.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/pandusatrianura/NativeStackRunner)](https://goreportcard.com/report/github.com/pandusatrianura/NativeStackRunner)
[![Release](https://img.shields.io/github/v/release/pandusatrianura/NativeStackRunner)](https://github.com/pandusatrianura/NativeStackRunner/releases)

---

## 🚀 Overview

**Native Stack Runner (`stack`)** is a single-binary CLI tool written in Go that runs local backend dependency binaries (**PostgreSQL**, **Redis / Dragonfly**, **MinIO**) directly in user space, isolated per-project under `./.stack/`.

- **No Docker Desktop required:** Say goodbye to gigabytes of VM RAM usage, sluggish WSL2 file syncing, and daemon licensing issues.
- **Fast Startup:** Native processes start in **< 150ms** compared to 3–10 seconds for container virtualization.
- **Project Isolation:** Data, logs, and PID locks stay scoped to `<project-dir>/.stack/`.
- **Zero Host Pollution:** Binaries are cached in `~/.stack/cache/bin/` with verified cryptographic SHA-256 checksums.

---

## 🛡️ Production & Hardening Features

- **Cryptographic SHA-256 Checksum Verification:** Validates every downloaded binary before extraction to prevent supply-chain tampering and corrupted partial downloads.
- **Pre-flight Port Prober:** Probes TCP port availability on `127.0.0.1` before starting any process, catching port collisions upfront with actionable error messages.
- **PostgreSQL Data Migration Guard:** Checks `.stack/data/postgres/PG_VERSION` before launch. If existing data belongs to PostgreSQL 15 and configuration requests 16, startup halts to prevent silent cluster corruption.
- **Linux Libc Environment Detector:** Intelligently detects `glibc` vs `musl` (e.g. Alpine Linux) and advises `gcompat` installation for glibc-dependent PostgreSQL binaries.
- **Interactive Terminal Progress Bar:** Dynamic rate-limited single-line progress indicator (`[service] [=========>      ] 45.0% (45.0 MB / 100.0 MB)`) that degrades gracefully in CI non-TTY environments.
- **Child Crash Watcher & Startup Rollback:** If any service crashes unexpectedly, the supervisor terminates sibling processes and removes stale PID files automatically.

---

## 📦 Supported Matrix

| Service | Engine / Distribution | Supported Platforms |
| :--- | :--- | :--- |
| **PostgreSQL** | Zonky Embedded PostgreSQL (16.1.0, 15.5.0, 14.10.0) | Windows (amd64), Linux (amd64, arm64), macOS (amd64, arm64) |
| **Redis** | Windows: Redis-x64 (5.0.14)<br>Linux/macOS: Dragonfly (1.14.0) | Windows (amd64), Linux (amd64, arm64), macOS (amd64, arm64) |
| **MinIO** | MinIO Static Server (`RELEASE.2024-01-18T22-51-28Z`) | Windows (amd64), Linux (amd64, arm64), macOS (amd64, arm64) |

---

## 🛠️ Installation

### Homebrew (macOS & Linux)
```bash
brew tap pandusatrianura/tap
brew install stack
```

### Scoop (Windows)
```powershell
scoop bucket add pandusatrianura https://github.com/pandusatrianura/scoop-bucket
scoop install stack
```

### Standalone Precompiled Binary
Download the precompiled archive for your OS from [GitHub Releases](https://github.com/pandusatrianura/NativeStackRunner/releases):
- **Windows:** `stack_windows_amd64.zip`
- **Linux:** `stack_linux_amd64.tar.gz` or `stack_linux_arm64.tar.gz`
- **macOS:** `stack_darwin_arm64.tar.gz` (Apple Silicon) or `stack_darwin_amd64.tar.gz` (Intel)

Extract and place `stack` (or `stack.exe`) in your system `$PATH`.

### Build from Source
```bash
git clone https://github.com/pandusatrianura/NativeStackRunner.git
cd NativeStackRunner
go build -o bin/stack ./cmd/stack
```

---

## ⚡ Quickstart

### 1. Declare Dependencies (`stack.yaml`)
Create `stack.yaml` in your project root:

```yaml
version: "1"
services:
  postgres:
    port: 5432
    version: "16"
    user: "postgres"
    database: "devdb"

  redis:
    port: 6379

  minio:
    port: 9000
    console_port: 9001
    access_key: "minioadmin"
    secret_key: "minioadmin"
```

*(Note: `stack` also seamlessly supports existing `docker-compose.yml` files!)*

### 2. Commands

```bash
# Initialize a new stack.yaml template in the current project
stack init

# Start all dependencies with live log multiplexing
stack up

# Print ready-to-use connection strings & environment variables
stack env
eval $(stack env --export)    # Load directly into shell session
stack env --json              # Output as JSON map

# Check status of running services
stack status

# Stop all running services
stack down

# Run ephemeral test suite with automatic dependency lifecycle and exit-code propagation
stack exec -- go test -v ./...
stack exec -- npm test
stack exec -c test-stack.yaml --quiet -- pytest

# Reset local data directory (.stack/data)
stack clean --force

# Check installed binary version and build metadata
stack version
stack --version
```

---

## ⚡ CI/CD Accelerator (GitHub Actions)

NativeStackRunner eliminates the heavy Docker daemon and container startup latency in continuous integration pipelines:

```yaml
name: Automated Integration Tests
on: [push, pull_request]

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.22'

      # Install NSR or build directly from source
      - name: Install stack CLI
        run: go install github.com/pandusatrianura/NativeStackRunner/cmd/stack@latest

      # Boots Postgres, Redis, MinIO, runs tests, and shuts down with test exit code
      - name: Run Test Suite
        run: stack exec --quiet -- go test -v ./...
```

---

## 🗺️ Roadmap

- **v1.0 GA (Current):** The Golden Trio (PostgreSQL 14–16, Redis 5/Dragonfly, MinIO S3) + `stack exec` CI/CD runner.
- **v1.1 (Next):** [Mailpit](https://github.com/axllent/mailpit) mock SMTP testing server and Web UI.
- **v1.2 (Planned):** Portable standalone MySQL / MariaDB engine.

---

## 🧪 Testing & CI

```bash
# Run all unit and integration tests
go test -v ./...

# Static analysis
go vet ./...

# Cross-compilation test
GOOS=linux GOARCH=amd64 go build ./cmd/stack
```

---

## 📄 License
MIT License.
