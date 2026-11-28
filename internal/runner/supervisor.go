package runner

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/fatih/color"
	"github.com/pandusatrianura/NativeStackRunner/internal/binary"
	"github.com/pandusatrianura/NativeStackRunner/internal/config"
)

// ServiceStatus provides the operational status of an individual dependency.
type ServiceStatus struct {
	Name    string
	Port    int
	PID     int
	Running bool
}

type runningHandle struct {
	service string
	handle  ProcessHandle
}

type processExit struct {
	service string
	err     error
}

// Supervisor orchestrates the lifecycle, error monitoring, and logging of local dependency processes.
type Supervisor struct {
	cfg          *config.StackConfig
	projectDir   string
	binaryMgr    *binary.Manager
	executor     ProcessExecutor
	outputWriter io.Writer
	outMu        sync.Mutex
	cmdRunner    CmdRunner
	httpClient   HTTPClient
}

// NewSupervisor instantiates a Supervisor for the given stack configuration and project directory.
func NewSupervisor(cfg *config.StackConfig, projectDir string, binMgr *binary.Manager, exec ProcessExecutor, out io.Writer) *Supervisor {
	if exec == nil {
		exec = &OSProcessExecutor{}
	}
	if out == nil {
		out = os.Stdout
	}
	return &Supervisor{
		cfg:          cfg,
		projectDir:   projectDir,
		binaryMgr:    binMgr,
		executor:     exec,
		outputWriter: out,
		cmdRunner:    DefaultCmdRunner,
		httpClient:   &http.Client{Timeout: 5 * time.Second},
	}
}

// SetCmdRunner sets the command runner for post-startup provisioning helpers.
func (s *Supervisor) SetCmdRunner(r CmdRunner) {
	s.cmdRunner = r
}

// SetHTTPClient sets the HTTP client for S3/API provisioning helpers.
func (s *Supervisor) SetHTTPClient(c HTTPClient) {
	s.httpClient = c
}

func (s *Supervisor) writeOutput(format string, a ...interface{}) {
	s.outMu.Lock()
	defer s.outMu.Unlock()
	_, _ = fmt.Fprintf(s.outputWriter, format, a...)
}

func (s *Supervisor) writeOutputLine(a ...interface{}) {
	s.outMu.Lock()
	defer s.outMu.Unlock()
	_, _ = fmt.Fprintln(s.outputWriter, a...)
}

// RunningStack represents an actively managed set of background processes in a stack.
type RunningStack struct {
	supervisor *Supervisor
	handles    []runningHandle
	pidsDir    string
	binPaths   map[string]string
	wg         sync.WaitGroup
	ctx        context.Context
	cancel     context.CancelFunc
	exitCh     chan processExit
}

// Stop gracefully terminates all running processes in the stack and cleans up PID files.
func (rs *RunningStack) Stop() {
	rs.cancel()
	for _, h := range rs.handles {
		_ = h.handle.Kill()
		_ = RemovePID(rs.pidsDir, h.service)
	}
	rs.supervisor.drainLogs(&rs.wg, 500*time.Millisecond)
}

// WaitReady blocks until all service ports accept TCP connections and post-start provisioning is complete.
func (rs *RunningStack) WaitReady(ctx context.Context, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for name, svc := range rs.supervisor.cfg.Services {
		targetAddr := fmt.Sprintf("127.0.0.1:%d", svc.Port)
		remTime := time.Until(deadline)
		if remTime <= 0 {
			return fmt.Errorf("timeout waiting for %s on %s", name, targetAddr)
		}
		if err := WaitForTCPReady(ctx, targetAddr, remTime); err != nil {
			return fmt.Errorf("waiting for %s on %s: %w", name, targetAddr, err)
		}
	}
	// Run post-startup provisioning synchronously to guarantee custom database and buckets exist
	rs.supervisor.postStartProvision(ctx, rs.binPaths)
	return nil
}

// ExitChan returns a channel signaling unexpected process exits.
func (rs *RunningStack) ExitChan() <-chan processExit {
	return rs.exitCh
}

