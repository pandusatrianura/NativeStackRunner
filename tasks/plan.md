# Implementation Plan: Phase 3 — Public Release & Packaging (v1.0 GA)

## Overview
Decompose and orchestrate the tasks required to transition Native Stack Runner (`stack`) from beta to public release v1.0 GA. This plan establishes data version protection for PostgreSQL, detects Linux C runtime environments (`glibc` vs `musl`), populates immutable SHA-256 binary checksums across the platform matrix, and provisions automated multi-architecture release pipelines via GoReleaser and GitHub Actions.

---

## Architecture Decisions
- **Decision 1: Retain Dragonfly as Default Redis Engine on Unix (Linux & macOS)**
  - *Rationale:* Upstream Redis does not distribute official precompiled static binaries for Unix. Dragonfly provides official, fully static binaries for `x86_64` and `arm64`, guarantees 100% RESP2/RESP3 protocol compliance, and executes reliably without glibc mismatch issues.
- **Decision 2: Default PostgreSQL Version to 16.1.0 with 15.x and 14.x Fallbacks**
  - *Rationale:* PostgreSQL 16 is the standard target for modern backend ecosystems (Prisma, Django 5, Spring Boot 3). Precompiled multi-architecture embedded bundles are verified in the Zonky repository for all 5 target platforms.
- **Decision 3: Zero-CGO Static Cross-Compilation (`CGO_ENABLED=0`)**
  - *Rationale:* Ensures binaries for Linux, macOS, and Windows can be cross-compiled cleanly from any host environment without foreign toolchains, avoiding glibc version locks.

---

## Task List

### Phase 1: Foundation (Data Integrity & Environment Guards)

- [ ] **Task 1: PostgreSQL Data Version Migration Guard (`internal/runner`)**
  - Inspect `.stack/data/postgres/PG_VERSION` before startup.
  - Reject execution if existing data cluster major version differs from `stack.yaml`.
  - Provide actionable error message advising `stack clean` or config rollback.
- [ ] **Task 2: Linux Libc (glibc vs musl) Environment Detector (`internal/binary`)**
  - Implement OS-specific libc detection (`libc_linux.go` and `libc_other.go`).
  - Intercept PostgreSQL startup on Alpine Linux / musl environments with friendly guidance (`gcompat`).

### Checkpoint: Foundation
- [ ] `go test -v ./internal/runner/...` passes 100% (including `TestValidatePostgresDataVersion`).
- [ ] `go test -v ./internal/binary/...` passes 100% (including `TestDetectLibc`).
- [ ] `go vet ./...` reports zero issues.

---

### Phase 2: Checksum Matrix & Manifest Hardening

- [ ] **Task 3: Populate Immutable Release Checksum Matrix in `manifest.go`**
  - Pin immutable release URLs for MinIO, Redis (Windows), Dragonfly (Linux/macOS), and PostgreSQL (Zonky).
  - Populate 64-character SHA-256 hexadecimal hashes for all 15 matrix permutations.
  - Add unit test asserting all permutations have valid, non-empty checksums.

### Checkpoint: Core Manifest
- [ ] All supported matrix permutations resolve valid URLs and 64-character hex SHA-256 checksums.
- [ ] `go test -v ./internal/binary/...` passes 100%.

---

### Phase 3: Packaging, CI/CD & Final Verification

- [ ] **Task 4: Configure GoReleaser Multi-Platform Packaging (`.goreleaser.yaml`)**
  - Configure builds for `windows/amd64`, `linux/amd64`, `linux/arm64`, `darwin/amd64`, `darwin/arm64`.
  - Setup archive generation (`.zip` for Windows, `.tar.gz` for Unix) and `checksums.txt`.
- [ ] **Task 5: GitHub Actions Workflows for Multi-OS CI and Automated Release**
  - Configure `.github/workflows/ci.yml` (multi-OS matrix test on Ubuntu, macOS, Windows).
  - Configure `.github/workflows/release.yml` (automated GoReleaser publishing on tag `v*`).
- [ ] **Task 6: System Regression, Cross-Compilation & Smoke Test Verification**
  - Run full test suite across all packages.
  - Verify cross-compilation dry run for all 5 targets.
  - Update user-facing documentation and execution logs.

### Checkpoint: Complete
- [ ] Complete test suite passes (`go test -v ./...`).
- [ ] All cross-compilation builds exit with code 0.
- [ ] Local binary `bin/stack.exe` is built and verified.
- [ ] Ready for v1.0.0 release tagging.

---

## Risks and Mitigations

| Risk | Impact | Mitigation |
| :--- | :--- | :--- |
| Upstream download assets changing or disappearing | High | Pin immutable release tags (`v1.14.0`, `16.1.0`) with SHA-256 hashes rather than floating tags like `latest`. |
| False positive musl detection on non-standard Linux distributions | Medium | Inspect both loader paths (`/lib/ld-musl*`) and OS release markers (`/etc/alpine-release`); provide clean fallback for standard glibc. |
| Incompatible PostgreSQL data directory causing silent crash or corruption | High | Pre-startup validation of `PG_VERSION` ensures immediate fail-closed protection with clear instructions to run `stack clean`. |
| Cross-compilation failures on developer workstations | Low | Strict enforcement of `CGO_ENABLED=0` guarantees portable pure-Go compilation without external C toolchains. |

---

## Open Questions
*All architectural questions have been reviewed and approved by user.*
