# Implementation Plan: GitHub 100% Community Health, Visual Showcase, & NSR v1.1 Features

> **Tracking:** `tasks/plan-community-and-v1-1.md`  
> **Based on Spec:** `tasks/spec-community-and-v1-1.md`  
> **Target Releases:** v1.0.1 (Community & Visuals) & v1.1.0 (Detached Mode & Mailpit)

---

## 1. Overview

This implementation plan decomposes the work into small, verifiable vertical slices. The work is organized into three distinct phases:
1. **Phase 1: Repository Health & Visual Polish:** Deliver GitHub 100% Community Standards compliance and Charm VHS terminal demo recording.
2. **Phase 2: Detached Execution Engine (`stack up -d`):** Free the terminal for developers by launching background services, waiting for TCP readiness, and gracefully handing off control to `stack status` and `stack down`.
3. **Phase 3: Mailpit Integration (4th Core Dependency):** Expand the Golden Trio to include mock SMTP email testing and Web UI with zero external dependencies.

---

## 2. Architecture Decisions & Dependency Graph

```
Phase 1: Community & Visuals (Zero Code Impact)
├── .github/ISSUE_TEMPLATE/*.yml
├── CONTRIBUTING.md & SECURITY.md
└── assets/demo.tape

Phase 2: Detached Mode Execution
├── cmd/stack/root.go (Remap --dir to -C, release -d)
│       └── cmd/stack/up.go (Add --detach / -d, wire StartAsync + WaitReady)
│               └── cmd/stack/up_test.go (Unit & CLI tests)

Phase 3: Mailpit Integration
├── internal/config/config.go & env.go (Schema & Env vars)
│       └── internal/binary/manifest.go (Multi-platform binary resolver)
│               └── internal/runner/mailpit.go (Process runner)
│                       └── internal/runner/supervisor.go (Lifecycle wiring)
│                               └── Test suites
```

---

## 3. Task List & Breakdown

### Phase 1: GitHub Community Health & Visual Polish (v1.0.1)

#### Task 1: GitHub Issue Forms (`bug_report.yml`, `feature_request.yml`, `config.yml`)
- **Description:** Create structured GitHub YAML issue forms enabling rich issue submissions with drop-downs for OS, CPU architecture, `stack version`, and `stack.yaml`.
- **Acceptance Criteria:**
  - [ ] `.github/ISSUE_TEMPLATE/bug_report.yml` validates with required OS, version, and log fields.
  - [ ] `.github/ISSUE_TEMPLATE/feature_request.yml` includes problem, proposed solution, and alternatives.
  - [ ] `.github/ISSUE_TEMPLATE/config.yml` defines community contact links.
- **Verification:** Inspect YAML syntax validity with `Get-Content` and schema validation.
- **Files:** `.github/ISSUE_TEMPLATE/bug_report.yml`, `.github/ISSUE_TEMPLATE/feature_request.yml`, `.github/ISSUE_TEMPLATE/config.yml`
- **Scope:** Small (3 files, documentation/config only).

#### Task 2: Open Source Community Documents (`CONTRIBUTING.md`, `SECURITY.md`)
- **Description:** Author comprehensive contributor guidelines and security vulnerability disclosure policy.
- **Acceptance Criteria:**
  - [ ] `CONTRIBUTING.md` details local development setup, `go test` workflows, code idioms, and PR conventions.
  - [ ] `SECURITY.md` defines supported versions and private vulnerability reporting contact.
- **Verification:** Confirm files exist in repository root.
- **Files:** `CONTRIBUTING.md`, `SECURITY.md`
- **Scope:** Small (2 files).

#### Task 3: Terminal Demo Recording Script (`assets/demo.tape`)
- **Description:** Create a Charm VHS tape script to generate a high-contrast GIF showing `stack up` booting in < 300ms vs slow container spin-up.
- **Acceptance Criteria:**
  - [ ] `assets/demo.tape` includes setup commands, typing emulation, and output pausing.
- **Verification:** Script syntax check.
- **Files:** `assets/demo.tape`
- **Scope:** XS (1 file).

---

### Checkpoint 1: Community Health Verification
- [ ] Confirm all community standard files (`README.md`, `LICENSE`, `CONTRIBUTING.md`, `SECURITY.md`, Issue Templates, PR Template) exist.
- [ ] Git commit and push to remote.

---

### Phase 2: Detached Execution Engine (stack up -d)

#### Task 4: Reclaim Flag Shorthand `-d` on `rootCmd`
- **Description:** Remap `rootCmd` flag `--dir` from shorthand `-d` to `-C` (matching standard `git -C` / `make -C`), freeing `-d` for `--detach` on `stack up`.
- **Acceptance Criteria:**
  - [ ] `stack --help` displays `-C, --dir string`.
  - [ ] Existing `--dir` long-form continues to work seamlessly.
- **Verification:** Run `go test ./cmd/stack/...` and verify help output.
- **Files:** `cmd/stack/root.go`
- **Scope:** XS (1 file).