// StartAsync launches all services in the background and returns a RunningStack controller.
func (s *Supervisor) StartAsync(parentCtx context.Context) (*RunningStack, error) {
	// 0. Pre-flight check: ensure all declared service ports are available
	if err := ValidateServicePorts(s.cfg.Services); err != nil {
		return nil, fmt.Errorf("pre-flight port check: %w", err)
	}

	pidsDir := filepath.Join(s.projectDir, ".stack", "pids")
	logsDir := filepath.Join(s.projectDir, ".stack", "logs")

	if err := os.MkdirAll(pidsDir, 0o755); err != nil {
		return nil, fmt.Errorf("creating pids directory: %w", err)
	}
	if err := os.MkdirAll(logsDir, 0o755); err != nil {
		return nil, fmt.Errorf("creating logs directory: %w", err)
	}

	colors := []*color.Color{
		color.New(color.FgCyan, color.Bold),
		color.New(color.FgGreen, color.Bold),
		color.New(color.FgMagenta, color.Bold),
		color.New(color.FgYellow, color.Bold),
		color.New(color.FgBlue, color.Bold),
	}

	var handles []runningHandle
	rollback := func() {
		for _, h := range handles {
			_ = h.handle.Kill()
			_ = RemovePID(pidsDir, h.service)
		}
	}

	ctx, cancel := context.WithCancel(parentCtx)
	rs := &RunningStack{
		supervisor: s,
		pidsDir:    pidsDir,
		binPaths:   make(map[string]string),
		ctx:        ctx,
		cancel:     cancel,
	}

	colorIdx := 0

	for name, svc := range s.cfg.Services {
		// 1. Ensure binary exists
		binPath, err := s.binaryMgr.EnsureBinary(ctx, svc.Name, svc.Version)
		if err != nil {
			rollback()
			cancel()
			return nil, fmt.Errorf("ensuring binary for %s: %w", name, err)
		}
		rs.binPaths[name] = binPath

		// 2. Build command spec
		cmdSpec, err := BuildServiceCmd(ctx, svc, binPath, s.projectDir)
		if err != nil {
			rollback()
			cancel()
			return nil, fmt.Errorf("building command for %s: %w", name, err)
		}

		// 3. Start process
		handle, stdout, stderr, err := s.executor.Start(ctx, cmdSpec)
		if err != nil {
			rollback()
			cancel()
			return nil, fmt.Errorf("starting process for %s: %w", name, err)
		}

		handles = append(handles, runningHandle{service: name, handle: handle})

		// 4. Save PID
		if err := SavePID(pidsDir, name, handle.PID()); err != nil {
			s.writeOutput("Warning: failed to save PID for %s: %v\n", name, err)
		}

		// 5. Pipe logs
		serviceColor := colors[colorIdx%len(colors)]
		colorIdx++
		prefix := serviceColor.Sprintf("[%s | %d] ", name, svc.Port)

		rs.wg.Add(2)
		go s.pipeLog(stdout, prefix, &rs.wg)
		go s.pipeLog(stderr, prefix, &rs.wg)
	}

	rs.handles = handles
	rs.exitCh = make(chan processExit, len(handles))

	// Monitor child processes for unexpected exit/crash
	for _, h := range handles {
		hCopy := h
		go func() {
			waitErr := hCopy.handle.Wait()
			// Only report if parent context has not initiated a planned shutdown
			if rs.ctx.Err() == nil {
				rs.exitCh <- processExit{service: hCopy.service, err: waitErr}
			}
		}()
	}

	return rs, nil
}

// DetachedServiceInfo holds information about a service running in the background.
type DetachedServiceInfo struct {
	Service string
	PID     int
	Port    int
	LogPath string
}

