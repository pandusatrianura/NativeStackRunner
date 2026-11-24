package binary_test

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/pandusatrianura/NativeStackRunner/internal/binary"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/ulikunitz/xz"
)

// mockDownloader implements binary.Downloader for isolated tests.
type mockDownloader struct {
	downloadCount int
	payload       []byte
	err           error
}

func (m *mockDownloader) Download(ctx context.Context, url string, destPath string) error {
	m.downloadCount++
	if m.err != nil {
		return m.err
	}
	if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
		return err
	}
	return os.WriteFile(destPath, m.payload, 0o755)
}

type mockManifest struct {
	spec *binary.Spec
	err  error
}

func (m *mockManifest) Resolve(service, version, targetOS, targetArch string) (*binary.Spec, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.spec, nil
}

func computeHash(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

func TestManifestResolve(t *testing.T) {
	manifest := binary.NewManifest()

	tests := []struct {
		name       string
		service    string
		version    string
		targetOS   string
		targetArch string
		wantErr    bool
		validate   func(t *testing.T, spec *binary.Spec)
	}{
		{
			name:       "minio on windows amd64",
			service:    "minio",
			version:    "latest",
			targetOS:   "windows",
			targetArch: "amd64",
			wantErr:    false,
			validate: func(t *testing.T, spec *binary.Spec) {
				assert.Equal(t, "minio", spec.Service)
				assert.Equal(t, binary.ArchiveRaw, spec.Type)
				assert.Equal(t, "minio.exe", spec.BinarySubpath)
				assert.Contains(t, spec.URL, "minio.windows-amd64")
				assert.NotEmpty(t, spec.SHA256)
			},
		},
		{
			name:       "minio on linux arm64",
			service:    "minio",
			version:    "latest",
			targetOS:   "linux",
			targetArch: "arm64",
			wantErr:    false,
			validate: func(t *testing.T, spec *binary.Spec) {
				assert.Equal(t, "minio", spec.Service)
				assert.Equal(t, binary.ArchiveRaw, spec.Type)
				assert.Equal(t, "minio", spec.BinarySubpath)
				assert.Contains(t, spec.URL, "minio.linux-arm64")
				assert.NotEmpty(t, spec.SHA256)
			},
		},
		{
			name:       "redis on windows amd64",
			service:    "redis",
			version:    "latest",
			targetOS:   "windows",
			targetArch: "amd64",
			wantErr:    false,
			validate: func(t *testing.T, spec *binary.Spec) {
				assert.Equal(t, "redis", spec.Service)
				assert.Equal(t, binary.ArchiveZip, spec.Type)
				assert.Equal(t, "redis-server.exe", spec.BinarySubpath)
			},
		},
		{
			name:       "postgres on linux amd64",
			service:    "postgres",
			version:    "16",
			targetOS:   "linux",
			targetArch: "amd64",
			wantErr:    false,
			validate: func(t *testing.T, spec *binary.Spec) {
				assert.Equal(t, "postgres", spec.Service)
				assert.Equal(t, binary.ArchiveZip, spec.Type)
				assert.Equal(t, "bin/postgres", spec.BinarySubpath)
			},
		},
		{
			name:       "unknown service returns error",
			service:    "unknown-db",
			version:    "1.0",
			targetOS:   "linux",
			targetArch: "amd64",
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec, err := manifest.Resolve(tt.service, tt.version, tt.targetOS, tt.targetArch)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, spec)
			if tt.validate != nil {
				tt.validate(t, spec)
			}
		})
	}
}

func TestManifestResolve_AllPermutationsHaveChecksum(t *testing.T) {
	manifest := binary.NewManifest()

	platforms := []struct {
		os   string
		arch string
	}{
		{"windows", "amd64"},
		{"linux", "amd64"},
		{"linux", "arm64"},
		{"darwin", "amd64"},
		{"darwin", "arm64"},
	}

	// 1. MinIO
	for _, p := range platforms {
		t.Run("minio_"+p.os+"_"+p.arch, func(t *testing.T) {
			spec, err := manifest.Resolve("minio", "latest", p.os, p.arch)
			require.NoError(t, err)
			require.NotNil(t, spec)
			assert.NotEmpty(t, spec.URL)
			assert.NotEmpty(t, spec.BinarySubpath)
			assert.Len(t, spec.SHA256, 64, "expected 64-char hex sha256")
		})
	}

	// 2. Redis
	for _, p := range platforms {
		t.Run("redis_"+p.os+"_"+p.arch, func(t *testing.T) {
			spec, err := manifest.Resolve("redis", "latest", p.os, p.arch)
			require.NoError(t, err)
			require.NotNil(t, spec)
			assert.NotEmpty(t, spec.URL)
			assert.NotEmpty(t, spec.BinarySubpath)
			assert.Len(t, spec.SHA256, 64, "expected 64-char hex sha256")
		})
	}

	// 3. PostgreSQL (versions 16.1.0, 15.5.0, 14.10.0)
	pgVersions := []string{"16.1.0", "15.5.0", "14.10.0"}
	for _, v := range pgVersions {
		for _, p := range platforms {
			t.Run("postgres_"+v+"_"+p.os+"_"+p.arch, func(t *testing.T) {
				spec, err := manifest.Resolve("postgres", v, p.os, p.arch)
				require.NoError(t, err)
				require.NotNil(t, spec)
				assert.NotEmpty(t, spec.URL)
				assert.NotEmpty(t, spec.BinarySubpath)
				assert.Len(t, spec.SHA256, 64, "expected 64-char hex sha256")
			})
		}
	}
}

