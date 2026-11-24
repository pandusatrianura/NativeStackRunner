package runner

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

var ErrPIDNotFound = errors.New("pid file not found")

// SavePID writes the service PID to {pidDir}/{service}.pid.
func SavePID(pidDir, service string, pid int) error {
	if err := os.MkdirAll(pidDir, 0o755); err != nil {
		return fmt.Errorf("creating pid directory: %w", err)
	}

	pidFile := filepath.Join(pidDir, fmt.Sprintf("%s.pid", strings.ToLower(service)))
	data := []byte(strconv.Itoa(pid))

	if err := os.WriteFile(pidFile, data, 0o644); err != nil {
		return fmt.Errorf("writing pid file %s: %w", pidFile, err)
	}
	return nil
}

// ReadPID reads the service PID from {pidDir}/{service}.pid.
func ReadPID(pidDir, service string) (int, error) {
	pidFile := filepath.Join(pidDir, fmt.Sprintf("%s.pid", strings.ToLower(service)))
	data, err := os.ReadFile(pidFile)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return 0, fmt.Errorf("%w for service %s", ErrPIDNotFound, service)
		}
		return 0, fmt.Errorf("reading pid file: %w", err)
	}

	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return 0, fmt.Errorf("parsing pid in %s: %w", pidFile, err)
	}

	return pid, nil
}

// RemovePID deletes {pidDir}/{service}.pid.
func RemovePID(pidDir, service string) error {
	pidFile := filepath.Join(pidDir, fmt.Sprintf("%s.pid", strings.ToLower(service)))
	if err := os.Remove(pidFile); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("removing pid file: %w", err)
	}
	return nil
}

// ListActivePIDs reads all .pid files in pidDir and returns a map of service -> pid.
func ListActivePIDs(pidDir string) (map[string]int, error) {
	result := make(map[string]int)
	entries, err := os.ReadDir(pidDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return result, nil
		}
		return nil, fmt.Errorf("reading pid directory: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".pid") {
			continue
		}
		serviceName := strings.TrimSuffix(entry.Name(), ".pid")
		pid, err := ReadPID(pidDir, serviceName)
		if err == nil {
			result[serviceName] = pid
		}
	}

	return result, nil
}
