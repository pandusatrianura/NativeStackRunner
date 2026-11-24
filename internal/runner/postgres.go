package runner

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// CmdRunner abstracts executing an external helper command for testing.
type CmdRunner func(ctx context.Context, name string, args []string, env []string, dir string) ([]byte, error)

// DefaultCmdRunner executes commands using standard OS exec.
func DefaultCmdRunner(ctx context.Context, name string, args []string, env []string, dir string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	if len(env) > 0 {
		cmd.Env = env
	}
	return cmd.CombinedOutput()
}

// WaitForTCPReady polls targetAddr until a TCP connection succeeds or timeout expires.
func WaitForTCPReady(ctx context.Context, targetAddr string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		if time.Now().After(deadline) {
			return fmt.Errorf("timeout waiting for %s to be ready", targetAddr)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		conn, err := net.DialTimeout("tcp", targetAddr, 100*time.Millisecond)
		if err == nil {
			_ = conn.Close()
			return nil
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// EnsurePostgresDatabase ensures the specified database exists on the running PostgreSQL instance.
func EnsurePostgresDatabase(ctx context.Context, runner CmdRunner, binPath string, port int, user, database, projectDir string) error {
	if database == "" || database == "postgres" {
		return nil
	}
	if runner == nil {
		runner = DefaultCmdRunner
	}

	createdbPath := filepath.Join(filepath.Dir(binPath), "createdb")
	if strings.HasSuffix(binPath, ".exe") {
		createdbPath += ".exe"
	}

	libDir := filepath.Join(filepath.Dir(filepath.Dir(binPath)), "lib")
	env := append(os.Environ(),
		fmt.Sprintf("LD_LIBRARY_PATH=%s:%s", libDir, os.Getenv("LD_LIBRARY_PATH")),
		fmt.Sprintf("DYLD_LIBRARY_PATH=%s:%s", libDir, os.Getenv("DYLD_LIBRARY_PATH")),
	)

	args := []string{
		"-h", "127.0.0.1",
		"-p", fmt.Sprintf("%d", port),
		"-U", user,
		database,
	}

	output, err := runner(ctx, createdbPath, args, env, projectDir)
	if err != nil {
		outStr := string(output)
		if strings.Contains(strings.ToLower(outStr), "already exists") {
			return nil
		}
		return fmt.Errorf("creating postgres database %q: %w (output: %s)", database, err, outStr)
	}

	return nil
}
