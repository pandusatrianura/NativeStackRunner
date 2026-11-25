# Contributing to NativeStackRunner

Thank you for your interest in contributing to **NativeStackRunner (`stack`)**! We welcome contributions of all kinds: bug fixes, new service runners, documentation improvements, and performance optimizations.

---

## 🏛️ Guiding Philosophy

1. **Zero Docker Dependency:** Services run as native host processes in user-space. We never spin up containers, virtualization daemons, or WSL2 VMs.
2. **Sub-Second Speed:** Boot times should remain in the hundreds of milliseconds.
3. **Project Scoped Isolation:** Data, logs, and PID locks stay within `<project-dir>/.stack/`.
4. **Supply Chain Integrity:** All precompiled third-party binaries must be verified using cryptographic SHA-256 checksums before execution.

---

## 🛠️ Development Setup

### Prerequisites
- [Go](https://go.dev/dl/) 1.22 or higher
- Git

### Getting the Code
```bash
git clone https://github.com/pandusatrianura/NativeStackRunner.git
cd NativeStackRunner
go mod download
```

### Running Tests & Static Analysis
Always ensure all tests pass and `go vet` reports zero issues before submitting changes:

```bash
# Run all unit and integration tests
go test -v ./...

# Run static analysis
go vet ./...

# Build local binary
go build -o bin/stack ./cmd/stack
```

---

## 📁 Codebase Architecture

```
NativeStackRunner/
├── cmd/stack/             # CLI commands (root, up, down, status, env, exec, clean, init, version)
├── internal/
│   ├── binary/            # Binary resolution, downloading, caching, SHA-256 verification, libc detection
│   ├── config/            # YAML configuration loading, parsing, and environment variable builders
│   └── runner/            # Process supervision, port health checking, PID tracking, and service builders
└── examples/              # Sample stack.yaml configuration files
```

---

## 🚀 Pull Request Workflow

1. **Fork & Branch:** Create a feature branch with a descriptive name (e.g. `feat/mailpit-support` or `fix/windows-pid-cleanup`).
2. **Follow Conventions:**
   - Write clear, idiomatic Go code.
   - Abstract I/O operations behind testable interfaces.
   - Use conventional commit messages:
     - `feat:` for new capabilities
     - `fix:` for bug fixes
     - `docs:` for documentation updates
     - `test:` for adding or improving tests
     - `refactor:` for internal code cleanup
3. **Verify Locally:**
   ```bash
   go test -v ./... && go vet ./...
   ```
4. **Submit PR:** Open a Pull Request on GitHub against the `main` branch. Describe the changes, motivation, and test coverage provided.

---

## 📜 Code of Conduct

We are committed to providing a welcoming, inclusive, and harassment-free environment for everyone. Please be respectful and collaborative in all discussions and reviews.
