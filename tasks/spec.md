# Spec: Phase 3 — Public Release & Packaging (v1.0 GA Readiness)

## Objective
Elevate Native Stack Runner (`stack` / `nsr`) from a hardened beta to a production-ready, publicly distributable command-line tool (v1.0 GA). This phase ensures that developers on Windows, macOS, and Linux can reliably download, verify, run, and cross-compile `stack` across all supported environments with zero silent data corruption or platform incompatibilities.

## Target Audience & Use Case
Full-stack engineers and backend developers who need isolated local dependencies (PostgreSQL, Redis, MinIO) without running Docker Desktop, virtual machines, or heavy package managers.

---

## Tech Stack
- **Language:** Go 1.26+ (Pure static binary with `CGO_ENABLED=0`)
- **CLI Framework:** `github.com/spf13/cobra` v1.8.0
- **Configuration:** `gopkg.in/yaml.v3` v3.0.1
- **Testing:** Standard `testing` package & `github.com/stretchr/testify` v1.9.0
- **Terminal UI / Colors:** `github.com/fatih/color` v1.16.0
- **Release Automation:** GoReleaser v2 (`.goreleaser.yaml`) & GitHub Actions

---

## Commands
```bash
# Build local executable
go build -o bin/stack.exe ./cmd/stack

# Run all unit and integration tests
go test -v ./...

# Static analysis and vet
go vet ./...

# Cross-compilation smoke checks
GOOS=linux GOARCH=amd64 go build -o /dev/null ./cmd/stack
GOOS=linux GOARCH=arm64 go build -o /dev/null ./cmd/stack
GOOS=darwin GOARCH=arm64 go build -o /dev/null ./cmd/stack
GOOS=darwin GOARCH=amd64 go build -o /dev/null ./cmd/stack
GOOS=windows GOARCH=amd64 go build -o /dev/null ./cmd/stack

# GoReleaser dry-run snapshot
goreleaser release --snapshot --clean --skip=publish
```

---

## Project Structure
```
NativeStackRunner/
├── .github/
│   └── workflows/
│       ├── ci.yml                 # Multi-platform CI matrix (ubuntu, macos, windows)
│       └── release.yml            # GoReleaser publishing on v* tags
├── .goreleaser.yaml               # Release packaging & artifact configuration
├── cmd/
│   └── stack/
│       ├── main.go                # Entry point
│       ├── root.go                # Root flags & global context
│       ├── up.go                  # Service startup & supervisor lifecycle
│       ├── down.go                # Service termination
│       ├── status.go              # Service health status
│       ├── clean.go               # Cache & data wipe
│       └── integration_test.go    # End-to-end CLI integration tests
├── internal/
│   ├── binary/
│   │   ├── downloader.go          # HTTP download abstraction & temp files
│   │   ├── manifest.go            # Service version & concrete SHA-256 catalog
│   │   ├── manager.go             # Extraction, permissions & hash verification
│   │   ├── progress.go            # Interactive progress bar
│   │   ├── libc.go                # Linux libc detector (glibc vs musl)
│   │   ├── libc_linux.go          # Linux specific libc detection
│   │   ├── libc_other.go          # Non-linux libc stub
│   │   └── libc_test.go           # Libc detector tests
│   ├── config/
│   │   ├── config.go              # stack.yaml & docker-compose parser
│   │   └── config_test.go         # Configuration parser tests
│   └── runner/
│       ├── port.go                # Pre-flight TCP port prober
│       ├── pid.go                 # PID management & process liveness
│       ├── process.go             # Service command builder & PG_VERSION guard
│       ├── process_test.go        # PG_VERSION guard & command builder tests
│       ├── supervisor.go          # Concurrency supervisor & crash watcher
│       └── supervisor_test.go     # Supervisor & runner tests
├── examples/
│   ├── stack.yaml                 # Sample native configuration
│   └── docker-compose.yml         # Sample compose configuration
├── tasks/
│   ├── spec.md                    # This specification
│   ├── plan.md                    # Technical implementation plan
│   └── todo.md                    # Actionable task list
├── go.mod
└── go.sum
```

---

## Code Style & Conventions
Follow standard Go idioms (`go-idioms`):
- Explicit error handling with error wrapping (`fmt.Errorf("...: %w", err)`).
- Custom typed/sentinel errors for expected conditions (`ErrIncompatiblePGVersion`, `ErrIncompatibleLibc`).
- Pure functions for version comparison and parsing.
- Interfaces defined at consumption points, returning concrete types.

