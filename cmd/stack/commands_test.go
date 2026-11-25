package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/pandusatrianura/NativeStackRunner/internal/runner"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStatusCommand_Output(t *testing.T) {
	tempProject := t.TempDir()
	origProjectDir := projectDir
	defer func() { projectDir = origProjectDir }()
	projectDir = tempProject

	t.Run("empty pids outputs no active services", func(t *testing.T) {
		cmd := newStatusCmd()
		var outBuf bytes.Buffer
		cmd.SetOut(&outBuf)
		cmd.SetErr(&outBuf)

		err := cmd.Execute()
		require.NoError(t, err)
		assert.Contains(t, outBuf.String(), "No active services")
	})

	t.Run("displays orphaned pid when process is dead", func(t *testing.T) {
		pidsDir := filepath.Join(tempProject, ".stack", "pids")
		require.NoError(t, os.MkdirAll(pidsDir, 0o755))
		require.NoError(t, runner.SavePID(pidsDir, "minio", 999999))
		defer func() { _ = runner.RemovePID(pidsDir, "minio") }()

		cmd := newStatusCmd()
		var outBuf bytes.Buffer
		cmd.SetOut(&outBuf)
		cmd.SetErr(&outBuf)

		err := cmd.Execute()
		require.NoError(t, err)
		assert.Contains(t, outBuf.String(), "minio")
		assert.Contains(t, outBuf.String(), "999999")
		assert.Contains(t, outBuf.String(), "STOPPED (orphaned PID)")
	})

	t.Run("displays running when process is alive", func(t *testing.T) {
		pidsDir := filepath.Join(tempProject, ".stack", "pids")
		require.NoError(t, os.MkdirAll(pidsDir, 0o755))
		currentPID := os.Getpid()
		require.NoError(t, runner.SavePID(pidsDir, "redis", currentPID))
		defer func() { _ = runner.RemovePID(pidsDir, "redis") }()

		cmd := newStatusCmd()
		var outBuf bytes.Buffer
		cmd.SetOut(&outBuf)
		cmd.SetErr(&outBuf)

		err := cmd.Execute()
		require.NoError(t, err)
		assert.Contains(t, outBuf.String(), "redis")
		assert.Contains(t, outBuf.String(), strconv.Itoa(currentPID))
		assert.Contains(t, outBuf.String(), "RUNNING")
	})
}

func TestDownCommand_Execution(t *testing.T) {
	tempProject := t.TempDir()
	origProjectDir := projectDir
	defer func() { projectDir = origProjectDir }()
	projectDir = tempProject

	t.Run("no active services outputs message", func(t *testing.T) {
		cmd := newDownCmd()
		var outBuf bytes.Buffer
		cmd.SetOut(&outBuf)
		cmd.SetErr(&outBuf)

		err := cmd.Execute()
		require.NoError(t, err)
		assert.Contains(t, outBuf.String(), "No active services found for this project.")
	})

	t.Run("cleans dead pid and reports done", func(t *testing.T) {
		pidsDir := filepath.Join(tempProject, ".stack", "pids")
		require.NoError(t, os.MkdirAll(pidsDir, 0o755))
		require.NoError(t, runner.SavePID(pidsDir, "postgres", 999999))

		cmd := newDownCmd()
		var outBuf bytes.Buffer
		cmd.SetOut(&outBuf)
		cmd.SetErr(&outBuf)

		err := cmd.Execute()
		require.NoError(t, err)
		assert.Contains(t, outBuf.String(), "Stopping services...")
		assert.Contains(t, outBuf.String(), "Done.")

		// Verify PID file was removed
		pids, err := runner.ListActivePIDs(pidsDir)
		require.NoError(t, err)
		assert.NotContains(t, pids, "postgres")
	})
}

