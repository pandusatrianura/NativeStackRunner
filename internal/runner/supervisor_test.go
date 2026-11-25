package runner_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/pandusatrianura/NativeStackRunner/internal/binary"
	"github.com/pandusatrianura/NativeStackRunner/internal/config"
	"github.com/pandusatrianura/NativeStackRunner/internal/runner"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockProcessHandle struct {
	pid    int
	killed bool
	waitCh chan error
	mu     sync.Mutex
}

func (m *mockProcessHandle) PID() int {
	return m.pid
}

func (m *mockProcessHandle) Wait() error {
	if m.waitCh != nil {
		return <-m.waitCh
	}
	return nil
}

func (m *mockProcessHandle) Kill() error {
	m.mu.Lock()
	m.killed = true
	m.mu.Unlock()
	if m.waitCh != nil {
		select {
		case m.waitCh <- nil:
		default:
		}
	}
	return nil
}

type mockExecutor struct {
	startedCmds []runner.SpecCmd
	handles     []*mockProcessHandle
	failService string
}

func (m *mockExecutor) Start(ctx context.Context, cmd runner.SpecCmd) (runner.ProcessHandle, io.Reader, io.Reader, error) {
	if m.failService != "" && cmd.Service == m.failService {
		return nil, nil, nil, fmt.Errorf("simulated failure starting %s", cmd.Service)
	}

	m.startedCmds = append(m.startedCmds, cmd)
	handle := &mockProcessHandle{
		pid:    1000 + len(m.startedCmds),
		waitCh: make(chan error, 1),
	}
	m.handles = append(m.handles, handle)

	stdout := strings.NewReader("ready to accept connections\n")
	stderr := strings.NewReader("")
	return handle, stdout, stderr, nil
}

type mockDownloader struct{}

func (m *mockDownloader) Download(ctx context.Context, url string, destPath string) error {
	if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
		return err
	}
	return os.WriteFile(destPath, []byte("mock binary"), 0o755)
}

type mockManifest struct{}

func (m *mockManifest) Resolve(service, version, targetOS, targetArch string) (*binary.Spec, error) {
	binName := service
	if targetOS == "windows" {
		binName += ".exe"
	}
	h := sha256.Sum256([]byte("mock binary"))
	return &binary.Spec{
		Service:       service,
		Version:       version,
		Type:          binary.ArchiveRaw,
		BinarySubpath: binName,
		SHA256:        hex.EncodeToString(h[:]),
	}, nil
}

func TestPIDManagement(t *testing.T) {
	tempDir := t.TempDir()
	pidDir := filepath.Join(tempDir, "pids")

	// Save PID
	err := runner.SavePID(pidDir, "redis", 4567)
	require.NoError(t, err)

	// Read PID
	pid, err := runner.ReadPID(pidDir, "redis")
	require.NoError(t, err)
	assert.Equal(t, 4567, pid)

	// List active PIDs
	pidsMap, err := runner.ListActivePIDs(pidDir)
	require.NoError(t, err)
	assert.Equal(t, 4567, pidsMap["redis"])

	// Remove PID
	err = runner.RemovePID(pidDir, "redis")
	require.NoError(t, err)

	// Read after remove returns error
	_, err = runner.ReadPID(pidDir, "redis")
	require.ErrorIs(t, err, runner.ErrPIDNotFound)
}

