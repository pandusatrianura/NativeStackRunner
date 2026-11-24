package binary

import (
	"errors"
	"os"
	"path/filepath"
)

// LibcType represents the C standard library implementation on the host system.
type LibcType string

const (
	LibcGlibc   LibcType = "glibc"
	LibcMusl    LibcType = "musl"
	LibcUnknown LibcType = "unknown"
)

// ErrIncompatibleLibc indicates the target binary cannot execute under the detected C library.
var ErrIncompatibleLibc = errors.New("incompatible C standard library (musl detected)")

// LibcDetector abstracts libc detection for testability and portability.
type LibcDetector interface {
	Detect() (LibcType, error)
}

// detectLibcFromRoot inspects standard Linux filesystem locations under rootDir to identify musl vs glibc.
// Exposed package-internal for cross-platform unit testing.
func detectLibcFromRoot(rootDir string) (LibcType, error) {
	// 1. Check for Alpine Linux indicator
	if _, err := os.Stat(filepath.Join(rootDir, "etc", "alpine-release")); err == nil {
		return LibcMusl, nil
	}

	// 2. Check standard musl dynamic linker patterns in lib, lib64, and usr/lib
	libDirs := []string{
		filepath.Join(rootDir, "lib"),
		filepath.Join(rootDir, "lib64"),
		filepath.Join(rootDir, "usr", "lib"),
	}

	for _, dir := range libDirs {
		matches, err := filepath.Glob(filepath.Join(dir, "ld-musl-*.so*"))
		if err == nil && len(matches) > 0 {
			return LibcMusl, nil
		}
	}

	return LibcGlibc, nil
}
