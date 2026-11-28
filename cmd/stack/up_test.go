package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/pandusatrianura/NativeStackRunner/internal/runner"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUpCommand_DetachedModeFlags(t *testing.T) {
	t.Run("up command provides --detach and -d flags", func(t *testing.T) {
		cmd := newUpCmd()
		flag := cmd.Flags().Lookup("detach")
		require.NotNil(t, flag, "detach flag must be registered")
		assert.Equal(t, "d", flag.Shorthand, "detach flag shorthand must be 'd'")
		assert.Equal(t, "false", flag.DefValue, "default detach flag value must be false")
	})

	t.Run("help message documents detached mode", func(t *testing.T) {
		cmd := newUpCmd()
		var outBuf bytes.Buffer
		cmd.SetOut(&outBuf)
		cmd.SetArgs([]string{"--help"})

		err := cmd.Execute()
		require.NoError(t, err)
		assert.Contains(t, outBuf.String(), "-d, --detach")
		assert.Contains(t, outBuf.String(), "background")
	})
}

func TestUpCommand_DetachedMode_Validation(t *testing.T) {
	tempProject := t.TempDir()
	origProjectDir := projectDir
	defer func() { projectDir = origProjectDir }()
	projectDir = tempProject

	t.Run("fails gracefully when stack.yaml is missing with -d", func(t *testing.T) {
		cmd := newUpCmd()
		cmd.SetArgs([]string{"-d"})
		var outBuf bytes.Buffer
		cmd.SetOut(&outBuf)
		cmd.SetErr(&outBuf)

		err := cmd.Execute()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "configuration file not found")
	})

	t.Run("fails when active running service is present with -d", func(t *testing.T) {
		pidsDir := filepath.Join(tempProject, ".stack", "pids")
		require.NoError(t, os.MkdirAll(pidsDir, 0o755))

		currentPID := os.Getpid()
		require.NoError(t, runner.SavePID(pidsDir, "postgres", currentPID))
		defer func() { _ = runner.RemovePID(pidsDir, "postgres") }()

		cmd := newUpCmd()
		cmd.SetArgs([]string{"--detach"})
		var outBuf bytes.Buffer
		cmd.SetOut(&outBuf)
		cmd.SetErr(&outBuf)

		err := cmd.Execute()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "services are already running for this project")
		assert.Contains(t, err.Error(), "postgres")
	})
}
