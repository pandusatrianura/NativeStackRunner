package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/pandusatrianura/NativeStackRunner/internal/runner"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExecCommand_Validation(t *testing.T) {
	tempProject := t.TempDir()
	origProjectDir := projectDir
	defer func() { projectDir = origProjectDir }()
	projectDir = tempProject

	t.Run("fails when no command specified", func(t *testing.T) {
		cmd := newExecCmd()
		cmd.SetArgs([]string{})
		err := cmd.Execute()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no command specified")
	})

	t.Run("fails when services are already running", func(t *testing.T) {
		pidsDir := filepath.Join(tempProject, ".stack", "pids")
		require.NoError(t, os.MkdirAll(pidsDir, 0o755))
		currentPID := os.Getpid()
		require.NoError(t, runner.SavePID(pidsDir, "postgres", currentPID))
		defer func() { _ = runner.RemovePID(pidsDir, "postgres") }()

		cmd := newExecCmd()
		cmd.SetArgs([]string{"--", "echo", "hello"})
		err := cmd.Execute()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "services are already running")
	})

	t.Run("fails when config is missing", func(t *testing.T) {
		cmd := newExecCmd()
		cmd.SetArgs([]string{"--", "echo", "hello"})
		err := cmd.Execute()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no stack.yaml or docker-compose.yml found")
	})
}
