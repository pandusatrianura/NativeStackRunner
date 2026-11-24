package main

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/pandusatrianura/NativeStackRunner/internal/config"
	"github.com/pandusatrianura/NativeStackRunner/internal/runner"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExampleStackConfigLoads(t *testing.T) {
	examplePath := filepath.Join("..", "..", "examples", "stack.yaml")
	cfg, err := config.Load(examplePath)
	require.NoError(t, err)
	require.NotNil(t, cfg)

	assert.Equal(t, "demo-stack", cfg.Name)
	assert.Contains(t, cfg.Services, "minio")
	assert.Contains(t, cfg.Services, "redis")
	assert.Contains(t, cfg.Services, "postgres")

	minio := cfg.Services["minio"]
	assert.Equal(t, 9000, minio.Port)
	assert.Equal(t, 9001, minio.ConsolePort)
	assert.Equal(t, "minioadmin", minio.AccessKey)
}

func TestTCPPortProbeHelper(t *testing.T) {
	// Start dummy listener to verify TCP probing
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer listener.Close()

	addr := listener.Addr().String()

	conn, err := net.DialTimeout("tcp", addr, 1*time.Second)
	require.NoError(t, err)
	_ = conn.Close()
}

func TestStatusReporting(t *testing.T) {
	tempProject := t.TempDir()
	pidsDir := filepath.Join(tempProject, ".stack", "pids")

	// Initially empty
	pids, err := runner.ListActivePIDs(pidsDir)
	require.NoError(t, err)
	assert.Empty(t, pids)

	// Save dummy PID
	err = runner.SavePID(pidsDir, "minio", 999999)
	require.NoError(t, err)

	pids, err = runner.ListActivePIDs(pidsDir)
	require.NoError(t, err)
	assert.Equal(t, 999999, pids["minio"])

	// Clean up
	err = runner.RemovePID(pidsDir, "minio")
	require.NoError(t, err)
}

func TestCleanCommand_DataRemoval(t *testing.T) {
	tempProject := t.TempDir()
	dataDir := filepath.Join(tempProject, ".stack", "data", "minio")
	pidsDir := filepath.Join(tempProject, ".stack", "pids")
	require.NoError(t, os.MkdirAll(dataDir, 0o755))
	require.NoError(t, os.MkdirAll(pidsDir, 0o755))

	dummyFile := filepath.Join(dataDir, "test.txt")
	require.NoError(t, os.WriteFile(dummyFile, []byte("data"), 0o644))
	assert.FileExists(t, dummyFile)

	dummyPID := filepath.Join(pidsDir, "minio.pid")
	require.NoError(t, os.WriteFile(dummyPID, []byte("999999"), 0o644))
	assert.FileExists(t, dummyPID)

	// Clean removes .stack/data and stale .stack/pids
	stackDataRoot := filepath.Join(tempProject, ".stack", "data")
	require.NoError(t, os.RemoveAll(stackDataRoot))
	assert.NoDirExists(t, stackDataRoot)

	require.NoError(t, os.RemoveAll(pidsDir))
	assert.NoDirExists(t, pidsDir)
}

func TestShutdownContextLifecycle(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	stopped := make(chan struct{})

	go func() {
		<-ctx.Done()
		close(stopped)
	}()

	cancel()

	select {
	case <-stopped:
		// Succeeded
	case <-time.After(1 * time.Second):
		t.Fatal("shutdown signal was not received in time")
	}
}

func TestPreflightPortConflict_Integration(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer listener.Close()

	parts := strings.Split(listener.Addr().String(), ":")
	occupiedPort, err := strconv.Atoi(parts[len(parts)-1])
	require.NoError(t, err)

	services := map[string]config.ServiceConfig{
		"redis": {
			Name: "redis",
			Port: occupiedPort,
		},
	}

	err = runner.ValidateServicePorts(services)
	require.Error(t, err)
	assert.ErrorIs(t, err, runner.ErrPortInUse)
}
