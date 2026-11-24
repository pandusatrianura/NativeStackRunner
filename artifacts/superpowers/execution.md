# Execution Log: Native Stack Runner Scaffolding & PoC Spike

## Step 1: Go Module Initialization & Core Dependencies Setup
- **Files changed:** `go.mod`
- **What changed:**
  - Initialized module `github.com/pandusatrianura/NativeStackRunner` on Go 1.26.4.
  - Added dependencies ready for import (`cobra`, `yaml.v3`, `testify`, `color`).
- **Verification command:** `go mod tidy`
- **Result:** Pass

## Step 2: Configuration Feature Slice (`internal/config`)
- **Files changed:** `internal/config/config.go`, `internal/config/config_test.go`, `go.mod`, `go.sum`
- **What changed:**
  - Defined `StackConfig` and `ServiceConfig` models with default ports and credential conventions.
  - Implemented `ParseYAML` for `stack.yaml` and `ParseCompose` for `docker-compose.yml` fallback.
  - Added port range validation and configuration auto-discovery.
  - Created table-driven unit tests for validation, defaults, and syntax errors.
- **Verification command:** `go test -v ./internal/config/...; go vet ./internal/config/...`
- **Result:** Pass

## Step 3: Binary Asset Manager Feature Slice (`internal/binary`)
- **Files changed:** `internal/binary/downloader.go`, `internal/binary/manifest.go`, `internal/binary/manager.go`, `internal/binary/manager_test.go`
- **What changed:**
  - Created `Downloader` interface and atomic `HTTPDownloader` with `.tmp` staging.
  - Implemented `Manifest` resolution matrix mapping MinIO, Redis, and PostgreSQL across OS/Arch targets.
  - Built `Manager` with ZipSlip-protected extraction (`.zip` and `.tar.gz`) and caching in `~/.stack/cache/`.
  - Added unit tests covering resolution, raw binary cache, zip archive unpacking, and directory traversal defense.
- **Verification command:** `go test -v ./internal/binary/...; go vet ./internal/binary/...`
- **Result:** Pass

## Step 4: Process Supervisor & Per-Project Runtime (`internal/runner`)
- **Files changed:** `internal/runner/pid.go`, `internal/runner/pid_windows.go`, `internal/runner/pid_unix.go`, `internal/runner/process.go`, `internal/runner/supervisor.go`, `internal/runner/supervisor_test.go`
- **What changed:**
  - Implemented PID tracking (`SavePID`, `ReadPID`, `RemovePID`, `ListActivePIDs`) with OS-specific liveness checks (`IsProcessAlive` and `StopProcess`).
  - Defined `ProcessExecutor` and `ProcessHandle` abstractions for isolated unit testing.
  - Built `BuildServiceCmd` command generator for MinIO, Redis, and PostgreSQL with data directory isolation under `./.stack/data/`.
  - Built `Supervisor` with colorized log prefixing (`[service | port]`), dynamic PID saving, and graceful child process shutdown on context cancellation.
  - Added unit tests covering PID lifecycle, command building, and supervisor concurrency.
- **Verification command:** `go test -v ./internal/runner/...; go vet ./internal/runner/...`
- **Result:** Pass

## Step 5: CLI Entry Point & Commands (`cmd/stack`)
- **Files changed:** `cmd/stack/main.go`, `cmd/stack/root.go`, `cmd/stack/up.go`, `cmd/stack/down.go`, `cmd/stack/status.go`, `go.mod`, `go.sum`
- **What changed:**
  - Built Cobra CLI entry point supporting root flags (`--dir`, `--verbose`) and structured logging with `slog`.
  - Implemented `stack up` with automatic configuration discovery, signal interception (`SIGINT`/`SIGTERM`), and supervisor execution.
  - Implemented `stack down` to stop active processes by PID and clean PID files.
  - Implemented `stack status` displaying a formatted tabular list of services, PIDs, and live running states.
- **Verification command:** `go build -o bin/stack.exe ./cmd/stack; .\bin\stack.exe --help; .\bin\stack.exe status`
- **Result:** Pass

## Step 6: PoC Spike Verification with Real Sample Service
- **Files changed:** `examples/stack.yaml`, `cmd/stack/integration_test.go`
- **What changed:**
  - Created declarative `examples/stack.yaml` configuring MinIO, Redis, and PostgreSQL.
  - Added integration tests verifying configuration loading, TCP probing helper, active/stopped status reporting, and context cancellation lifecycle.
  - Ran full test suite across all packages (`cmd/stack`, `internal/config`, `internal/binary`, `internal/runner`).