func TestEnsureBinary_Raw(t *testing.T) {
	tempCache := t.TempDir()
	mockDL := &mockDownloader{
		payload: []byte("#!/bin/sh\necho mock minio"),
	}

	mgr, err := binary.NewManager(tempCache, mockDL)
	require.NoError(t, err)

	binName := "minio"
	if runtime.GOOS == "windows" {
		binName = "minio.exe"
	}
	mgr.SetManifest(&mockManifest{
		spec: &binary.Spec{
			Service:       "minio",
			Version:       "mock",
			Type:          binary.ArchiveRaw,
			BinarySubpath: binName,
			SHA256:        computeHash(mockDL.payload),
		},
	})

	ctx := context.Background()
	binPath, err := mgr.EnsureBinary(ctx, "minio", "latest")
	require.NoError(t, err)
	assert.FileExists(t, binPath)
	assert.Equal(t, 1, mockDL.downloadCount)

	// Second call must use cache and not download again
	cachedPath, err := mgr.EnsureBinary(ctx, "minio", "latest")
	require.NoError(t, err)
	assert.Equal(t, binPath, cachedPath)
	assert.Equal(t, 1, mockDL.downloadCount)
}

func TestEnsureBinary_Zip(t *testing.T) {
	tempCache := t.TempDir()

	// Build in-memory zip archive containing mock binary
	buf := new(bytes.Buffer)
	zipWriter := zip.NewWriter(buf)

	binName := "redis-server.exe"
	f, err := zipWriter.Create(binName)
	require.NoError(t, err)
	_, err = f.Write([]byte("mock-redis-executable"))
	require.NoError(t, err)
	require.NoError(t, zipWriter.Close())

	mockDL := &mockDownloader{payload: buf.Bytes()}
	mgr, err := binary.NewManager(tempCache, mockDL)
	require.NoError(t, err)
	mgr.SetManifest(&mockManifest{
		spec: &binary.Spec{
			Service:       "redis",
			Version:       "mock",
			Type:          binary.ArchiveZip,
			BinarySubpath: binName,
			SHA256:        computeHash(mockDL.payload),
		},
	})

	ctx := context.Background()
	binPath, err := mgr.EnsureBinary(ctx, "redis", "latest")
	require.NoError(t, err)
	assert.FileExists(t, binPath)
	assert.Equal(t, 1, mockDL.downloadCount)
}

func TestEnsureBinary_SiblingPermissions(t *testing.T) {
	tempCache := t.TempDir()

	buf := new(bytes.Buffer)
	zipWriter := zip.NewWriter(buf)

	binSubpath := "bin/postgres"
	if runtime.GOOS == "windows" {
		binSubpath = "bin/postgres.exe"
	}

	f1, err := zipWriter.Create(binSubpath)
	require.NoError(t, err)
	_, err = f1.Write([]byte("mock-postgres"))
	require.NoError(t, err)

	siblingSubpath := "bin/initdb"
	if runtime.GOOS == "windows" {
		siblingSubpath = "bin/initdb.exe"
	}
	f2, err := zipWriter.Create(siblingSubpath)
	require.NoError(t, err)
	_, err = f2.Write([]byte("mock-initdb"))
	require.NoError(t, err)
	require.NoError(t, zipWriter.Close())

	mockDL := &mockDownloader{payload: buf.Bytes()}
	mgr, err := binary.NewManager(tempCache, mockDL)
	require.NoError(t, err)
	mgr.SetManifest(&mockManifest{
		spec: &binary.Spec{
			Service:       "postgres",
			Version:       "mock",
			Type:          binary.ArchiveZip,
			BinarySubpath: binSubpath,
			SHA256:        computeHash(mockDL.payload),
		},
	})

	ctx := context.Background()
	binPath, err := mgr.EnsureBinary(ctx, "postgres", "16")
	require.NoError(t, err)
	assert.FileExists(t, binPath)

	siblingPath := filepath.Join(filepath.Dir(binPath), filepath.Base(siblingSubpath))
	assert.FileExists(t, siblingPath)
}

