package binary

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/ulikunitz/xz"
)

// Manager coordinates checking the local cache, downloading, and extracting service binaries.
type Manager struct {
	cacheDir              string
	downloader            Downloader
	manifest              ManifestResolver
	libcDetector          LibcDetector
	enforceLinuxLibcCheck bool
}

// NewManager creates a new Manager instance.
// If cacheDir is empty, defaults to ~/.stack/cache.
func NewManager(cacheDir string, dl Downloader) (*Manager, error) {
	if cacheDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("resolving user home directory: %w", err)
		}
		cacheDir = filepath.Join(home, ".stack", "cache")
	}

	if dl == nil {
		dl = NewHTTPDownloader(0)
	}

	return &Manager{
		cacheDir:     cacheDir,
		downloader:   dl,
		manifest:     NewManifest(),
		libcDetector: DefaultLibcDetector(),
	}, nil
}

// SetLibcDetector configures the C standard library detector (primarily for testing and overrides).
func (m *Manager) SetLibcDetector(d LibcDetector) {
	m.libcDetector = d
}

// SetManifest configures the manifest resolver (useful for testing).
func (m *Manager) SetManifest(manifest ManifestResolver) {
	m.manifest = manifest
}

// SetProgressOutput enables interactive progress reporting on the underlying downloader.
func (m *Manager) SetProgressOutput(w io.Writer) {
	if hd, ok := m.downloader.(*HTTPDownloader); ok {
		hd.SetProgressOutput(w)
	}
}

// EnsureBinary ensures the specified binary is downloaded, extracted, and ready for execution.
// Returns the absolute path to the executable binary.
func (m *Manager) EnsureBinary(ctx context.Context, service, version string) (string, error) {
	spec, err := m.manifest.Resolve(service, version, "", "")
	if err != nil {
		return "", fmt.Errorf("resolving binary spec: %w", err)
	}

	// Intercept PostgreSQL on Linux if host uses musl libc
	if (runtime.GOOS == "linux" || m.enforceLinuxLibcCheck) && (spec.Service == "postgres" || spec.Service == "postgresql") {
		if m.libcDetector != nil {
			if libc, err := m.libcDetector.Detect(); err == nil && libc == LibcMusl {
				return "", fmt.Errorf("%w: PostgreSQL precompiled binaries require glibc. Alpine Linux uses musl; please install 'gcompat' (apk add gcompat) or use a glibc environment", ErrIncompatibleLibc)
			}
		}
	}

	installDir := filepath.Join(m.cacheDir, "bin", spec.Service, spec.Version)
	binPath := filepath.Join(installDir, spec.BinarySubpath)

	// Check if executable already exists in cache
	if info, err := os.Stat(binPath); err == nil && !info.IsDir() {
		return binPath, nil
	}

	// Prepare download location
	if err := os.MkdirAll(installDir, 0o755); err != nil {
		return "", fmt.Errorf("creating service installation directory: %w", err)
	}

	downloadPath := filepath.Join(installDir, "download_asset")
	if spec.Type == ArchiveZip {
		downloadPath += ".zip"
	} else if spec.Type == ArchiveTarGz {
		downloadPath += ".tar.gz"
	}

	// Download the asset
	if err := m.downloader.Download(ctx, spec.URL, downloadPath); err != nil {
		return "", fmt.Errorf("downloading %s: %w", spec.Service, err)
	}
	defer func() {
		// Clean up downloaded archive after extraction if it is an archive
		if spec.Type != ArchiveRaw {
			_ = os.Remove(downloadPath)
		}
	}()

	// Verify SHA-256 checksum if specified
	if err := VerifyChecksum(downloadPath, spec.SHA256); err != nil {
		_ = os.Remove(downloadPath)
		return "", err
	}

	// Extract or position the binary
	switch spec.Type {
	case ArchiveRaw:
		// Raw binary was downloaded directly to downloadPath. Rename to target binary path.
		if err := os.Rename(downloadPath, binPath); err != nil {
			return "", fmt.Errorf("positioning raw binary: %w", err)
		}
		_ = os.Chmod(binPath, 0o755)

	case ArchiveZip:
		if err := extractZip(downloadPath, installDir); err != nil {
			return "", fmt.Errorf("%w: %s", ErrExtractionFailed, err)
		}

	case ArchiveTarGz:
		if err := extractTarGz(downloadPath, installDir); err != nil {
			return "", fmt.Errorf("%w: %s", ErrExtractionFailed, err)
		}

	case ArchiveTarXz:
		if err := extractTarXz(downloadPath, installDir); err != nil {
			return "", fmt.Errorf("%w: %s", ErrExtractionFailed, err)
		}
	}

	// Check if extracted binary exists
	if _, err := os.Stat(binPath); err != nil {
		return "", fmt.Errorf("%w: expected at %s: %w", ErrBinaryNotFound, binPath, err)
	}

	ensureExecutablePermissions(installDir)
	return binPath, nil
}

