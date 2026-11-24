package binary_test

import (
	"bytes"
	"testing"

	"github.com/pandusatrianura/NativeStackRunner/internal/binary"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFormatBytes(t *testing.T) {
	tests := []struct {
		input    int64
		expected string
	}{
		{500, "500 B"},
		{1024, "1.0 KB"},
		{1536, "1.5 KB"},
		{1048576, "1.0 MB"},
		{1073741824, "1.0 GB"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			assert.Equal(t, tt.expected, binary.FormatBytes(tt.input))
		})
	}
}

func TestProgressWriter_KnownContentLength(t *testing.T) {
	var buf bytes.Buffer
	totalSize := int64(1000)
	pw := binary.NewProgressWriter(&buf, "minio", totalSize)

	// Write half the content
	n, err := pw.Write(make([]byte, 500))
	require.NoError(t, err)
	assert.Equal(t, 500, n)

	// Write remainder
	n, err = pw.Write(make([]byte, 500))
	require.NoError(t, err)
	assert.Equal(t, 500, n)

	pw.Finish()

	output := buf.String()
	assert.Contains(t, output, "[minio]")
	assert.Contains(t, output, "%")
}

func TestProgressWriter_UnknownContentLength(t *testing.T) {
	var buf bytes.Buffer
	pw := binary.NewProgressWriter(&buf, "redis", -1)

	n, err := pw.Write(make([]byte, 4096))
	require.NoError(t, err)
	assert.Equal(t, 4096, n)

	pw.Finish()

	output := buf.String()
	assert.Contains(t, output, "[redis] Downloading:")
	assert.Contains(t, output, "4.0 KB")
}