func TestEnsureBinary_ZipSlipDefense(t *testing.T) {
	tempCache := t.TempDir()

	// Build malicious zip with ../ traversal
	buf := new(bytes.Buffer)
	zipWriter := zip.NewWriter(buf)

	f, err := zipWriter.Create("../malicious.sh")
	require.NoError(t, err)
	_, err = f.Write([]byte("malicious content"))
	require.NoError(t, err)
	require.NoError(t, zipWriter.Close())

	mockDL := &mockDownloader{payload: buf.Bytes()}
	mgr, err := binary.NewManager(tempCache, mockDL)
	require.NoError(t, err)
	mgr.SetManifest(&mockManifest{
		spec: &binary.Spec{
			Service:       "redis",
			Version:       "mock",
			Type:          binary.ArchiveZip,
			BinarySubpath: "redis-server.exe",
			SHA256:        computeHash(mockDL.payload),
		},
	})

	ctx := context.Background()
	_, err = mgr.EnsureBinary(ctx, "redis", "latest")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "illegal file path in zip archive")
}

func TestVerifyChecksum(t *testing.T) {
	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "test.bin")
	content := []byte("hello native stack runner")
	require.NoError(t, os.WriteFile(filePath, content, 0o644))

	hasher := sha256.New()
	hasher.Write(content)
	correctHash := hex.EncodeToString(hasher.Sum(nil))

	// 1. Valid hash passes
	err := binary.VerifyChecksum(filePath, correctHash)
	require.NoError(t, err)

	// 2. Empty hash passes (optional check)
	err = binary.VerifyChecksum(filePath, "")
	require.NoError(t, err)

	// 3. Mismatched hash fails with ErrChecksumMismatch
	err = binary.VerifyChecksum(filePath, "0000000000000000000000000000000000000000000000000000000000000000")
	require.Error(t, err)
	assert.ErrorIs(t, err, binary.ErrChecksumMismatch)
}

func TestEnsureBinary_NestedXzInZip(t *testing.T) {
	tempCache := t.TempDir()

	binSubpath := "bin/postgres"
	if runtime.GOOS == "windows" {
		binSubpath = "bin/postgres.exe"
	}

	// 1. Create inner .txz archive
	txzBuf := new(bytes.Buffer)
	xzWriter, err := xz.NewWriter(txzBuf)
	require.NoError(t, err)
	tarWriter := tar.NewWriter(xzWriter)

	mockContent := []byte("mock-embedded-postgres-binary")
	err = tarWriter.WriteHeader(&tar.Header{
		Name: binSubpath,
		Mode: 0o755,
		Size: int64(len(mockContent)),
	})
	require.NoError(t, err)
	_, err = tarWriter.Write(mockContent)
	require.NoError(t, err)
	require.NoError(t, tarWriter.Close())
	require.NoError(t, xzWriter.Close())

	// 2. Wrap inner .txz into outer .zip (matching Zonky JAR structure)
	zipBuf := new(bytes.Buffer)
	zipWriter := zip.NewWriter(zipBuf)
	f, err := zipWriter.Create("postgres-windows-x86_64.txz")
	require.NoError(t, err)
	_, err = f.Write(txzBuf.Bytes())
	require.NoError(t, err)
	require.NoError(t, zipWriter.Close())

	mockDL := &mockDownloader{payload: zipBuf.Bytes()}
	mgr, err := binary.NewManager(tempCache, mockDL)
	require.NoError(t, err)

	mgr.SetManifest(&mockManifest{
		spec: &binary.Spec{
			Service:       "postgres",
			Version:       "16.1.0",
			Type:          binary.ArchiveZip,
			BinarySubpath: binSubpath,
			SHA256:        computeHash(mockDL.payload),
		},
	})

	ctx := context.Background()
	binPath, err := mgr.EnsureBinary(ctx, "postgres", "16")
	require.NoError(t, err)
	assert.FileExists(t, binPath)

	// Ensure the extracted binary contains our payload
	data, err := os.ReadFile(binPath)
	require.NoError(t, err)
	assert.Equal(t, mockContent, data)

	// Verify the nested .txz file was cleaned up
	nestedPath := filepath.Join(filepath.Dir(filepath.Dir(binPath)), "postgres-windows-x86_64.txz")
	assert.NoFileExists(t, nestedPath)
}
