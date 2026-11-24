//go:build !windows

package runner

import (
	"os"
	"syscall"
	"time"
)

// IsProcessAlive checks whether the process with the given PID is currently running on Unix.
func IsProcessAlive(pid int) bool {
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	err = process.Signal(syscall.Signal(0))
	return err == nil
}

// StopProcess terminates the process with the given PID on Unix.
func StopProcess(pid int) error {
	process, err := os.FindProcess(pid)
	if err != nil {
		return err
	}

	_ = process.Signal(syscall.SIGTERM)

	done := make(chan struct{})
	go func() {
		for i := 0; i < 30; i++ {
			if !IsProcessAlive(pid) {
				close(done)
				return
			}
			time.Sleep(100 * time.Millisecond)
		}
	}()

	select {
	case <-done:
		return nil
	case <-time.After(3 * time.Second):
		return process.Kill()
	}
}