- **Verification command:** `go test -v ./...; go vet ./...; go build -o bin/stack.exe ./cmd/stack; .\bin\stack.exe status`
- **Result:** Pass

## Step 1: Supervisor Child Crash Monitoring & Partial Startup Rollback
- **Files changed:** `internal/runner/supervisor.go`, `internal/runner/supervisor_test.go`
- **What changed:**
  - Added startup rollback mechanism: if any service fails to download, build, or start, all previously-spawned handles are immediately killed and their PID files removed.
  - Implemented proactive child crash detection: spawned background watcher goroutines for each process handle calling `Wait()`. An unexpected exit triggers a graceful teardown and returns an explicit error.
  - Added unit tests `TestSupervisor_ChildCrashDetection` and `TestSupervisor_StartupRollback`.
- **Verification command:** `go test -v ./internal/runner/...`
- **Result:** Pass

## Step 2: Robust Docker Compose Port Parsing for IP Bindings
- **Files changed:** `internal/config/config.go`, `internal/config/config_test.go`
- **What changed:**
  - Upgraded `extractHostPort` to clean quotes, trim protocol suffixes (`/tcp`), and correctly extract the host port from IP-bound declarations such as `"127.0.0.1:5433:5432"` and `"0.0.0.0:6380:6379/tcp"`.
  - Added unit test assertions verifying IP-bound port parsing in `TestParseCompose`.
- **Verification command:** `go test -v ./internal/config/...`
- **Result:** Pass

## Step 3: Executable Permissions on Sibling Binaries
- **Files changed:** `internal/binary/manager.go`, `internal/binary/manager_test.go`
- **What changed:**
  - Implemented `ensureExecutablePermissions` to recursively ensure executable permissions (`0755`) for all binary files (including PostgreSQL siblings `initdb`, `pg_ctl`, etc.) upon extraction.
  - Added unit test `TestEnsureBinary_SiblingPermissions` verifying sibling executable presence and permissions.
- **Verification command:** `go test -v ./internal/binary/...`
- **Result:** Pass

## Step 4: Re-entrancy Check for `stack up` & New `stack clean` Command
- **Files changed:** `cmd/stack/up.go`, `cmd/stack/clean.go`, `cmd/stack/root.go`, `cmd/stack/integration_test.go`
- **What changed:**
  - Added re-entrancy prevention in `stack up` by detecting if live processes already exist in `.stack/pids/` and warning the user.
  - Implemented `stack clean` to safely remove `./.stack/data/` for clean state resets, with safeguards preventing deletion while services are still active.
  - Registered `clean` command in Cobra root command.
  - Added unit test `TestCleanCommand_DataRemoval` in `integration_test.go`.
- **Verification command:** `go test -v ./cmd/stack/...; go build -o bin/stack.exe ./cmd/stack; .\bin\stack.exe clean --help`
- **Result:** Pass

## Step 5: Full Regression & System Verification
- **Files changed:** Repository-wide
- **What changed:**
  - Ran complete test suite across all packages: `cmd/stack`, `internal/config`, `internal/binary`, `internal/runner`.
  - Confirmed 0 build errors, 0 test failures, and 0 `go vet` warnings.
  - Recompiled CLI binary `bin/stack.exe` and validated all command subtrees (`up`, `down`, `status`, `clean`).
- **Verification command:** `go test -v ./...; go vet ./...; .\bin\stack.exe --help`
- **Result:** Pass

## Step 1: Service Alias Normalization & Context-Bounded `initdb`
- **Files changed:** `internal/config/config.go`, `internal/config/config_test.go`, `internal/runner/process.go`, `internal/runner/supervisor.go`, `internal/runner/supervisor_test.go`
- **What changed:**
  - Implemented `normalizeServiceName` in `internal/config/config.go` mapping aliases (`postgresql`, `pg` -> `postgres`; `dragonfly` -> `redis`).
  - Updated `BuildServiceCmd` in `internal/runner/process.go` to accept `context.Context` and run `initdb` with `exec.CommandContext(ctx, ...)`.
  - Added unit test in `config_test.go` verifying service alias mapping and defaults.
- **Verification command:** `go test -v ./internal/config/...; go test -v ./internal/runner/...`
- **Result:** Pass

## Step 2: Log Draining Synchronization with Timeout in Supervisor
- **Files changed:** `internal/runner/supervisor.go`
- **What changed:**
  - Implemented `drainLogs(wg *sync.WaitGroup, timeout time.Duration)` to synchronize and flush trailing stdout/stderr lines from child processes upon teardown.
  - Added bounded 500ms timeout guard to ensure supervisor shutdown never blocks indefinitely if an OS pipe remains open.