func TestBuildServiceCmd(t *testing.T) {
	tempProject := t.TempDir()

	// MinIO command
	minioCfg := config.ServiceConfig{
		Name:        "minio",
		Port:        9000,
		ConsolePort: 9001,
		AccessKey:   "admin",
		SecretKey:   "secret123",
	}
	cmd, err := runner.BuildServiceCmd(context.Background(), minioCfg, "/path/to/minio", tempProject)
	require.NoError(t, err)
	assert.Equal(t, "minio", cmd.Service)
	assert.Equal(t, "/path/to/minio", cmd.BinPath)
	assert.Contains(t, cmd.Args, "server")
	assert.Contains(t, cmd.Args, ":9000")
	assert.Contains(t, cmd.Args, ":9001")
	assert.Contains(t, cmd.Env, "MINIO_ROOT_USER=admin")
	assert.Contains(t, cmd.Env, "MINIO_ROOT_PASSWORD=secret123")

	// Redis command
	redisCfg := config.ServiceConfig{
		Name: "redis",
		Port: 6379,
	}
	redisCmd, err := runner.BuildServiceCmd(context.Background(), redisCfg, "/path/to/redis-server", tempProject)
	require.NoError(t, err)
	assert.Equal(t, "redis", redisCmd.Service)
	assert.Contains(t, redisCmd.Args, "--port")
	assert.Contains(t, redisCmd.Args, "6379")

	// Postgres command (with pre-existing PG_VERSION to avoid invoking initdb)
	pgDataDir := filepath.Join(tempProject, ".stack", "data", "postgres")
	require.NoError(t, os.MkdirAll(pgDataDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(pgDataDir, "PG_VERSION"), []byte("16\n"), 0o644))

	pgCfg := config.ServiceConfig{
		Name:    "postgres",
		Port:    5432,
		Version: "16.1.0",
		User:    "postgres",
	}
	pgCmd, err := runner.BuildServiceCmd(context.Background(), pgCfg, filepath.Join(tempProject, "bin", "postgres"), tempProject)
	require.NoError(t, err)
	assert.Equal(t, "postgres", pgCmd.Service)
	assert.Contains(t, pgCmd.Args, "-p")
	assert.Contains(t, pgCmd.Args, "5432")
	var hasLD, hasDYLD bool
	for _, env := range pgCmd.Env {
		if strings.HasPrefix(env, "LD_LIBRARY_PATH=") {
			hasLD = true
		}
		if strings.HasPrefix(env, "DYLD_LIBRARY_PATH=") {
			hasDYLD = true
		}
	}
	assert.True(t, hasLD, "expected LD_LIBRARY_PATH in env")
	assert.True(t, hasDYLD, "expected DYLD_LIBRARY_PATH in env")
}

func TestSupervisor_StartAndCancel(t *testing.T) {
	tempProject := t.TempDir()
	tempCache := t.TempDir()

	binMgr, err := binary.NewManager(tempCache, &mockDownloader{})
	require.NoError(t, err)
	binMgr.SetManifest(&mockManifest{})

	cfg := &config.StackConfig{
		Name: "test-stack",
		Services: map[string]config.ServiceConfig{
			"minio": {
				Name: "minio",
				Port: 9000,
			},
		},
	}

	exec := &mockExecutor{}
	var outBuf bytes.Buffer
	supervisor := runner.NewSupervisor(cfg, tempProject, binMgr, exec, &outBuf)

	ctx, cancel := context.WithCancel(context.Background())

	errCh := make(chan error, 1)
	go func() {
		errCh <- supervisor.Start(ctx)
	}()

	time.Sleep(100 * time.Millisecond)
	cancel()

	select {
	case err := <-errCh:
		require.NoError(t, err)
	case <-time.After(3 * time.Second):
		t.Fatal("supervisor did not stop in time after cancellation")
	}

	assert.Len(t, exec.startedCmds, 1)
	assert.True(t, exec.handles[0].killed)

	output := outBuf.String()
	assert.Contains(t, output, "[minio | 9000]")
	assert.Contains(t, output, "ready to accept connections")
	assert.Contains(t, output, "Shutting down services gracefully")
}

func TestSupervisor_ChildCrashDetection(t *testing.T) {
	tempProject := t.TempDir()
	tempCache := t.TempDir()

	binMgr, err := binary.NewManager(tempCache, &mockDownloader{})
	require.NoError(t, err)
	binMgr.SetManifest(&mockManifest{})

	cfg := &config.StackConfig{
		Name: "test-stack",
		Services: map[string]config.ServiceConfig{
			"minio": {
				Name: "minio",
				Port: 9000,
			},
		},
	}

	exec := &mockExecutor{}
	var outBuf bytes.Buffer
	supervisor := runner.NewSupervisor(cfg, tempProject, binMgr, exec, &outBuf)

	ctx := context.Background()
	errCh := make(chan error, 1)
	go func() {
		errCh <- supervisor.Start(ctx)
	}()

	time.Sleep(100 * time.Millisecond)
	require.Len(t, exec.handles, 1)

	// Simulate unexpected crash of child process
	exec.handles[0].waitCh <- errors.New("signal: killed (fatal port collision)")

	select {
	case err := <-errCh:
		require.Error(t, err)
		assert.Contains(t, err.Error(), "terminated unexpectedly")
		assert.Contains(t, outBuf.String(), "[ERROR]")
	case <-time.After(3 * time.Second):
		t.Fatal("supervisor did not catch child process crash")
	}

	assert.True(t, exec.handles[0].killed)
}