// StartDetached launches services in the background, redirects output to log files,
// verifies readiness via TCP, provisions custom DB/buckets, and returns the list of started services.
func (s *Supervisor) StartDetached(ctx context.Context) ([]DetachedServiceInfo, error) {
	// 0. Pre-flight check: ensure all declared service ports are available
	if err := ValidateServicePorts(s.cfg.Services); err != nil {
		return nil, fmt.Errorf("pre-flight port check: %w", err)
	}

	pidsDir := filepath.Join(s.projectDir, ".stack", "pids")
	logsDir := filepath.Join(s.projectDir, ".stack", "logs")

	if err := os.MkdirAll(pidsDir, 0o755); err != nil {
		return nil, fmt.Errorf("creating pids directory: %w", err)
	}
	if err := os.MkdirAll(logsDir, 0o755); err != nil {
		return nil, fmt.Errorf("creating logs directory: %w", err)
	}

	var handles []runningHandle
	rollback := func() {
		for _, h := range handles {
			_ = h.handle.Kill()
			_ = RemovePID(pidsDir, h.service)
		}
	}

	binPaths := make(map[string]string)
	var startedServices []DetachedServiceInfo

	for name, svc := range s.cfg.Services {
		// 1. Ensure binary exists
		binPath, err := s.binaryMgr.EnsureBinary(ctx, svc.Name, svc.Version)
		if err != nil {
			rollback()
			return nil, fmt.Errorf("ensuring binary for %s: %w", name, err)
		}
		binPaths[name] = binPath

		// 2. Build command spec
		cmdSpec, err := BuildServiceCmd(ctx, svc, binPath, s.projectDir)
		if err != nil {
			rollback()
			return nil, fmt.Errorf("building command for %s: %w", name, err)
		}

		// 3. Open log file
		logFilePath := filepath.Join(logsDir, fmt.Sprintf("%s.log", name))
		logFile, err := os.OpenFile(logFilePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
		if err != nil {
			rollback()
			return nil, fmt.Errorf("opening log file for %s: %w", name, err)
		}

		// 4. Start detached process
		handle, err := s.executor.StartDetached(cmdSpec, logFile)
		_ = logFile.Close()
		if err != nil {
			rollback()
			return nil, fmt.Errorf("starting detached process for %s: %w", name, err)
		}

		handles = append(handles, runningHandle{service: name, handle: handle})

		// 5. Save PID
		if err := SavePID(pidsDir, name, handle.PID()); err != nil {
			s.writeOutput("Warning: failed to save PID for %s: %v\n", name, err)
		}

		startedServices = append(startedServices, DetachedServiceInfo{
			Service: name,
			PID:     handle.PID(),
			Port:    svc.Port,
			LogPath: logFilePath,
		})
	}

	// 6. Wait for all services to become ready on their TCP ports
	for _, info := range startedServices {
		targetAddr := fmt.Sprintf("127.0.0.1:%d", info.Port)
		if err := WaitForTCPReady(ctx, targetAddr, 30*time.Second); err != nil {
			rollback()
			return nil, fmt.Errorf("service %s (PID %d) did not become ready on port %d: %w", info.Service, info.PID, info.Port, err)
		}
	}

	// 7. Run post-startup provisioning synchronously (custom database & buckets)
	s.postStartProvision(ctx, binPaths)

	return startedServices, nil
}

// Start boots all services in the stack, multiplexes logs, monitors child health, and terminates on ctx cancellation or crash.
func (s *Supervisor) Start(ctx context.Context) error {
	stack, err := s.StartAsync(ctx)
	if err != nil {
		return err
	}
	defer stack.Stop()

	s.writeOutput("\nAll services launched successfully in project %s.\nPress Ctrl+C to stop.\n\n", s.projectDir)

	// Post-startup provisioning (e.g. creating custom databases or buckets)
	go s.postStartProvision(ctx, stack.binPaths)

	select {
	case <-ctx.Done():
		s.writeOutputLine("\nShutting down services gracefully...")
		return nil

	case exit := <-stack.exitCh:
		errMsg := fmt.Sprintf("\n[ERROR] Service %q terminated unexpectedly (exit: %v). Shutting down all services...", exit.service, exit.err)
		s.writeOutputLine(errMsg)
		return fmt.Errorf("service %q terminated unexpectedly: %w", exit.service, exit.err)
	}
}

// postStartProvision handles automated post-startup configuration such as custom database or bucket creation.
func (s *Supervisor) postStartProvision(ctx context.Context, binPaths map[string]string) {
	for name, svc := range s.cfg.Services {
		if name == "postgres" && svc.Database != "" && svc.Database != "postgres" {
			targetAddr := fmt.Sprintf("127.0.0.1:%d", svc.Port)
			if err := WaitForTCPReady(ctx, targetAddr, 10*time.Second); err != nil {
				s.writeOutput("[postgres | %d] Warning: timeout waiting for database connection: %v\n", svc.Port, err)
				continue
			}
			binPath := binPaths[name]
			if err := EnsurePostgresDatabase(ctx, s.cmdRunner, binPath, svc.Port, svc.User, svc.Database, s.projectDir); err != nil {
				s.writeOutput("[postgres | %d] Warning: auto-creating database %q: %v\n", svc.Port, svc.Database, err)
			} else {
				s.writeOutput("[postgres | %d] Database %q verified/created.\n", svc.Port, svc.Database)
			}
		}

		if name == "minio" && len(svc.Buckets) > 0 {
			targetAddr := fmt.Sprintf("127.0.0.1:%d", svc.Port)
			if err := WaitForTCPReady(ctx, targetAddr, 10*time.Second); err != nil {
				s.writeOutput("[minio | %d] Warning: timeout waiting for MinIO connection: %v\n", svc.Port, err)
				continue
			}
			endpoint := fmt.Sprintf("http://127.0.0.1:%d", svc.Port)
			user := svc.AccessKey
			if user == "" {
				user = "minioadmin"
			}
			secret := svc.SecretKey
			if secret == "" {
				secret = "minioadmin"
			}
			for _, b := range svc.Buckets {
				if err := EnsureMinIOBucket(ctx, s.httpClient, endpoint, user, secret, b); err != nil {
					s.writeOutput("[minio | %d] Warning: auto-creating bucket %q: %v\n", svc.Port, b, err)
				} else {
					s.writeOutput("[minio | %d] S3 Bucket %q verified/created.\n", svc.Port, b)
				}
			}
		}
	}
}

// drainLogs waits up to timeout for all logging goroutines to finish flushing output.
func (s *Supervisor) drainLogs(wg *sync.WaitGroup, timeout time.Duration) {
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(timeout):
	}
}