#### Task 5: Implement Detached Mode in `cmd/stack/up.go`
- **Description:** Add `--detach, -d` flag to `stack up`. When set, launch the stack using `supervisor.StartAsync(ctx)`, block synchronously on `stack.WaitReady(ctx, 30*time.Second)`, print connection summary, and exit with code 0 while child processes run in the background.
- **Acceptance Criteria:**
  - [ ] `stack up -d` boots services, writes PIDs to `.stack/pids/`, and exits without terminating child processes.
  - [ ] `stack status` detects active running services and PIDs.
  - [ ] `stack down` terminates background processes cleanly.
- **Verification:** CLI smoke test with `stack up -d`, `stack status`, `stack down`.
- **Files:** `cmd/stack/up.go`
- **Scope:** S (1-2 files).

#### Task 6: Unit and CLI Tests for Detached Mode
- **Description:** Add unit tests validating detached mode flag parsing, readiness waiting, and non-blocking exit.
- **Acceptance Criteria:**
  - [ ] Test validates `--detach` flag initiates async startup and calls `WaitReady`.
  - [ ] Test validates that unexpected child crash during detached startup triggers error and rollback.
- **Verification:** `go test -v ./cmd/stack/... -run TestUpCommand`
- **Files:** `cmd/stack/up_test.go`
- **Scope:** S (1 file).

---

### Checkpoint 2: Detached Mode Verification
- [ ] `go test -v ./...` passes 100%.
- [ ] Live execution of `stack up -d` -> `stack status` -> `stack down` runs cleanly.

---

### Phase 3: Mailpit Service Integration (v1.1.0)

#### Task 7: Configuration Schema & Environment Injection for Mailpit
- **Description:** Add `MailpitConfig` struct to `internal/config/config.go` with ports (default SMTP 1025, Web UI 8025). Add `SMTP_HOST`, `SMTP_PORT`, `MAILPIT_URL` to `internal/config/env.go`.
- **Acceptance Criteria:**
  - [ ] `stack.yaml` parses `services.mailpit.port` and `services.mailpit.ui_port`.
  - [ ] `BuildEnvironment` injects `SMTP_PORT=1025` and `MAILPIT_URL=http://localhost:8025`.
- **Verification:** `go test -v ./internal/config/...`
- **Files:** `internal/config/config.go`, `internal/config/config_test.go`, `internal/config/env.go`, `internal/config/env_test.go`
- **Scope:** S (4 files).

#### Task 8: Mailpit Binary Resolver & SHA-256 Checksums
- **Description:** Add Mailpit official release download URLs and verified SHA-256 checksums to `internal/binary/manifest.go` for Linux, macOS, and Windows (amd64/arm64).
- **Acceptance Criteria:**
  - [ ] `manifest.go` defines `mailpit` binary spec with official tar.gz/zip URLs from `github.com/axllent/mailpit`.
- **Verification:** `go test -v ./internal/binary/...`
- **Files:** `internal/binary/manifest.go`
- **Scope:** XS (1 file).

#### Task 9: Mailpit Runner Process Builder & Supervisor Wiring
- **Description:** Create `internal/runner/mailpit.go` to construct the CLI invocation (`mailpit --smtp-listen 127.0.0.1:1025 --listen 127.0.0.1:8025`). Wire into `Supervisor.StartAsync`.
- **Acceptance Criteria:**
  - [ ] `BuildMailpitCmd` produces exact flag arguments.
  - [ ] Supervisor manages Mailpit lifecycle, logs, and PID tracking.
  - [ ] Preflight port validator checks Mailpit SMTP and UI ports.
- **Verification:** `go test -v ./internal/runner/...`
- **Files:** `internal/runner/mailpit.go`, `internal/runner/mailpit_test.go`, `internal/runner/supervisor.go`
- **Scope:** M (3 files).

#### Task 10: Update Examples and Documentation for Mailpit
- **Description:** Update `examples/stack.yaml` and `README.md` to document the mailpit service configuration and default environment variables.
- **Acceptance Criteria:**
  - [ ] `examples/stack.yaml` contains sample `mailpit` service block.
  - [ ] `README.md` lists Mailpit under Supported Services and CLI reference.
- **Verification:** Linting and manual review.
- **Files:** `examples/stack.yaml`, `README.md`
- **Scope:** S (2 files).

---

### Checkpoint 3: Complete v1.1.0 Verification
- [ ] `go test -v ./...` passes 100% across all packages.
- [ ] `go vet ./...` reports 0 issues.
- [ ] `stack exec -c examples/stack.yaml -- go test ./...` verifies Golden Trio + Mailpit live execution.
- [ ] Tag `v1.1.0` ready to ship.

---

## 4. Risks and Mitigations

| Risk | Impact | Mitigation |
| :--- | :---: | :--- |
| **Breaking `--dir` CLI flag shorthand** | Low | Only the single-letter shorthand `-d` changes to `-C`; the full `--dir` flag remains 100% backward compatible. |
| **Mailpit port collision (1025/8025)** | Medium | Preflight port checker (`ValidateServicePorts`) automatically catches collisions before starting any service. |
| **Process leak on detached mode** | High | PIDs are persistently stored in `.stack/pids/mailpit.pid` and `stack down` guarantees termination via signal forwarding. |