func TestSupervisor_StartupRollback(t *testing.T) {
	tempProject := t.TempDir()
	tempCache := t.TempDir()

	binMgr, err := binary.NewManager(tempCache, &mockDownloader{})
	require.NoError(t, err)
	binMgr.SetManifest(&mockManifest{})

	// Two services configured
	cfg := &config.StackConfig{
		Name: "test-stack",
		Services: map[string]config.ServiceConfig{
			"minio": {
				Name: "minio",
				Port: 9000,
			},
			"redis": {
				Name: "redis",
				Port: 6379,
			},
		},
	}

	// Fail the second service
	exec := &mockExecutor{failService: "redis"}
	var outBuf bytes.Buffer
	supervisor := runner.NewSupervisor(cfg, tempProject, binMgr, exec, &outBuf)

	ctx := context.Background()
	err = supervisor.Start(ctx)
	require.Error(t, err)

	// Verify that minio was killed and its PID file cleaned up
	pidsDir := filepath.Join(tempProject, ".stack", "pids")
	activePIDs, _ := runner.ListActivePIDs(pidsDir)
	assert.Empty(t, activePIDs, "all PID files should be cleaned up after rollback")
}

func TestSupervisor_PreflightPortConflict(t *testing.T) {
	tempProject := t.TempDir()
	tempCache := t.TempDir()

	binMgr, err := binary.NewManager(tempCache, &mockDownloader{})
	require.NoError(t, err)
	binMgr.SetManifest(&mockManifest{})

	// Occupy a port
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer listener.Close()

	parts := strings.Split(listener.Addr().String(), ":")
	occupiedPort, err := strconv.Atoi(parts[len(parts)-1])
	require.NoError(t, err)

	cfg := &config.StackConfig{
		Name: "test-stack",
		Services: map[string]config.ServiceConfig{
			"redis": {
				Name: "redis",
				Port: occupiedPort,
			},
		},
	}

	exec := &mockExecutor{}
	var outBuf bytes.Buffer
	supervisor := runner.NewSupervisor(cfg, tempProject, binMgr, exec, &outBuf)

	err = supervisor.Start(context.Background())
	require.Error(t, err)
	assert.ErrorIs(t, err, runner.ErrPortInUse)
	assert.Empty(t, exec.startedCmds, "no processes should be launched when pre-flight fails")
}

func TestSupervisor_StartAsync_Lifecycle(t *testing.T) {
	tempProject := t.TempDir()
	tempCache := t.TempDir()

	binMgr, err := binary.NewManager(tempCache, &mockDownloader{})
	require.NoError(t, err)
	binMgr.SetManifest(&mockManifest{})

	// Spin up a mock listener to simulate the service port becoming ready
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer listener.Close()

	parts := strings.Split(listener.Addr().String(), ":")
	testPort, err := strconv.Atoi(parts[len(parts)-1])
	require.NoError(t, err)

	// Free the port before preflight check, and re-open it asynchronously to simulate service boot
	_ = listener.Close()

	cfg := &config.StackConfig{
		Name: "test-stack",
		Services: map[string]config.ServiceConfig{
			"redis": {
				Name: "redis",
				Port: testPort,
			},
		},
	}

	exec := &mockExecutor{}
	var outBuf bytes.Buffer
	supervisor := runner.NewSupervisor(cfg, tempProject, binMgr, exec, &outBuf)

	ctx := context.Background()
	stack, err := supervisor.StartAsync(ctx)
	require.NoError(t, err)
	require.NotNil(t, stack)

	// Simulate service becoming ready
	go func() {
		time.Sleep(50 * time.Millisecond)
		l, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", testPort))
		if err == nil {
			defer l.Close()
			time.Sleep(300 * time.Millisecond)
		}
	}()

	err = stack.WaitReady(ctx, 1*time.Second)
	require.NoError(t, err)

	pidsDir := filepath.Join(tempProject, ".stack", "pids")
	activePIDs, err := runner.ListActivePIDs(pidsDir)
	require.NoError(t, err)
	assert.Contains(t, activePIDs, "redis")

	// Call Stop and verify PIDs are cleaned up
	stack.Stop()
	activePIDsAfter, _ := runner.ListActivePIDs(pidsDir)
	assert.Empty(t, activePIDsAfter)
}
