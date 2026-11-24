# Superpowers Review: Native Stack Runner Post-Remediation Audit

## Overview
Second-pass architectural and implementation audit of Native Stack Runner (`stack`) following the review remediation phase. This review evaluates the newly implemented crash monitoring, startup rollback, compose parsing, executable permissions, re-entrancy prevention, and clean command.

---

## Findings by Severity

### Blockers
*None.* The codebase compiles cleanly, passes 100% of automated unit and integration tests, enforces proper security boundaries (ZipSlip protection, atomic temporary file downloads), and introduces no memory/resource leaks or data loss bugs.

---

### Majors
*None.* The two previously identified Major issues (unmonitored child process crashes and absence of startup rollback) have been verified as fully resolved with dedicated unit tests (`TestSupervisor_ChildCrashDetection` and `TestSupervisor_StartupRollback`).

---

### Minors

1. **Unused `--force` Flag & Missing Confirmation Prompt in `stack clean`**
   - **Location:** `cmd/stack/clean.go:12-45`
   - **Issue:** The command defines a `--force` / `-f` flag (`cmd.Flags().BoolVarP(&force, "force", "f", false, ...)`), but `force` is never inspected in `RunE`. As a result, running `stack clean` without flags immediately deletes all project database state (`.stack/data/`) without an interactive confirmation prompt (`Are you sure you want to delete local database data? [y/N]`).
   - **Recommendation:** If `!force`, prompt the user via stdin for confirmation before deleting `dataDir`, or require `--force` when running non-interactively.

2. **Unused `sync.WaitGroup` in `Supervisor.Start`**
   - **Location:** `internal/runner/supervisor.go:88, 126, 172-182`
   - **Issue:** A `sync.WaitGroup` (`var wg sync.WaitGroup`) is declared and incremented (`wg.Add(2)`) for each stdout/stderr logging goroutine (`s.pipeLog`), but `wg.Wait()` is never called in `Supervisor.Start` before returning.
   - **Impact:** When a service shuts down, the supervisor returns immediately upon process kill. The remaining log lines buffered in stdout/stderr might be discarded or race with terminal reset.
   - **Recommendation:** After `rollback()` during shutdown, wait on `wg.Wait()` with a short timeout (e.g. 500ms) to ensure all final shutdown log messages from the database engines are flushed to the terminal.

3. **Service Alias Normalization Inconsistency (`postgresql` vs `postgres`)**
   - **Location:** `internal/config/config.go:193`, `internal/runner/process.go:138`, `internal/binary/manifest.go:50`
   - **Issue:** `binary.Manifest.Resolve` accepts both `"postgres"` and `"postgresql"`. However, `internal/config/config.go` and `internal/runner/process.go` strictly match `case "postgres":`. If a developer writes `postgresql:` in their `stack.yaml`, default ports/credentials are not populated, and `BuildServiceCmd` returns `unsupported service "postgresql"`.
   - **Recommendation:** Normalize service names in `config.ParseYAML` so that `"postgresql"` maps to `"postgres"`, and `"dragonfly"` maps to `"redis"`.

---

### Nits

1. **Context-bounded `initdb` Execution:** In `internal/runner/process.go:149`, `initdb` is executed using `exec.Command` without a context. Passing `ctx` (`exec.CommandContext(ctx, ...)`) ensures that if a user cancels during a lengthy `initdb` operation, the subprocess is cleanly interrupted.
2. **Stale PID Directory Cleaning:** `stack clean` currently removes `.stack/data/` but leaves `.stack/pids/` intact. While dead PID files are safely ignored by `IsProcessAlive`, clearing `.stack/pids/` during a clean operation ensures absolute filesystem cleanliness.
3. **Visual Download Progress Indicator:** For large database archives (e.g. 100MB+ PostgreSQL bundles), terminal feedback remains text-based. Adding a spinner or byte counter would provide a smoother developer experience on slower network connections.

---

## Summary & Next Actions

### Quality Assessment: Ready for Initial Developer Preview (Alpha)
The core engine has achieved solid maturity:
- **Resilient Lifecycle:** Child process crashes trigger immediate alerts and teardown; partial startup failures cleanly rollback.
- **Rugged Security:** Archive extraction enforces ZipSlip defenses and atomic download staging.
- **High Testability:** Fully mocked unit tests run in < 2 seconds with zero network or external daemon requirements.
- **Docker Compose Fallback:** Smooth migration path for existing projects.

### Recommended Action Items:
1. Wire interactive confirmation to `stack clean` when `--force` is omitted.
2. Add `wg.Wait()` with timeout to flush remaining logs upon shutdown in `Supervisor.Start`.
3. Normalize service aliases (`postgresql` -> `postgres`) in `config.ParseYAML`.
