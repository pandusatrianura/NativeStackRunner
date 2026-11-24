# Implementation Plan: Phase 2 Beta Hardening

## Goal
Implement the three Phase 2 Beta Hardening features to prepare Native Stack Runner (`stack`) for real-world developer workloads:
1. **SHA-256 Checksum Verification:** Prevent supply-chain tampering and corrupted partial downloads by validating archive/binary hashes before extraction.
2. **Pre-flight Port Prober:** Proactively detect TCP port collisions before any binary download or process launch, giving clear, actionable guidance.
3. **Interactive Download Progress Bar:** Render single-line visual download progress (`[service] [=====>    ] 45% (45MB/100MB)`) in terminal for large asset downloads.

## Assumptions
- Go 1.26 toolchain is available and working.
- SHA-256 verification is enforced whenever a checksum is specified in the manifest spec; if checksum fails, the temporary file is deleted and `ErrChecksumMismatch` is returned.
- Port prober verifies both service `Port` and `ConsolePort` on `127.0.0.1` before starting any service.
- Progress bar uses carriage return (`\r`) for smooth terminal rendering and degrades gracefully when stdout is not a TTY (e.g. CI environments).
- Unit tests remain 100% isolated and testable without external network dependencies.

---

## Plan

### Step 1: SHA-256 Checksum Verification (`internal/binary`)
- **Files:**
  - `internal/binary/manifest.go`
  - `internal/binary/downloader.go`
  - `internal/binary/manager.go`
  - `internal/binary/manager_test.go`
- **Change:**
  - Add `SHA256 string` field to `Spec` in `internal/binary/manifest.go`.
  - Define `ErrChecksumMismatch = errors.New("binary checksum mismatch: potential corruption or security tampering")`.
  - In `internal/binary/manager.go`, implement `verifyChecksum(filePath string, expectedHash string) error` using `crypto/sha256`.
  - In `Manager.EnsureBinary`, call `verifyChecksum` immediately after downloading the asset; if hash mismatch occurs, remove download file and return `ErrChecksumMismatch`.
  - Add unit tests in `manager_test.go` testing:
    1. Correct checksum passes.
    2. Corrupted/altered file triggers `ErrChecksumMismatch` and file is deleted.
    3. Blank checksum (unspecified) passes for backward compatibility.
- **Verify:**
  ```bash
  go test -v ./internal/binary/...
  ```

### Step 2: Pre-flight Port Prober (`internal/runner`)
- **Files:**
  - `internal/runner/port.go`
  - `internal/runner/port_test.go`
  - `internal/runner/supervisor.go`
  - `internal/runner/supervisor_test.go`
- **Change:**
  - Create `internal/runner/port.go`:
    - `CheckPortAvailable(port int) error`: attempts `net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))`. If occupied, returns `ErrPortInUse` with clear instructions.
    - `ValidateServicePorts(services map[string]config.ServiceConfig) error`: validates all service ports and console ports before any service starts.
  - In `internal/runner/supervisor.go`:
    - In `Supervisor.Start`, invoke `ValidateServicePorts(s.cfg.Services)` as the very first step before binary acquisition or process execution.
  - Write unit tests in `port_test.go` and `supervisor_test.go` testing:
    1. Free port validation succeeds.
    2. Occupied port triggers `ErrPortInUse` with offending service name and port number.
    3. Supervisor rejects start before touching filesystem if a port is blocked.
- **Verify:**
  ```bash
  go test -v ./internal/runner/...
  ```

### Step 3: Interactive Download Progress Bar (`internal/binary`)
- **Files:**
  - `internal/binary/progress.go`
  - `internal/binary/downloader.go`
  - `internal/binary/progress_test.go`
- **Change:**
  - Create `internal/binary/progress.go`:
    - `ProgressTracker`: tracks bytes copied, total content length, formats human-readable sizes (KB, MB), and writes single-line updates using `\r`.
    - Handles unknown content lengths (spinning indicator) and detects whether output supports interactive updates.
  - In `internal/binary/downloader.go`:
    - Add `WithProgress(io.Writer)` option to `HTTPDownloader` so progress tracking can be streamed to terminal output without breaking non-interactive callers.
  - Write unit tests in `progress_test.go` testing progress formatting, percentage calculation, and byte transfer tracking.
- **Verify:**
  ```bash
  go test -v ./internal/binary/...
  ```

### Step 4: CLI Integration & End-to-End System Verification
- **Files:**
  - `cmd/stack/up.go`
  - `cmd/stack/integration_test.go`
- **Change:**
  - In `cmd/stack/up.go`, configure `binary.Manager` with interactive progress tracking connected to `os.Stdout`.
  - Add integration tests covering port conflict interception and full CLI workflow.
  - Run full test suite across all packages.
  - Compile final executable `bin/stack.exe`.
- **Verify:**
  ```bash
  go test -v ./...
  go vet ./...
  go build -o bin/stack.exe ./cmd/stack
  .\bin\stack.exe --help
  ```

---

## Risks & Mitigations
1. **Risk:** Port prober creating race conditions if a third-party application claims the port in the millisecond between probing and process binding.
   - *Mitigation:* The existing child process crash monitor (`handle.Wait()`) remains active as a secondary defense to catch late port collisions.
2. **Risk:** Progress bar rendering polluting non-interactive logs or CI output.
   - *Mitigation:* Ensure progress reporter detects non-terminal output or verbose logging mode and disables `\r` carriage return rewrites when output is redirected.

---

## Rollback Plan
- All changes are tracked in git and can be reverted cleanly using `git checkout` if any regression occurs.