### Example Code Style
```go
// ErrIncompatiblePGVersion indicates a mismatch between data directory version and configured version.
var ErrIncompatiblePGVersion = errors.New("incompatible postgresql data directory version")

func ValidatePostgresDataVersion(dataDir, requestedVersion string) error {
	versionFile := filepath.Join(dataDir, "PG_VERSION")
	content, err := os.ReadFile(versionFile)
	if os.IsNotExist(err) {
		return nil // Fresh directory, initdb needed
	}
	if err != nil {
		return fmt.Errorf("reading PG_VERSION: %w", err)
	}

	existingMajor := strings.TrimSpace(string(content))
	requestedMajor := extractMajorVersion(requestedVersion)

	if existingMajor != requestedMajor {
		return fmt.Errorf("%w: data directory is version %s, but configuration requested %s; run 'stack clean' or change stack.yaml",
			ErrIncompatiblePGVersion, existingMajor, requestedMajor)
	}
	return nil
}
```

---

## Testing Strategy
- **Framework:** Go standard `testing` + `testify` for assertions.
- **Unit Testing:**
  - `libc_test.go`: Test musl vs glibc detection logic using filesystem mocks and synthetic `/lib` layout.
  - `process_test.go`: Test `ValidatePostgresDataVersion` with matching, mismatched, and non-existent `PG_VERSION`.
  - `manifest_test.go`: Test that all supported matrix permutations have non-empty URLs, valid binary subpaths, and correct 64-char hex SHA-256 hashes.
- **Integration Testing:**
  - Cross-compilation verification in CI across `linux/amd64`, `linux/arm64`, `darwin/amd64`, `darwin/arm64`, `windows/amd64`.
- **Boundaries:** All tests must run locally and in CI in < 5 seconds without starting external services or making live HTTP calls.

---

## Boundaries
- **Always do:**
  - Run `go test -v ./...` and `go vet ./...` before considering any task complete.
  - Validate and fail closed on corrupt or mismatched binary hashes.
  - Ensure zero external network calls in unit tests.
- **Ask first:**
  - Changing default port allocations or supported default versions.
  - Adding heavy external CGO dependencies.
- **Never do:**
  - Silently overwrite or start PostgreSQL with an incompatible data directory version.
  - Ignore musl libc on Linux and allow cryptic dynamic linker panics.
  - Hardcode developer-specific absolute paths in CI or release configs.

---

## Success Criteria
1. **Concrete SHA-256 Checksums:**
   - `internal/binary/manifest.go` contains valid SHA-256 checksums for all official pinned release binaries.
   - All entries in the resolution matrix (Windows, Linux, macOS) are validated and covered by unit tests.
2. **PostgreSQL Linux Distro (musl vs glibc) Validation:**
   - Detects whether running on a `musl` environment (e.g. Alpine Linux) when target OS is Linux.
   - Emits a helpful, actionable error explaining that glibc is required (or advising `gcompat` installation).
3. **PG_VERSION Data Version Migration Guard:**
   - Reading `PG_VERSION` before startup prevents launching a PostgreSQL instance against mismatched existing data clusters.
   - Clear error instructing the user to either run `stack clean` or update `stack.yaml`.
4. **Packaging & CI/CD Pipeline:**
   - Valid `.goreleaser.yaml` generates cross-platform binaries with archives and `checksums.txt`.
   - `.github/workflows/ci.yml` matrix tests build on Ubuntu, macOS, and Windows.
   - `.github/workflows/release.yml` triggers on tag push to publish GitHub Release.

---

## Resolved Architectural Decisions
1. **Dragonfly vs Native Redis on Linux/macOS:**
   - **Decision:** Confirmed and retained Dragonfly v1.14.0 as the drop-in Redis engine for Linux & macOS.
   - **Rationale:** Dragonfly distributes official static binaries requiring no host glibc / compiler setup, supports 100% RESP2/RESP3 API compatibility, and consumes minimal memory during local dev.
2. **Default PostgreSQL Version:**
   - **Decision:** Confirmed PostgreSQL 16.1.0 as the default version when unspecified, with full backwards compatibility fallback to 15.5.0 and 14.10.0.
   - **Rationale:** Matched with industry framework defaults (Django 5, Spring Boot 3, Prisma), proven stability on Zonky maven bundles across all 5 architectures, and safeguarded by the `PG_VERSION` migration guard.
