# Task Breakdown: Phase 3 — Public Release & Packaging (v1.0 GA)

## Phase 1: Foundation (Data Integrity & Environment Guards)

### Task 1: PostgreSQL Data Version Migration Guard
**Description:** Implement pre-flight validation of PostgreSQL data directory version against configured version in `stack.yaml`. Reading `.stack/data/postgres/PG_VERSION` before startup prevents launching PostgreSQL instances against incompatible existing data clusters, avoiding runtime crashes or data corruption.

**Acceptance criteria:**
- [ ] `ValidatePostgresDataVersion(dataDir, requestedVersion string) error` is implemented in `internal/runner/process.go`.
- [ ] If `PG_VERSION` does not exist (fresh directory), validation succeeds and allows `initdb`.
- [ ] If `PG_VERSION` major version matches requested major version (e.g. `"16"` vs `"16.1.0"`), validation succeeds.
- [ ] If `PG_VERSION` major version differs (e.g. `"15"` vs `"16.1.0"`), returns `ErrIncompatiblePGVersion` with a clear explanation instructing the user to run `stack clean` or adjust `stack.yaml`.
- [ ] `BuildServiceCmd` invokes `ValidatePostgresDataVersion` before running `initdb` or spawning `postgres`.

**Verification:**
- [ ] Tests pass: `go test -v ./internal/runner/... -run TestValidatePostgresDataVersion`
- [ ] Build succeeds: `go vet ./internal/runner/...`

**Dependencies:** None

**Files likely touched:**
- `internal/runner/process.go`
- `internal/runner/process_test.go`

**Estimated scope:** Small (2 files)

---

### Task 2: Linux Libc (glibc vs musl) Environment Detector
**Description:** Build a runtime C library detector for Linux to determine if the host system is running `musl` (e.g. Alpine Linux) or standard `glibc`. Because Zonky PostgreSQL bundles require glibc, attempting to run on musl produces cryptic dynamic linker errors. The detector enables `stack` to intercept execution on Alpine and give clear guidance (`gcompat`).

**Acceptance criteria:**
- [ ] Create `internal/binary/libc.go` defining `LibcType` (`LibcGlibc`, `LibcMusl`, `LibcUnknown`) and sentinel error `ErrIncompatibleLibc`.
- [ ] Implement `libc_linux.go` inspecting loader paths (`/lib/ld-musl-*`) and release files (`/etc/alpine-release`).
- [ ] Implement `libc_other.go` for non-Linux OS (Windows, macOS) returning `LibcUnknown, nil`.
- [ ] In `Manager.EnsureBinary`, when `runtime.GOOS == "linux"`, service is `"postgres"`, and libc is `LibcMusl`, return `ErrIncompatibleLibc`.
- [ ] Add unit tests in `libc_test.go` verifying detection logic with mocked filesystem helpers.

**Verification:**
- [ ] Tests pass: `go test -v ./internal/binary/... -run TestDetectLibc`
- [ ] Build succeeds: `go vet ./internal/binary/...`

**Dependencies:** None

**Files likely touched:**
- `internal/binary/libc.go`
- `internal/binary/libc_linux.go`
- `internal/binary/libc_other.go`
- `internal/binary/libc_test.go`
- `internal/binary/manager.go`

**Estimated scope:** Medium (5 files)

---

### Checkpoint 1: Foundation Guards
- [ ] All tests in `internal/runner` pass (`go test -v ./internal/runner/...`)
- [ ] All tests in `internal/binary` pass (`go test -v ./internal/binary/...`)
- [ ] Static analysis is clean (`go vet ./...`)

---

## Phase 2: Checksum Matrix & Manifest Hardening

### Task 3: Populate Immutable Release Checksum Matrix in `manifest.go`
**Description:** Update `internal/binary/manifest.go` to provide immutable release URLs and concrete SHA-256 hashes for all supported services, operating systems, and architectures (Windows, Linux, macOS on amd64 and arm64). This activates the Phase 2 cryptographic download verification engine for all production downloads.

**Acceptance criteria:**
- [ ] MinIO resolution points to pinned immutable release with official SHA-256 for all 5 platform targets.
- [ ] Redis resolution specifies pinned Windows release (v5.0.14.1) with verified SHA-256.
- [ ] Dragonfly resolution specifies pinned release (v1.14.0) with official SHA-256 for Linux and macOS (amd64 and arm64).
- [ ] PostgreSQL resolution specifies pinned Zonky maven JARs with verified SHA-256 for versions 16.1.0, 15.5.0, 14.10.0 across all OS/Arch permutations.
- [ ] Add comprehensive unit test `TestManifestResolve_AllPermutationsHaveChecksum` iterating through every supported matrix permutation, asserting valid URL and 64-character hex SHA-256 hash.