func TestCleanCommand_InteractiveAndForce(t *testing.T) {
	tempProject := t.TempDir()
	origProjectDir := projectDir
	defer func() { projectDir = origProjectDir }()
	projectDir = tempProject

	t.Run("nothing to clean outputs message", func(t *testing.T) {
		cmd := newCleanCmd()
		var outBuf bytes.Buffer
		cmd.SetOut(&outBuf)
		cmd.SetErr(&outBuf)

		err := cmd.Execute()
		require.NoError(t, err)
		assert.Contains(t, outBuf.String(), "No project data or runtime directory found to clean.")
	})

	t.Run("interactive prompt canceled on n", func(t *testing.T) {
		dataDir := filepath.Join(tempProject, ".stack", "data", "redis")
		require.NoError(t, os.MkdirAll(dataDir, 0o755))

		cmd := newCleanCmd()
		var outBuf bytes.Buffer
		inBuf := bytes.NewBufferString("n\n")
		cmd.SetOut(&outBuf)
		cmd.SetIn(inBuf)

		err := cmd.Execute()
		require.NoError(t, err)
		assert.Contains(t, outBuf.String(), "Clean operation canceled.")
		assert.DirExists(t, dataDir)
	})

	t.Run("interactive prompt succeeds on y", func(t *testing.T) {
		dataDir := filepath.Join(tempProject, ".stack", "data", "redis")
		require.NoError(t, os.MkdirAll(dataDir, 0o755))

		cmd := newCleanCmd()
		var outBuf bytes.Buffer
		inBuf := bytes.NewBufferString("y\n")
		cmd.SetOut(&outBuf)
		cmd.SetIn(inBuf)

		err := cmd.Execute()
		require.NoError(t, err)
		assert.Contains(t, outBuf.String(), "Cleaned local project data and runtime state.")
		assert.NoDirExists(t, dataDir)
	})

	t.Run("force flag deletes without prompt", func(t *testing.T) {
		dataDir := filepath.Join(tempProject, ".stack", "data", "redis")
		require.NoError(t, os.MkdirAll(dataDir, 0o755))

		cmd := newCleanCmd()
		cmd.SetArgs([]string{"--force"})
		var outBuf bytes.Buffer
		cmd.SetOut(&outBuf)

		err := cmd.Execute()
		require.NoError(t, err)
		assert.Contains(t, outBuf.String(), "Cleaned local project data and runtime state.")
		assert.NoDirExists(t, dataDir)
	})

	t.Run("fails when active running service detected", func(t *testing.T) {
		dataDir := filepath.Join(tempProject, ".stack", "data", "redis")
		pidsDir := filepath.Join(tempProject, ".stack", "pids")
		require.NoError(t, os.MkdirAll(dataDir, 0o755))
		require.NoError(t, os.MkdirAll(pidsDir, 0o755))

		currentPID := os.Getpid()
		require.NoError(t, runner.SavePID(pidsDir, "redis", currentPID))
		defer func() { _ = runner.RemovePID(pidsDir, "redis") }()

		cmd := newCleanCmd()
		cmd.SetArgs([]string{"--force"})
		var outBuf bytes.Buffer
		cmd.SetOut(&outBuf)
		cmd.SetErr(&outBuf)

		err := cmd.Execute()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cannot clean: service \"redis\"")
		assert.Contains(t, err.Error(), "is still running")
	})
}

func TestUpCommand_ValidationErrors(t *testing.T) {
	tempProject := t.TempDir()
	origProjectDir := projectDir
	defer func() { projectDir = origProjectDir }()
	projectDir = tempProject

	t.Run("missing configuration returns error", func(t *testing.T) {
		cmd := newUpCmd()
		var outBuf bytes.Buffer
		cmd.SetOut(&outBuf)
		cmd.SetErr(&outBuf)

		err := cmd.Execute()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "configuration file not found")
	})

	t.Run("active running service prevents startup", func(t *testing.T) {
		pidsDir := filepath.Join(tempProject, ".stack", "pids")
		require.NoError(t, os.MkdirAll(pidsDir, 0o755))

		currentPID := os.Getpid()
		require.NoError(t, runner.SavePID(pidsDir, "minio", currentPID))
		defer func() { _ = runner.RemovePID(pidsDir, "minio") }()

		cmd := newUpCmd()
		var outBuf bytes.Buffer
		cmd.SetOut(&outBuf)
		cmd.SetErr(&outBuf)

		err := cmd.Execute()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "services are already running for this project")
		assert.Contains(t, err.Error(), "minio")
	})

	t.Run("detach flag accepted by up command", func(t *testing.T) {
		cmd := newUpCmd()
		cmd.SetArgs([]string{"--detach"})
		var outBuf bytes.Buffer
		cmd.SetOut(&outBuf)
		cmd.SetErr(&outBuf)

		err := cmd.Execute()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "configuration file not found")
	})
}

func TestVersionCommand_Output(t *testing.T) {
	t.Run("version sub-command prints metadata", func(t *testing.T) {
		cmd := newVersionCmd()
		var outBuf bytes.Buffer
		cmd.SetOut(&outBuf)
		cmd.SetErr(&outBuf)

		err := cmd.Execute()
		require.NoError(t, err)
		out := outBuf.String()
		assert.Contains(t, out, "Native Stack Runner (stack) version")
		assert.Contains(t, out, "Commit:")
		assert.Contains(t, out, "OS/Arch:")
	})

	t.Run("root command accepts --version flag", func(t *testing.T) {
		var outBuf bytes.Buffer
		rootCmd.SetOut(&outBuf)
		rootCmd.SetArgs([]string{"--version"})

		err := rootCmd.Execute()
		require.NoError(t, err)
		assert.Contains(t, outBuf.String(), "stack version")
		rootCmd.SetArgs(nil)
	})
}

