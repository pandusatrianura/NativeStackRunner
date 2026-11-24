package runner

import (
	"bufio"
	"context"
	"fmt"
	"io"
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
	}
}

// Start boots all services in the stack, multiplexes logs, monitors child health, and terminates on ctx cancellation or crash.
func (s *Supervisor) Start(ctx context.Context) error {
	// 0. Pre-flight check: ensure all declared service ports are available
	if err := ValidateServicePorts(s.cfg.Services); err != nil {
		return fmt.Errorf("pre-flight port check: %w", err)
	}

	pidsDir := filepath.Join(s.projectDir, ".stack", "pids")
	logsDir := filepath.Join(s.projectDir, ".stack", "logs")

	if err := os.MkdirAll(pidsDir, 0o755); err != nil {
		return fmt.Errorf("creating pids directory: %w", err)
	}
	if err := os.MkdirAll(logsDir, 0o755); err != nil {
		return fmt.Errorf("creating logs directory: %w", err)
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

	var wg sync.WaitGroup
	colorIdx := 0

	for name, svc := range s.cfg.Services {
		// 1. Ensure binary exists
		binPath, err := s.binaryMgr.EnsureBinary(ctx, svc.Name, svc.Version)
		if err != nil {
			rollback()
			return fmt.Errorf("ensuring binary for %s: %w", name, err)
		}

		// 2. Build command spec
		cmdSpec, err := BuildServiceCmd(ctx, svc, binPath, s.projectDir)
		if err != nil {
			rollback()
			return fmt.Errorf("building command for %s: %w", name, err)
		}

		// 3. Start process
		handle, stdout, stderr, err := s.executor.Start(ctx, cmdSpec)
		if err != nil {
			rollback()
			return fmt.Errorf("starting process for %s: %w", name, err)
		}

		handles = append(handles, runningHandle{service: name, handle: handle})

		// 4. Save PID
		if err := SavePID(pidsDir, name, handle.PID()); err != nil {
			_, _ = fmt.Fprintf(s.outputWriter, "Warning: failed to save PID for %s: %v\n", name, err)
		}

		// 5. Pipe logs
		serviceColor := colors[colorIdx%len(colors)]
		colorIdx++
		prefix := serviceColor.Sprintf("[%s | %d] ", name, svc.Port)

		wg.Add(2)
		go s.pipeLog(stdout, prefix, &wg)
		go s.pipeLog(stderr, prefix, &wg)
	}

	_, _ = fmt.Fprintf(s.outputWriter, "\nAll services launched successfully in project %s.\nPress Ctrl+C to stop.\n\n", s.projectDir)

	// Monitor child processes for unexpected exit/crash
	exitCh := make(chan processExit, len(handles))
	for _, h := range handles {
		hCopy := h
		go func() {
			waitErr := hCopy.handle.Wait()
			// Only report if parent context has not initiated a planned shutdown
			if ctx.Err() == nil {
				exitCh <- processExit{service: hCopy.service, err: waitErr}
			}
		}()
	}

	select {
	case <-ctx.Done():
		_, _ = fmt.Fprintln(s.outputWriter, "\nShutting down services gracefully...")
		rollback()
		s.drainLogs(&wg, 500*time.Millisecond)
		return nil

	case exit := <-exitCh:
		errMsg := fmt.Sprintf("\n[ERROR] Service %q terminated unexpectedly (exit: %v). Shutting down all services...", exit.service, exit.err)
		_, _ = fmt.Fprintln(s.outputWriter, errMsg)
		rollback()
		s.drainLogs(&wg, 500*time.Millisecond)
		return fmt.Errorf("service %q terminated unexpectedly: %w", exit.service, exit.err)
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
	for scanner.Scan() {
		line := scanner.Text()
		_, _ = fmt.Fprintf(s.outputWriter, "%s%s\n", prefix, line)
	}
}
