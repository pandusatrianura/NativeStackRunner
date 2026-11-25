package runner

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/pandusatrianura/NativeStackRunner/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExtractMajorVersion(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"16.1.0", "16"},
		{"15.5.0", "15"},
		{"14", "14"},
		{"  16.2  ", "16"},
		{"", "16"}, // default
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			assert.Equal(t, tt.expected, extractMajorVersion(tt.input))
		})
	}
}

func TestValidatePostgresDataVersion(t *testing.T) {
	tempDir := t.TempDir()

	t.Run("fresh directory without PG_VERSION succeeds", func(t *testing.T) {
		freshDir := filepath.Join(tempDir, "fresh")
		require.NoError(t, os.MkdirAll(freshDir, 0o755))

		err := ValidatePostgresDataVersion(freshDir, "16.1.0")
		assert.NoError(t, err)
	})

	t.Run("matching major version succeeds", func(t *testing.T) {
		matchDir := filepath.Join(tempDir, "match16")
		require.NoError(t, os.MkdirAll(matchDir, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(matchDir, "PG_VERSION"), []byte("16\n"), 0o644))

		err := ValidatePostgresDataVersion(matchDir, "16.1.0")
		assert.NoError(t, err)

		// Also check with empty requested version (defaults to 16)
		err = ValidatePostgresDataVersion(matchDir, "")
		assert.NoError(t, err)
	})

	t.Run("matching major version 15 succeeds", func(t *testing.T) {
		matchDir := filepath.Join(tempDir, "match15")
		require.NoError(t, os.MkdirAll(matchDir, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(matchDir, "PG_VERSION"), []byte("15\n"), 0o644))

		err := ValidatePostgresDataVersion(matchDir, "15.5.0")
		assert.NoError(t, err)
	})

	t.Run("mismatched major version returns ErrIncompatiblePGVersion", func(t *testing.T) {
		mismatchDir := filepath.Join(tempDir, "mismatch")
		require.NoError(t, os.MkdirAll(mismatchDir, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(mismatchDir, "PG_VERSION"), []byte("15\n"), 0o644))

		err := ValidatePostgresDataVersion(mismatchDir, "16.1.0")
		require.Error(t, err)
		assert.True(t, errors.Is(err, ErrIncompatiblePGVersion))
		assert.Contains(t, err.Error(), "data directory at")
		assert.Contains(t, err.Error(), "was initialized with PostgreSQL version 15, but configuration requested 16")
		assert.Contains(t, err.Error(), "stack clean")
	})
}

func TestBuildServiceCmd_PostgresVersionMismatch(t *testing.T) {
	tempProject := t.TempDir()
	dataDir := filepath.Join(tempProject, ".stack", "data", "postgres")
	require.NoError(t, os.MkdirAll(dataDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dataDir, "PG_VERSION"), []byte("14\n"), 0o644))

	svc := config.ServiceConfig{
		Name:    "postgres",
		Port:    5432,
		Version: "16.1.0",
		User:    "postgres",
	}

	_, err := BuildServiceCmd(context.Background(), svc, "postgres", tempProject)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrIncompatiblePGVersion))
}

func TestSupervisor_PipeLogLargeLine(t *testing.T) {
	var outBuf bytes.Buffer
	s := NewSupervisor(nil, "", nil, nil, &outBuf)

	// Line larger than default 64KB bufio.MaxScanTokenSize (100 KB)
	largeContent := strings.Repeat("X", 100*1024)
	r := strings.NewReader(largeContent + "\n")

	var wg sync.WaitGroup
	wg.Add(1)
	s.pipeLog(r, "[test] ", &wg)
	wg.Wait()

	assert.Contains(t, outBuf.String(), "[test] ")
	assert.Contains(t, outBuf.String(), largeContent)
}

func TestBuildServiceCmd_Mailpit(t *testing.T) {
	tempProject := t.TempDir()

	svc := config.ServiceConfig{
		Name:   "mailpit",
		Port:   1025,
		UIPort: 8025,
	}

	spec, err := BuildServiceCmd(context.Background(), svc, "mailpit", tempProject)
	require.NoError(t, err)

	assert.Equal(t, "mailpit", spec.Service)
	assert.Equal(t, "mailpit", spec.BinPath)
	assert.Contains(t, spec.Args, "--smtp-listen")
	assert.Contains(t, spec.Args, "127.0.0.1:1025")
	assert.Contains(t, spec.Args, "--listen")
	assert.Contains(t, spec.Args, "127.0.0.1:8025")
	assert.Contains(t, spec.Args, "--db-file")
}