func TestInitCommand_Scaffolding(t *testing.T) {
	tempProject := t.TempDir()
	origProjectDir := projectDir
	defer func() { projectDir = origProjectDir }()
	projectDir = tempProject

	t.Run("creates stack.yaml in empty directory", func(t *testing.T) {
		cmd := newInitCmd()
		var outBuf bytes.Buffer
		cmd.SetOut(&outBuf)
		cmd.SetErr(&outBuf)

		err := cmd.Execute()
		require.NoError(t, err)
		assert.Contains(t, outBuf.String(), "Initialized")
		assert.FileExists(t, filepath.Join(tempProject, "stack.yaml"))
	})

	t.Run("refuses to overwrite existing stack.yaml without force", func(t *testing.T) {
		cmd := newInitCmd()
		var outBuf bytes.Buffer
		cmd.SetOut(&outBuf)
		cmd.SetErr(&outBuf)

		err := cmd.Execute()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "already exists")
	})

	t.Run("overwrites existing stack.yaml with force flag", func(t *testing.T) {
		cmd := newInitCmd()
		cmd.SetArgs([]string{"--force"})
		var outBuf bytes.Buffer
		cmd.SetOut(&outBuf)
		cmd.SetErr(&outBuf)

		err := cmd.Execute()
		require.NoError(t, err)
		assert.Contains(t, outBuf.String(), "Initialized")
	})
}

func TestEnvCommand_Output(t *testing.T) {
	tempProject := t.TempDir()
	origProjectDir := projectDir
	defer func() { projectDir = origProjectDir }()
	projectDir = tempProject

	t.Run("fails when configuration is missing", func(t *testing.T) {
		cmd := newEnvCmd()
		var outBuf bytes.Buffer
		cmd.SetOut(&outBuf)
		cmd.SetErr(&outBuf)

		err := cmd.Execute()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "configuration file not found")
	})

	// Create test configuration
	configContent := `
version: "1"
name: test-env
services:
  postgres:
    port: 5432
    user: myuser
    password: mypassword
    database: mydb
  redis:
    port: 6379
  minio:
    port: 9000
    access_key: admin
    secret_key: secret
`
	require.NoError(t, os.WriteFile(filepath.Join(tempProject, "stack.yaml"), []byte(configContent), 0o644))

	t.Run("prints formatted environment variables by default", func(t *testing.T) {
		cmd := newEnvCmd()
		var outBuf bytes.Buffer
		cmd.SetOut(&outBuf)
		cmd.SetErr(&outBuf)

		err := cmd.Execute()
		require.NoError(t, err)
		out := outBuf.String()
		assert.Contains(t, out, "DATABASE_URL=\"postgres://myuser:mypassword@localhost:5432/mydb?sslmode=disable\"")
		assert.Contains(t, out, "REDIS_URL=\"redis://localhost:6379\"")
		assert.Contains(t, out, "S3_ENDPOINT=\"http://localhost:9000\"")
	})

	t.Run("formats with export prefix when flag provided", func(t *testing.T) {
		cmd := newEnvCmd()
		cmd.SetArgs([]string{"--export"})
		var outBuf bytes.Buffer
		cmd.SetOut(&outBuf)
		cmd.SetErr(&outBuf)

		err := cmd.Execute()
		require.NoError(t, err)
		out := outBuf.String()
		assert.Contains(t, out, "export DATABASE_URL=")
		assert.Contains(t, out, "export REDIS_URL=")
		assert.Contains(t, out, "export S3_ENDPOINT=")
	})

	t.Run("formats as JSON map when flag provided", func(t *testing.T) {
		cmd := newEnvCmd()
		cmd.SetArgs([]string{"--json"})
		var outBuf bytes.Buffer
		cmd.SetOut(&outBuf)
		cmd.SetErr(&outBuf)

		err := cmd.Execute()
		require.NoError(t, err)

		var res map[string]string
		err = json.Unmarshal(outBuf.Bytes(), &res)
		require.NoError(t, err)
		assert.Equal(t, "postgres://myuser:mypassword@localhost:5432/mydb?sslmode=disable", res["DATABASE_URL"])
		assert.Equal(t, "redis://localhost:6379", res["REDIS_URL"])
		assert.Equal(t, "http://localhost:9000", res["S3_ENDPOINT"])
		assert.Equal(t, "admin", res["AWS_ACCESS_KEY_ID"])
	})
}
