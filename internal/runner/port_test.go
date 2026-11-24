package runner_test

import (
	"net"
	"strconv"
	"strings"
	"testing"

	"github.com/pandusatrianura/NativeStackRunner/internal/config"
	"github.com/pandusatrianura/NativeStackRunner/internal/runner"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCheckPortAvailable_FreeAndOccupied(t *testing.T) {
	// 1. Bind an ephemeral port to occupy it
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer listener.Close()

	addr := listener.Addr().String()
	parts := strings.Split(addr, ":")
	occupiedPort, err := strconv.Atoi(parts[len(parts)-1])
	require.NoError(t, err)

	// Checking occupied port must return ErrPortInUse
	err = runner.CheckPortAvailable(occupiedPort)
	require.Error(t, err)
	assert.ErrorIs(t, err, runner.ErrPortInUse)

	// Close listener and re-check; now port must be available
	require.NoError(t, listener.Close())
	err = runner.CheckPortAvailable(occupiedPort)
	require.NoError(t, err)
}

func TestValidateServicePorts(t *testing.T) {
	// Occupy a port
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer listener.Close()

	addr := listener.Addr().String()
	parts := strings.Split(addr, ":")
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
	assert.Contains(t, err.Error(), "service \"redis\"")
}