// ensureExecutablePermissions recursively sets 0755 permissions on all files in installDir.
func ensureExecutablePermissions(installDir string) {
	_ = filepath.Walk(installDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info == nil || info.IsDir() {
			return nil
		}
		_ = os.Chmod(path, 0o755)
		return nil
	})
}

// extractZip unpacks a zip/jar archive safely guarding against Zip Slip vulnerabilities.
func extractZip(archivePath, targetDir string) error {
	r, err := zip.OpenReader(archivePath)
	if err != nil {
		return fmt.Errorf("opening zip reader: %w", err)
	}
	defer func() {
		_ = r.Close()
	}()

	targetDirClean := filepath.Clean(targetDir)

	for _, f := range r.File {
		destPath := filepath.Join(targetDir, f.Name)
		destClean := filepath.Clean(destPath)

		// Guard against Zip Slip directory traversal
		if !strings.HasPrefix(destClean, targetDirClean+string(filepath.Separator)) && destClean != targetDirClean {
			return fmt.Errorf("illegal file path in zip archive: %s", f.Name)
		}

		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(destPath, 0o755); err != nil {
				return err
			}
			continue
		}

		if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
			return err
		}

		rc, err := f.Open()
		if err != nil {
			return err
		}

		out, err := os.OpenFile(destPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, f.Mode())
		if err != nil {
			_ = rc.Close()
			return err
		}

		_, copyErr := io.Copy(out, rc)
		_ = rc.Close()
		_ = out.Close()

		if copyErr != nil {
			return copyErr
		}
	}

	// Unpack nested .txz or .tar.xz archives if present (e.g. Zonky embedded-postgres JARs)
	entries, readErr := os.ReadDir(targetDir)
	if readErr == nil {
		for _, entry := range entries {
			if !entry.IsDir() && (strings.HasSuffix(entry.Name(), ".txz") || strings.HasSuffix(entry.Name(), ".tar.xz")) {
				nestedPath := filepath.Join(targetDir, entry.Name())
				if err := extractTarXz(nestedPath, targetDir); err != nil {
					return fmt.Errorf("extracting nested xz archive %s: %w", entry.Name(), err)
				}
				_ = os.Remove(nestedPath)
			}
		}
	}

	return nil
}

// extractTar unpacks a tar stream safely.
func extractTar(reader io.Reader, targetDir string) error {
	tr := tar.NewReader(reader)
	targetDirClean := filepath.Clean(targetDir)

	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return err
		}

		destPath := filepath.Join(targetDir, hdr.Name)
		destClean := filepath.Clean(destPath)

		// Guard against Tar directory traversal
		if !strings.HasPrefix(destClean, targetDirClean+string(filepath.Separator)) && destClean != targetDirClean {
			return fmt.Errorf("illegal file path in tar archive: %s", hdr.Name)
		}

		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(destPath, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
				return err
			}
			out, err := os.OpenFile(destPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(hdr.Mode))
			if err != nil {
				return err
			}
			if _, err := io.Copy(out, tr); err != nil {
				_ = out.Close()
				return err
			}
			_ = out.Close()
		case tar.TypeSymlink:
			if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
				return err
			}
			_ = os.Remove(destPath)
			_ = os.Symlink(hdr.Linkname, destPath)
		}
	}

	return nil
}

// extractTarGz unpacks a .tar.gz archive safely.
func extractTarGz(archivePath, targetDir string) error {
	f, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer func() {
		_ = f.Close()
	}()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer func() {
		_ = gz.Close()
	}()

	return extractTar(gz, targetDir)
}

// extractTarXz unpacks a .tar.xz or .txz archive safely.
func extractTarXz(archivePath, targetDir string) error {
	f, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer func() {
		_ = f.Close()
	}()

	xzReader, err := xz.NewReader(f)
	if err != nil {
		return fmt.Errorf("creating xz reader: %w", err)
	}

	return extractTar(xzReader, targetDir)
}

// VerifyChecksum checks whether the file matches the expected SHA-256 hash string.
func VerifyChecksum(filePath string, expectedHash string) error {
	if expectedHash == "" {
		return nil
	}

	f, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("opening file for checksum verification: %w", err)
	}
	defer func() {
		_ = f.Close()
	}()

	hasher := sha256.New()
	if _, err := io.Copy(hasher, f); err != nil {
		return fmt.Errorf("hashing file: %w", err)
	}

	actualHash := hex.EncodeToString(hasher.Sum(nil))
	if !strings.EqualFold(actualHash, strings.TrimSpace(expectedHash)) {
		return fmt.Errorf("%w: expected %s, got %s", ErrChecksumMismatch, expectedHash, actualHash)
	}

	return nil
}
