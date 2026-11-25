package runner

import (
	"errors"
	"fmt"
	"net"

	"github.com/pandusatrianura/NativeStackRunner/internal/config"
)

var ErrPortInUse = errors.New("port is already in use by another process")

// CheckPortAvailable tests whether a TCP port on localhost (127.0.0.1) can be bound.
func CheckPortAvailable(port int) error {
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	l, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("%w: %d (address %s is unavailable; please stop conflicting service or change port in stack.yaml)", ErrPortInUse, port, addr)
	}
	_ = l.Close()
	return nil
}

// ValidateServicePorts checks all ports and console ports across declared services before startup.
func ValidateServicePorts(services map[string]config.ServiceConfig) error {
	for name, svc := range services {
		if svc.Port > 0 {
			if err := CheckPortAvailable(svc.Port); err != nil {
				return fmt.Errorf("pre-flight port check failed for service %q: %w", name, err)
			}
		}
		if svc.ConsolePort > 0 {
			if err := CheckPortAvailable(svc.ConsolePort); err != nil {
				return fmt.Errorf("pre-flight port check failed for service %q console: %w", name, err)
			}
		}
		if svc.UIPort > 0 {
			if err := CheckPortAvailable(svc.UIPort); err != nil {
				return fmt.Errorf("pre-flight port check failed for service %q UI: %w", name, err)
			}
		}
	}
	return nil
}
