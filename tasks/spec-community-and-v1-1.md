# Specification: GitHub 100% Community Health, Visual Showcase, & NSR v1.1 Features

> **Target Version:** v1.0.1 (Community Health & Visuals) & v1.1.0 (Detached Mode & Mailpit)  
> **Workflow Phase:** Phase 1 (Specify) — Gated Review

---

## 1. Assumptions & Design Invariants

```
ASSUMPTIONS & INVARIANTS:
1. Community Health items (Issue Templates, CONTRIBUTING.md, SECURITY.md) will achieve a 100% Health Score on GitHub Repository Insights without altering existing Go code.
2. For Detached Mode (`stack up -d`): Docker-compose convention uses `-d` for `--detach`. To avoid Cobra flag collision with `rootCmd -d, --dir`, the `--dir` shorthand will be adjusted to `-C` (matching `git -C` / `make -C`) or kept long-form `--dir`, freeing `-d` for `--detach`.
3. Mailpit is a single portable Go binary from github.com/axllent/mailpit with zero CGo requirements, running on ports 1025 (SMTP) and 8025 (Web UI).
4. All new features must maintain 100% pass rate on `go test -v ./...` across Windows, Linux, and macOS without Docker.
```

---

## 2. Objective & Scope

### Phase 1: GitHub 100% Community Health Standard
- **Bug Report Form:** `.github/ISSUE_TEMPLATE/bug_report.yml` with structured drop-downs for OS, CPU Arch, Go version, `stack.yaml`, and terminal logs.
- **Feature Request Form:** `.github/ISSUE_TEMPLATE/feature_request.yml` for database proposals, CLI ergonomics, and integrations.
- **Issue Configuration:** `.github/ISSUE_TEMPLATE/config.yml` enabling discussions/community links.
- **Contributor Guide:** `CONTRIBUTING.md` covering architecture, development setup, test execution, commit conventions, and PR lifecycle.
- **Security Policy:** `SECURITY.md` establishing a responsible vulnerability disclosure workflow.

### Phase 2: Visual Appeal & Terminal Showcase
- **VHS Script (`assets/demo.tape`):** Automated script for Charm's VHS tool generating `assets/demo.gif` demonstrating the sub-second boot time of `stack up` vs slow container spin-up.
- **README Integration:** Embedded visual badge and showcase link in `README.md`.

### Phase 3: v1.1 Product Features
- **Detached Mode (`stack up --detach`, `-d`):**
  - Spawns services in the background using `supervisor.StartAsync(ctx)`.
  - Waits synchronously until `WaitReady` succeeds (health checks pass, DBs/buckets provisioned).
  - Persists PIDs in `.stack/pids/` and multiplexes logs to `.stack/logs/`.
  - Prints service connection table and exits gracefully with code 0.
  - Lifecycle managed seamlessly via `stack status` and `stack down`.
- **Mailpit Service Integration:**
  - Added to `internal/binary/manifest.go` with cryptographic checksums for Windows, Linux, and macOS.
  - Added to `internal/config/` with fields `port` (default 1025) and `ui_port` (default 8025).
  - Environment variable injection in `internal/config/env.go`: `SMTP_HOST`, `SMTP_PORT`, `MAILPIT_URL`.
  - Runner process builder in `internal/runner/mailpit.go`.

---

## 3. Tech Stack & Dependencies

- **Language:** Go 1.22+
- **CLI Framework:** `github.com/spf13/cobra`
- **Output Styling:** `github.com/fatih/color`
- **Mailpit Binary:** `github.com/axllent/mailpit` official multi-platform releases
- **Visual Recording:** Charm VHS (`vhs < assets/demo.tape`)

---

## 4. Commands

```bash
# Development & Testing
go test -v ./...
go vet ./...
go build -o bin/stack ./cmd/stack

# Detached Mode Execution
./bin/stack up -d
./bin/stack status
./bin/stack down

# Mailpit Stack Execution
./bin/stack up -c examples/stack-with-mailpit.yaml
```

---

## 5. Project Structure Changes

```
NativeStackRunner/
├── .github/
│   ├── ISSUE_TEMPLATE/
│   │   ├── bug_report.yml       # [NEW] Structured GitHub YAML issue form
│   │   ├── feature_request.yml  # [NEW] Feature suggestion YAML form
│   │   └── config.yml           # [NEW] Community links & blank issue toggle
│   ├── pull_request_template.md # [EXISTING]
│   └── workflows/
├── assets/
│   └── demo.tape                # [NEW] Charm VHS terminal recording script
├── CONTRIBUTING.md              # [NEW] Open source contribution guidelines
├── SECURITY.md                  # [NEW] Responsible disclosure policy
├── cmd/stack/
│   ├── root.go                  # [MODIFIED] Free -d shorthand for up command
│   └── up.go                    # [MODIFIED] Add --detach / -d flag
├── internal/
│   ├── binary/
│   │   └── manifest.go          # [MODIFIED] Add Mailpit official download URLs & SHA-256
│   ├── config/
│   │   ├── config.go            # [MODIFIED] Add MailpitConfig struct
│   │   └── env.go               # [MODIFIED] Add SMTP_PORT, MAILPIT_URL variables
│   └── runner/
│       ├── mailpit.go           # [NEW] Process builder & health check for Mailpit
│       └── supervisor.go        # [MODIFIED] Wire mailpit runner
└── examples/
    └── stack.yaml               # [MODIFIED] Include optional mailpit service
```

---

## 6. Code Style & Example

### Example: Detached Flag in `cmd/stack/up.go`
```go
cmd.Flags().BoolVarP(&detach, "detach", "d", false, "Run services in the background and print status")
```

### Example: Mailpit Process Builder
```go
func BuildMailpitCmd(binPath string, port, uiPort int) *exec.Cmd {
    return exec.Command(binPath,
        "--smtp-listen", fmt.Sprintf("127.0.0.1:%d", port),
        "--listen", fmt.Sprintf("127.0.0.1:%d", uiPort),
    )
}
```

---

## 7. Testing Strategy

1. **Unit Tests:**
   - `internal/config`: Test parsing `mailpit` block in `stack.yaml` and env variable injection.
   - `internal/runner`: Test `BuildMailpitCmd` flags, port validation, and health checks.
   - `cmd/stack`: Test `stack up -d` execution, PID persistence, and graceful exit without hanging.
2. **Integration / Smoke Tests:**
   - Execute `stack up -d` against `examples/stack.yaml`, verify process is alive with `stack status`, and terminate cleanly with `stack down`.

---

## 8. Boundaries

- **Always:** Validate all user inputs; ensure PIDs are reliably saved and cleaned; test cross-platform compatibility.
- **Ask First:** Bumping major dependencies or changing default ports.
- **Never:** Leave orphaned background processes when `--detach` or `stack down` fails; never bypass SHA-256 checksum verification.

---

## 9. Success Criteria

1. [ ] GitHub displays 100% score under Insights -> Community Standards (`README.md`, `LICENSE`, `CONTRIBUTING.md`, `SECURITY.md`, Issue templates, PR template).
2. [ ] `stack up -d` starts PostgreSQL, Redis, MinIO, and Mailpit in background, verifies health, outputs status table, and frees terminal immediately.
3. [ ] `stack status` correctly displays running background services and their PIDs.
4. [ ] `stack down` halts all background services cleanly and removes `.stack/pids/`.
5. [ ] `go test -v ./...` passes 100% with 0 warnings.
