package runner_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/pandusatrianura/NativeStackRunner/internal/runner"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSignS3Request(t *testing.T) {
	req, err := http.NewRequest(http.MethodPut, "http://127.0.0.1:9000/mybucket", nil)
	require.NoError(t, err)

	fixedTime := time.Date(2026, 11, 24, 12, 0, 0, 0, time.UTC)
	runner.SignS3Request(req, "minioadmin", "minioadmin", "us-east-1", fixedTime)

	auth := req.Header.Get("Authorization")
	assert.True(t, strings.HasPrefix(auth, "AWS4-HMAC-SHA256 Credential=minioadmin/20261124/us-east-1/s3/aws4_request"))
	assert.Contains(t, auth, "SignedHeaders=host;x-amz-content-sha256;x-amz-date")
	assert.Contains(t, auth, "Signature=")
	assert.Equal(t, "20261124T120000Z", req.Header.Get("x-amz-date"))
	assert.Equal(t, "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855", req.Header.Get("x-amz-content-sha256"))
}

func TestEnsureMinIOBucket(t *testing.T) {
	ctx := context.Background()

	t.Run("skips when bucket name is empty", func(t *testing.T) {
		err := runner.EnsureMinIOBucket(ctx, nil, "http://localhost:9000", "admin", "secret", "")
		require.NoError(t, err)
	})

	t.Run("succeeds on HTTP 200 OK", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, http.MethodPut, r.Method)
			assert.Equal(t, "/uploads", r.URL.Path)
			assert.NotEmpty(t, r.Header.Get("Authorization"))
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		err := runner.EnsureMinIOBucket(ctx, server.Client(), server.URL, "admin", "secret", "uploads")
		require.NoError(t, err)
	})

	t.Run("succeeds on HTTP 409 Conflict (BucketAlreadyOwnedByYou)", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusConflict)
			w.Write([]byte(`<Error><Code>BucketAlreadyOwnedByYou</Code></Error>`))
		}))
		defer server.Close()

		err := runner.EnsureMinIOBucket(ctx, server.Client(), server.URL, "admin", "secret", "uploads")
		require.NoError(t, err, "existing bucket must be treated idempotently")
	})

	t.Run("fails and returns error on HTTP 403 Forbidden", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusForbidden)
			w.Write([]byte(`<Error><Code>AccessDenied</Code></Error>`))
		}))
		defer server.Close()

		err := runner.EnsureMinIOBucket(ctx, server.Client(), server.URL, "admin", "secret", "uploads")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "returned HTTP 403")
		assert.Contains(t, err.Error(), "AccessDenied")
	})
}
