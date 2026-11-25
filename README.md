# NativeStackRunner (`stack`)

NativeStackRunner is an open-source command-line tool built with Go that provisions and executes backend services—specifically PostgreSQL, Redis, and MinIO—as native processes on your machine. It gives you the convenience of service management without the memory overhead, virtualization layers, or container daemons required by Docker.

[![CI](https://github.com/pandusatrianura/NativeStackRunner/actions/workflows/ci.yml/badge.svg)](https://github.com/pandusatrianura/NativeStackRunner/actions/workflows/ci.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/pandusatrianura/NativeStackRunner)](https://goreportcard.com/report/github.com/pandusatrianura/NativeStackRunner)
[![Release](https://img.shields.io/github/v/release/pandusatrianura/NativeStackRunner)](https://github.com/pandusatrianura/NativeStackRunner/releases)

---

## Why NativeStackRunner?

Traditional local development often relies on container runtimes to spin up databases and caches. While containers offer isolation, they also introduce notable drawbacks on developer machines:
- High idle RAM usage (often 2–4 GB for VM engines).
- Slow boot times (several seconds per container).
- Slower disk I/O when synchronizing files across virtualization boundaries.

NativeStackRunner solves this by downloading official standalone service binaries and managing them directly in user-space. Each project keeps its own isolated data and configuration inside a local `.stack` folder, booting in milliseconds on native host ports.

---

## Key Capabilities

- **Sub-Second Process Startup:** PostgreSQL, Redis, and MinIO boot in less than 500 milliseconds.
- **Project-Level Isolation:** Cluster data, log files, and process identifiers are stored locally in `.stack/` inside your project directory.
- **Automatic Resource Provisioning:** Automatically initializes database clusters (`initdb`), creates custom databases, and creates pre-configured S3 buckets.
- **Port Conflict Detection:** Pre-flight TCP verification prevents services from launching if a target port is already occupied.
- **Integrity Validation:** Downloaded binaries are verified using cryptographic SHA-256 checksums before extraction.
- **Ephemeral CI/CD Testing (`stack exec`):** Boots all declared dependencies, waits for health checks, executes your test command with auto-injected environment variables, and tears down cleanly with the test exit code.

---

## Supported Services

| Service | Supported Versions | Supported Operating Systems |
| :--- | :--- | :--- |
| **PostgreSQL** | 16, 15, 14 | Windows (amd64), macOS (Apple Silicon & Intel), Linux (amd64 & arm64) |
| **Redis** | 5.x / Dragonfly | Windows (amd64), macOS (Apple Silicon & Intel), Linux (amd64 & arm64) |
| **MinIO (S3)** | Latest stable | Windows (amd64), macOS (Apple Silicon & Intel), Linux (amd64 & arm64) |

---

## Installation

### Method 1: Homebrew (macOS & Linux)

```bash
brew tap pandusatrianura/homebrew-tap
brew install stack
```

### Method 2: Scoop (Windows)

```powershell
scoop bucket add pandusatrianura https://github.com/pandusatrianura/scoop-bucket
scoop install stack
```

### Method 3: Go Install

```bash
go install github.com/pandusatrianura/NativeStackRunner/cmd/stack@latest
```

### Method 4: Precompiled Binaries

Download the appropriate archive for your operating system from the [Releases](https://github.com/pandusatrianura/NativeStackRunner/releases) page, extract the executable, and add it to your system PATH.

### Method 5: Build From Source

```bash
git clone https://github.com/pandusatrianura/NativeStackRunner.git
cd NativeStackRunner
go build -o bin/stack ./cmd/stack
```

---

## How to Run NativeStackRunner

### Step 1: Initialize Configuration

Navigate to your application folder and create a `stack.yaml` file by running:

```bash
stack init
```

This generates a template configuration in the current directory. You can customize the ports, versions, database name, and S3 buckets:

```yaml
version: "1"
name: my-app-stack

services:
  postgres:
    version: "16"
    port: 5432
    database: myapp_dev
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
      - avatars
```

*(Note: NativeStackRunner also supports reading standard `docker-compose.yml` files out of the box).*

---

### Step 2: Start the Stack (`stack up`)

Start all declared services in your terminal:

```bash
stack up
```

What happens under the hood:
1. Verifies that all required host ports (e.g. 5432, 6379, 9000) are free.
2. Checks the local cache (`~/.cache/stack/bin`) for precompiled service binaries. Missing binaries are fetched and verified via SHA-256 automatically.
3. Initializes the database cluster under `.stack/data/postgres` if it does not yet exist.
4. Spawns each service and monitors logs in real time with distinct terminal color tags.
5. Auto-creates custom databases (`myapp_dev`) and MinIO buckets (`uploads`, `avatars`).

To stop services, press `Ctrl+C` in the running terminal, or run `stack down` from another window.

---

### Step 3: Inspect Service Health & Environment Variables

Open another terminal in your project directory:

```bash
# View active service statuses and PIDs
stack status

# Print connection environment variables
stack env

# Export variables directly into your current shell
eval $(stack env --export)

# Or export in JSON format for scripting
stack env --json
```

Default environment variables generated by `stack env`:
- `DATABASE_URL=postgres://postgres:password@localhost:5432/myapp_dev?sslmode=disable`
- `REDIS_URL=redis://localhost:6379`
- `S3_ENDPOINT=http://localhost:9000`
- `AWS_ACCESS_KEY_ID=minioadmin`
- `AWS_SECRET_ACCESS_KEY=minioadmin`

---

### Step 4: Accelerated Testing in CI/CD (`stack exec`)

When running automated test suites, you usually only need dependencies while tests execute. Use `stack exec` to handle the full lifecycle automatically:

```bash
# Run unit & integration tests
stack exec -- go test -v ./...

# Or for Node.js / Python projects
stack exec -- npm test
stack exec -- pytest

# Quiet mode for CI logs
stack exec --quiet -- npm test
```

`stack exec` operates in five phases:
1. Launches dependencies in the background.
2. Waits until all service TCP ports accept connections and auto-creates databases/buckets.
3. Injects connection environment variables (`DATABASE_URL`, `REDIS_URL`, etc.) into the child process.
4. Executes your command and captures its standard output, error, and return code.
5. Shuts down all services cleanly without leaving zombie background processes.
6. Exits with the exact return code of your test command.

#### Example: GitHub Actions Workflow

```yaml
name: Test Suite
on: [push, pull_request]

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.22'

      - name: Install NativeStackRunner
        run: go install github.com/pandusatrianura/NativeStackRunner/cmd/stack@latest

      - name: Run Test Suite
        run: stack exec --quiet -- go test -v ./...
```

---

### Step 5: Maintenance & Data Reset

- **Stop Running Services:**
  ```bash
  stack down
  ```
- **Reset Project Data:**
  If you want to wipe local databases and start from a fresh state:
  ```bash
  stack clean --force
  ```
  *(Note: This deletes `.stack/data/` for the current project. Cached binaries in your home directory remain untouched).*

---

## Directory Layout in Your Project

When running inside a project, NativeStackRunner creates a localized `.stack` directory:

```
my-project/
├── stack.yaml              # Service configuration file
├── .stack/                 # Created automatically (add to your .gitignore)
│   ├── pids/               # Active process ID tracking files
│   ├── logs/               # Individual service logs (postgres.log, redis.log, minio.log)
│   └── data/               # Persistent database storage
│       ├── postgres/       # PostgreSQL data cluster (PG_VERSION, base, etc.)
│       └── minio/          # Stored S3 bucket objects
```

---

## CLI Command Reference

| Command | Description |
| :--- | :--- |
| `stack init` | Generates a starter `stack.yaml` in the current working directory. |
| `stack up` | Downloads missing binaries and starts all declared dependencies. |
| `stack down` | Gracefully terminates all background services for the project. |
| `stack status` | Displays process status, PIDs, and ports for the current stack. |
| `stack env` | Outputs connection strings and environment variables for client libraries. |
| `stack exec -- <cmd>` | Starts stack, executes command with injected env, and stops stack on exit. |
| `stack clean` | Cleans the `.stack/data` directory to reset local state. |
| `stack version` | Shows the current CLI binary version, git commit, and build date. |

---

## Contributing & Roadmap

- **v1.0 GA (Current):** PostgreSQL 14–16, Redis / Dragonfly, MinIO S3, and `stack exec` test runner.
- **v1.1 (Upcoming):** Mailpit lightweight SMTP email mocking server and web interface.
- **v1.2 (Planned):** Standalone MySQL / MariaDB support.

Pull requests and issue reports are welcome on [GitHub](https://github.com/pandusatrianura/NativeStackRunner).

---

## License

This project is licensed under the [MIT License](LICENSE).
