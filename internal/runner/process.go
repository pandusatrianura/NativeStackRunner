package runner

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/pandusatrianura/NativeStackRunner/internal/config"
)

// ErrIncompatiblePGVersion indicates a mismatch between data directory version and configured version.
var ErrIncompatiblePGVersion = errors.New("incompatible postgresql data directory version")

// extractMajorVersion extracts the major version number from a semver or major version string.
// E.g., "16.1.0" -> "16", "15" -> "15", "14.10.0" -> "14". Defaults to "16" if empty.
func extractMajorVersion(version string) string {
	version = strings.TrimSpace(version)
	if version == "" {
		return "16"
	}
	parts := strings.Split(version, ".")
	return parts[0]
}

// ValidatePostgresDataVersion checks whether existing PG_VERSION in dataDir matches requestedVersion.
func ValidatePostgresDataVersion(dataDir, requestedVersion string) error {
	versionFile := filepath.Join(dataDir, "PG_VERSION")
	content, err := os.ReadFile(versionFile)
	if os.IsNotExist(err) {
		return nil // Fresh directory, initdb needed
	}
	if err != nil {
		return fmt.Errorf("reading PG_VERSION from %s: %w", dataDir, err)
	}

	existingMajor := strings.TrimSpace(string(content))
	requestedMajor := extractMajorVersion(requestedVersion)

	if existingMajor != requestedMajor {
		return fmt.Errorf("%w: data directory at %s was initialized with PostgreSQL version %s, but configuration requested %s. Running with mismatched data causes corruption; run 'stack clean' or change version in stack.yaml",
			ErrIncompatiblePGVersion, dataDir, existingMajor, requestedMajor)
	}
	return nil
}

// SpecCmd describes a process execution specification.
type SpecCmd struct {
	Service string
	BinPath string
	Args    []string
	Env     []string
	WorkDir string
}

// ProcessHandle abstracts a running process handle for testability.
type ProcessHandle interface {
	PID() int
	Wait() error
	Kill() error
}

// ProcessExecutor abstracts the launching of operating system processes.
type ProcessExecutor interface {
	Start(ctx context.Context, cmd SpecCmd) (ProcessHandle, io.Reader, io.Reader, error)
}

// OSProcessExecutor is the default OS-backed ProcessExecutor.
type OSProcessExecutor struct{}

type osProcessHandle struct {
	cmd *exec.Cmd
}

func (h *osProcessHandle) PID() int {
	if h.cmd.Process != nil {
		return h.cmd.Process.Pid
	}
	return 0
}

func (h *osProcessHandle) Wait() error {
	return h.cmd.Wait()
}

func (h *osProcessHandle) Kill() error {
	if h.cmd.Process != nil {
		return h.cmd.Process.Kill()
	}
	return nil
}

// Start launches a new child process with piped stdout and stderr.
func (e *OSProcessExecutor) Start(ctx context.Context, spec SpecCmd) (ProcessHandle, io.Reader, io.Reader, error) {
	cmd := exec.CommandContext(ctx, spec.BinPath, spec.Args...)
	cmd.Dir = spec.WorkDir

	if len(spec.Env) > 0 {
		cmd.Env = append(os.Environ(), spec.Env...)
	}

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return nil, nil, nil, fmt.Errorf("creating stdout pipe: %w", err)
	}

	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return nil, nil, nil, fmt.Errorf("creating stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, nil, nil, fmt.Errorf("starting process %s: %w", spec.BinPath, err)
	}

	return &osProcessHandle{cmd: cmd}, stdoutPipe, stderrPipe, nil
}

// BuildServiceCmd generates the execution spec for a specific service.
func BuildServiceCmd(ctx context.Context, svc config.ServiceConfig, binPath, projectDir string) (SpecCmd, error) {
	dataDir := filepath.Join(projectDir, ".stack", "data", svc.Name)
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return SpecCmd{}, fmt.Errorf("creating service data dir %s: %w", dataDir, err)
	}

	spec := SpecCmd{
		Service: svc.Name,
		BinPath: binPath,
		WorkDir: projectDir,
	}

	// Append custom environment variables
	for k, v := range svc.Env {
		spec.Env = append(spec.Env, fmt.Sprintf("%s=%s", k, v))
	}

	switch svc.Name {
	case "minio":
		// Pre-create bucket directories in filesystem storage
		for _, b := range svc.Buckets {
			if b != "" {
				_ = os.MkdirAll(filepath.Join(dataDir, b), 0o755)
			}
		}

		// MinIO syntax: minio server <dataDir> --address :<port> --console-address :<consolePort>
		consolePort := svc.ConsolePort
		if consolePort == 0 {
			consolePort = svc.Port + 1
		}
		spec.Args = []string{
			"server",
			dataDir,
			"--address", fmt.Sprintf(":%d", svc.Port),
			"--console-address", fmt.Sprintf(":%d", consolePort),
		}

		user := svc.AccessKey
		if user == "" {
			user = "minioadmin"
		}
		secret := svc.SecretKey
		if secret == "" {
			secret = "minioadmin"
		}
		spec.Env = append(spec.Env,
			fmt.Sprintf("MINIO_ROOT_USER=%s", user),
			fmt.Sprintf("MINIO_ROOT_PASSWORD=%s", secret),
		)

	case "redis":
		// Redis or Dragonfly syntax
		spec.Args = []string{
			"--port", strconv.Itoa(svc.Port),
			"--dir", dataDir,
		}

	case "postgres":
		// Validate that dataDir version matches configured version if already initialized
		if err := ValidatePostgresDataVersion(dataDir, svc.Version); err != nil {
			return SpecCmd{}, err
		}

		libDir := filepath.Join(filepath.Dir(filepath.Dir(binPath)), "lib")

		// Check if dataDir needs initialization (PG_VERSION check)
		pgVersionFile := filepath.Join(dataDir, "PG_VERSION")
		if _, err := os.Stat(pgVersionFile); os.IsNotExist(err) {
			// Needs initdb
			initdbPath := filepath.Join(filepath.Dir(binPath), "initdb")
			if strings.HasSuffix(binPath, ".exe") {
				initdbPath += ".exe"
			}
			initCmd := exec.CommandContext(ctx, initdbPath, "-D", dataDir, "-U", svc.User, "-A", "trust")
			initCmd.Env = append(os.Environ(),
				fmt.Sprintf("LD_LIBRARY_PATH=%s:%s", libDir, os.Getenv("LD_LIBRARY_PATH")),
				fmt.Sprintf("DYLD_LIBRARY_PATH=%s:%s", libDir, os.Getenv("DYLD_LIBRARY_PATH")),
			)
			initCmd.Dir = projectDir
			if output, err := initCmd.CombinedOutput(); err != nil {
				return SpecCmd{}, fmt.Errorf("running initdb: %w (output: %s)", err, string(output))
			}
		}

		spec.Args = []string{
			"-D", dataDir,
			"-p", strconv.Itoa(svc.Port),
		}

		// Inject LD_LIBRARY_PATH on Linux / DYLD_LIBRARY_PATH on macOS
		spec.Env = append(spec.Env,
			fmt.Sprintf("LD_LIBRARY_PATH=%s:%s", libDir, os.Getenv("LD_LIBRARY_PATH")),
			fmt.Sprintf("DYLD_LIBRARY_PATH=%s:%s", libDir, os.Getenv("DYLD_LIBRARY_PATH")),
		)

	default:
		return SpecCmd{}, fmt.Errorf("unsupported service %q", svc.Name)
	}

	return spec, nil
}
