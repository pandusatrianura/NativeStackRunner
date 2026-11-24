package binary

import (
	"fmt"
	"io"
	"strings"
	"sync"
	"time"
)

// ProgressWriter tracks byte copying progress and writes single-line updates to an io.Writer.
type ProgressWriter struct {
	totalBytes   int64
	currentBytes int64
	serviceName  string
	out          io.Writer
	mu           sync.Mutex
	lastUpdate   time.Time
}

// NewProgressWriter creates a new ProgressWriter.
func NewProgressWriter(out io.Writer, serviceName string, totalBytes int64) *ProgressWriter {
	return &ProgressWriter{
		out:         out,
		serviceName: serviceName,
		totalBytes:  totalBytes,
	}
}

func (pw *ProgressWriter) Write(p []byte) (int, error) {
	n := len(p)
	pw.mu.Lock()
	defer pw.mu.Unlock()

	pw.currentBytes += int64(n)

	now := time.Now()
	if now.Sub(pw.lastUpdate) > 40*time.Millisecond || (pw.totalBytes > 0 && pw.currentBytes >= pw.totalBytes) {
		pw.lastUpdate = now
		pw.render()
	}

	return n, nil
}

func (pw *ProgressWriter) render() {
	if pw.out == nil {
		return
	}

	if pw.totalBytes <= 0 {
		_, _ = fmt.Fprintf(pw.out, "\r[%s] Downloading: %s...      ", pw.serviceName, formatBytes(pw.currentBytes))
		return
	}

	percent := float64(pw.currentBytes) / float64(pw.totalBytes) * 100
	if percent > 100 {
		percent = 100
	}

	const barWidth = 25
	completed := int((percent / 100) * float64(barWidth))
	if completed > barWidth {
		completed = barWidth
	}

	bar := strings.Repeat("=", completed)
	if completed < barWidth {
		bar += ">" + strings.Repeat(" ", barWidth-completed-1)
	}

	_, _ = fmt.Fprintf(pw.out, "\r[%s] [%s] %5.1f%% (%s / %s)   ",
		pw.serviceName,
		bar,
		percent,
		formatBytes(pw.currentBytes),
		formatBytes(pw.totalBytes),
	)

	if pw.currentBytes >= pw.totalBytes {
		_, _ = fmt.Fprintln(pw.out)
	}
}

// Finish flushes a final newline if the progress bar hasn't completed yet.
func (pw *ProgressWriter) Finish() {
	pw.mu.Lock()
	defer pw.mu.Unlock()
	if pw.out != nil && (pw.totalBytes <= 0 || pw.currentBytes < pw.totalBytes) {
		_, _ = fmt.Fprintln(pw.out)
	}
}

// FormatBytes converts raw bytes to a human-readable string (KB, MB, GB).
func FormatBytes(b int64) string {
	return formatBytes(b)
}

func formatBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}
