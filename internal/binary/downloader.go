package binary

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

var (
	ErrUnsupportedPlatform = errors.New("unsupported operating system or architecture")
	ErrDownloadFailed      = errors.New("binary download failed")
	ErrExtractionFailed    = errors.New("archive extraction failed")
	ErrBinaryNotFound      = errors.New("executable binary not found in archive")
	ErrChecksumMismatch    = errors.New("binary checksum mismatch: potential corruption or security tampering")
)

// Downloader abstracts HTTP fetching for testability.
type Downloader interface {
	Download(ctx context.Context, url string, destPath string) error
}

// HTTPDownloader is the production implementation of Downloader.
type HTTPDownloader struct {
	client      *http.Client
	progressOut io.Writer
}

// NewHTTPDownloader returns a configured HTTPDownloader.
func NewHTTPDownloader(timeout time.Duration) *HTTPDownloader {
	if timeout == 0 {
		timeout = 5 * time.Minute
	}
	return &HTTPDownloader{
		client: &http.Client{Timeout: timeout},
	}
}

// SetProgressOutput sets the output writer for download progress reporting.
func (d *HTTPDownloader) SetProgressOutput(w io.Writer) {
	d.progressOut = w
}

// Download downloads from url to destPath safely via a temporary file.
func (d *HTTPDownloader) Download(ctx context.Context, url string, destPath string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("creating download request: %w", err)
	}

	resp, err := d.client.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %s: %w", ErrDownloadFailed, url, err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("%w: HTTP status %d for %s", ErrDownloadFailed, resp.StatusCode, url)
	}

	if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
		return fmt.Errorf("creating destination directory: %w", err)
	}

	// Download to a temporary file in the same directory for atomic rename
	tmpPath := destPath + ".tmp"
	tmpFile, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return fmt.Errorf("creating temporary download file: %w", err)
	}

	var writer io.Writer = tmpFile
	if d.progressOut != nil {
		serviceName := filepath.Base(filepath.Dir(filepath.Dir(destPath)))
		if serviceName == "." || serviceName == "" {
			serviceName = filepath.Base(destPath)
		}
		pw := NewProgressWriter(d.progressOut, serviceName, resp.ContentLength)
		defer pw.Finish()
		writer = io.MultiWriter(tmpFile, pw)
	}

	if _, err := io.Copy(writer, resp.Body); err != nil {
		_ = tmpFile.Close()
		_ = os.Remove(tmpPath)
		return fmt.Errorf("writing download content: %w", err)
	}

	if err := tmpFile.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("closing temporary download file: %w", err)
	}

	if err := os.Rename(tmpPath, destPath); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("renaming temp file to %s: %w", destPath, err)
	}

	return nil
}