// Stop gracefully terminates all running services belonging to this project.
func (s *Supervisor) Stop() error {
	pidsDir := filepath.Join(s.projectDir, ".stack", "pids")
	activePIDs, err := ListActivePIDs(pidsDir)
	if err != nil {
		return fmt.Errorf("listing active pids: %w", err)
	}

	for service, pid := range activePIDs {
		if IsProcessAlive(pid) {
			_ = StopProcess(pid)
		}
		_ = RemovePID(pidsDir, service)
	}
	return nil
}

// Status inspects each configured service and determines whether it is currently running.
func (s *Supervisor) Status() ([]ServiceStatus, error) {
	pidsDir := filepath.Join(s.projectDir, ".stack", "pids")
	statuses := make([]ServiceStatus, 0, len(s.cfg.Services))

	for name, svc := range s.cfg.Services {
		stat := ServiceStatus{
			Name: name,
			Port: svc.Port,
		}

		pid, err := ReadPID(pidsDir, name)
		if err == nil && pid > 0 {
			stat.PID = pid
			stat.Running = IsProcessAlive(pid)
		}

		statuses = append(statuses, stat)
	}

	return statuses, nil
}

func (s *Supervisor) pipeLog(r io.Reader, prefix string, wg *sync.WaitGroup) {
	defer wg.Done()
	if r == nil {
		return
	}
	scanner := bufio.NewScanner(r)
	// Buffer up to 1MB per line (default 64KB causes silent abort and deadlock on large logs)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		s.writeOutput("%s%s\n", prefix, line)
	}
}