**Verification:**
- [ ] Tests pass: `go test -v ./internal/binary/... -run TestManifestResolve`
- [ ] Static check: `go vet ./internal/binary/...`

**Dependencies:** Tasks 1, 2

**Files likely touched:**
- `internal/binary/manifest.go`
- `internal/binary/manifest_test.go`

**Estimated scope:** Small (2 files)

---

### Checkpoint 2: Manifest & Integrity
- [ ] Matrix test validates 100% of supported targets have non-empty SHA-256 checksums
- [ ] `go test -v ./internal/binary/...` passes without network dependencies

---

## Phase 3: Packaging, CI/CD & Final Verification

### Task 4: Configure GoReleaser Multi-Platform Packaging
**Description:** Author `.goreleaser.yaml` to enable reproducible, automated cross-compilation and artifact packaging for `stack` across all target architectures.

**Acceptance criteria:**
- [ ] Create `.goreleaser.yaml` supporting `windows/amd64` (`stack.exe`), `linux/amd64`, `linux/arm64`, `darwin/amd64`, and `darwin/arm64` (`stack`).
- [ ] Enforces pure static compilation with `CGO_ENABLED=0` and production ldflags (`-s -w -trimpath`).
- [ ] Configures archive formats: `.zip` for Windows and `.tar.gz` for Linux and macOS.
- [ ] Configures SHA-256 checksums file generation (`checksums.txt`).

**Verification:**
- [ ] YAML syntax validation
- [ ] Dry-run cross-compilation smoke tests:
  - `GOOS=linux GOARCH=amd64 go build -o bin/dryrun-linux ./cmd/stack`
  - `GOOS=darwin GOARCH=arm64 go build -o bin/dryrun-darwin ./cmd/stack`
  - Clean up temporary dry-run binaries.

**Dependencies:** Task 3

**Files likely touched:**
- `.goreleaser.yaml`

**Estimated scope:** Small (1 file)

---

### Task 5: GitHub Actions Workflows for Multi-OS CI and Automated Release
**Description:** Set up automated continuous integration and release workflows using GitHub Actions. The CI workflow verifies pull requests across Windows, Linux, and macOS runners. The release workflow triggers GoReleaser on git tag pushes.

**Acceptance criteria:**
- [ ] Create `.github/workflows/ci.yml` running matrix tests across `ubuntu-latest`, `macos-latest`, and `windows-latest` on push/PR to `main`.
- [ ] CI runs `go test -v ./...` and `go vet ./...`.
- [ ] Create `.github/workflows/release.yml` triggering on tag push matching `v*` and running GoReleaser action with `GITHUB_TOKEN`.

**Verification:**
- [ ] Verify GitHub Actions YAML workflow syntax

**Dependencies:** Task 4

**Files likely touched:**
- `.github/workflows/ci.yml`
- `.github/workflows/release.yml`

**Estimated scope:** Small (2 files)

---

### Task 6: System Regression, Cross-Compilation & Final Verification
**Description:** Execute full repository-wide regression testing, compile the final production binary, perform cross-compilation checks, and verify all CLI commands.

**Acceptance criteria:**
- [ ] `go test -v ./...` passes 100% across all packages (`cmd/stack`, `internal/config`, `internal/binary`, `internal/runner`).
- [ ] `go vet ./...` reports zero warnings.
- [ ] Cross-compilation for all 5 targets succeeds without warnings.
- [ ] Compile production binary `bin/stack.exe`.
- [ ] Execute `.\bin\stack.exe --help` and verify command tree.

**Verification:**
- [ ] `go test -v ./...`
- [ ] `go vet ./...`
- [ ] `go build -o bin/stack.exe ./cmd/stack`
- [ ] `.\bin\stack.exe --help`

**Dependencies:** Tasks 1-5

**Files likely touched:**
- Repository-wide / documentation updates

**Estimated scope:** Medium (documentation & binary)

---

### Checkpoint 3: Complete GA Release Readiness
- [ ] Complete test suite passes (`go test -v ./...`)
- [ ] All cross-compilation builds exit with code 0
- [ ] Local binary `bin/stack.exe` is built and functional
- [ ] All tasks in `tasks/todo.md` checked off
