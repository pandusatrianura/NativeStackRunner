package binary

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockLibcDetector struct {
	libcType LibcType
	err      error
}

func (m *mockLibcDetector) Detect() (LibcType, error) {
	return m.libcType, m.err
}

type mockLibcDownloader struct{}

func (m *mockLibcDownloader) Download(ctx context.Context, url string, destPath string) error {
	return nil
}

func TestDetectLibcFromRoot(t *testing.T) {
	tempRoot := t.TempDir()

	t.Run("defaults to glibc when standard directories are empty", func(t *testing.T) {
		libc, err := detectLibcFromRoot(tempRoot)
		require.NoError(t, err)
		assert.Equal(t, LibcGlibc, libc)
	})

	t.Run("detects musl when alpine-release is present", func(t *testing.T) {
		alpineRoot := t.TempDir()
		etcDir := filepath.Join(alpineRoot, "etc")
		require.NoError(t, os.MkdirAll(etcDir, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(etcDir, "alpine-release"), []byte("3.19.0\n"), 0o644))

		libc, err := detectLibcFromRoot(alpineRoot)
		require.NoError(t, err)
		assert.Equal(t, LibcMusl, libc)
	})

	t.Run("detects musl when ld-musl dynamic linker is found", func(t *testing.T) {
		muslRoot := t.TempDir()
		libDir := filepath.Join(muslRoot, "lib")
		require.NoError(t, os.MkdirAll(libDir, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(libDir, "ld-musl-x86_64.so.1"), []byte("stub"), 0o755))

		libc, err := detectLibcFromRoot(muslRoot)
		require.NoError(t, err)
		assert.Equal(t, LibcMusl, libc)
	})
}

func TestManager_MuslPostgresRejection(t *testing.T) {
	tempDir := t.TempDir()
	mgr, err := NewManager(tempDir, &mockLibcDownloader{})
	require.NoError(t, err)

	// Inject mock detector reporting Musl
	mgr.SetLibcDetector(&mockLibcDetector{libcType: LibcMusl})

	// Override enforce target to simulate Linux Postgres check
	mgr.enforceLinuxLibcCheck = true

	_, err = mgr.EnsureBinary(context.Background(), "postgres", "16.1.0")
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrIncompatibleLibc))
	assert.Contains(t, err.Error(), "gcompat")
}