- **Verification command:** `go test -v ./internal/runner/...`
- **Result:** Pass

## Step 3: Interactive Confirmation Prompt & Stale PID Cleanup in `stack clean`
- **Files changed:** `cmd/stack/clean.go`, `cmd/stack/integration_test.go`
- **What changed:**
  - Added interactive confirmation prompt to `stack clean` when `--force` is omitted, reading user confirmation from `os.Stdin`.
  - Added stale `.stack/pids/` directory removal during `clean` to leave the project directory completely pristine.
  - Added unit test in `integration_test.go` verifying dual-directory removal (`.stack/data` and `.stack/pids`).
- **Verification command:** `go test -v ./cmd/stack/...; go build -o bin/stack.exe ./cmd/stack; .\bin\stack.exe clean --help; .\bin\stack.exe clean --force`
- **Result:** Pass

## Step 4: Full Regression & System Verification
- **Files changed:** Repository-wide
- **What changed:**
  - Ran complete test suite across all packages (`cmd/stack`, `internal/config`, `internal/binary`, `internal/runner`).
  - Validated static analysis via `go vet ./...` (0 warnings).
  - Recompiled CLI binary `bin/stack.exe` and verified `--help` command tree.
- **Verification command:** `go test -v ./...; go vet ./...; .\bin\stack.exe --help`
- **Result:** Pass

## Step 1: SHA-256 Checksum Verification (`internal/binary`)
- **Files changed:** `internal/binary/manifest.go`, `internal/binary/downloader.go`, `internal/binary/manager.go`, `internal/binary/manager_test.go`
- **What changed:**
  - Added `SHA256` field to `Spec` struct in `manifest.go`.
  - Defined `ErrChecksumMismatch` in `downloader.go`.
  - Implemented `VerifyChecksum` in `manager.go` using `crypto/sha256` and integrated verification before extracting downloaded assets in `Manager.EnsureBinary`.
  - Added unit test `TestVerifyChecksum` in `manager_test.go` covering valid hash, empty optional hash, and mismatch rejection.
- **Verification command:** `go test -v ./internal/binary/...`
- **Result:** Pass

## Step 2: Pre-flight Port Prober (`internal/runner`)
- **Files changed:** `internal/runner/port.go`, `internal/runner/port_test.go`, `internal/runner/supervisor.go`, `internal/runner/supervisor_test.go`
- **What changed:**
  - Implemented `CheckPortAvailable(port int)` and `ValidateServicePorts(services)` checking TCP ports on `127.0.0.1`.
  - Added pre-flight validation in `Supervisor.Start` as step 0, proactively aborting before downloading assets or spawning processes if any port is already in use.
  - Added unit tests `TestCheckPortAvailable_FreeAndOccupied`, `TestValidateServicePorts`, and `TestSupervisor_PreflightPortConflict`.
- **Verification command:** `go test -v ./internal/runner/...`
- **Result:** Pass

## Step 3: Interactive Download Progress Bar (`internal/binary`)
- **Files changed:** `internal/binary/progress.go`, `internal/binary/downloader.go`, `internal/binary/manager.go`, `internal/binary/progress_test.go`
- **What changed:**
  - Implemented `ProgressWriter` that tracks download progress, formats human-readable sizes (`FormatBytes`), and renders single-line terminal progress updates.
  - Added `SetProgressOutput(w io.Writer)` to `HTTPDownloader` and `Manager` to optionally stream progress to the user's terminal.
  - Added unit tests in `progress_test.go` covering byte formatting, determinate progress calculation, and indeterminate status.
- **Verification command:** `go test -v ./internal/binary/...`
- **Result:** Pass

## Step 4: CLI Integration & End-to-End System Verification
- **Files changed:** `cmd/stack/up.go`, `cmd/stack/integration_test.go`
- **What changed:**
  - Connected interactive progress bar to CLI stdout in `cmd/stack/up.go`.
  - Added integration test `TestPreflightPortConflict_Integration` in `cmd/stack/integration_test.go`.
  - Ran comprehensive regression suite across all packages (`cmd/stack`, `internal/config`, `internal/binary`, `internal/runner`).
  - Successfully compiled final CLI binary `bin/stack.exe`.
- **Verification command:** `go test -v ./...; go vet ./...; go build -o bin/stack.exe ./cmd/stack; .\bin\stack.exe --help`
- **Result:** Pass
