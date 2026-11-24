package runner_test

import (
	"context"
	"errors"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/pandusatrianura/NativeStackRunner/internal/runner"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEnsurePostgresDatabase(t *testing.T) {
	ctx := context.Background()

	t.Run("skips when database is default postgres or empty", func(t *testing.T) {
		var called bool
		mockRunner := func(ctx context.Context, name string, args, env []string, dir string) ([]byte, error) {
			called = true
			return nil, nil
		}

		err := runner.EnsurePostgresDatabase(ctx, mockRunner, "/bin/postgres", 5432, "postgres", "postgres", "")
		require.NoError(t, err)
		assert.False(t, called)

		err = runner.EnsurePostgresDatabase(ctx, mockRunner, "/bin/postgres", 5432, "postgres", "", "")
		require.NoError(t, err)
		assert.False(t, called)
	})

	t.Run("successfully executes createdb with correct arguments and env", func(t *testing.T) {
		var recordedArgs []string
		var recordedEnv []string
		mockRunner := func(ctx context.Context, name string, args, env []string, dir string) ([]byte, error) {
			recordedArgs = args
			recordedEnv = env
			return []byte("CREATE DATABASE"), nil
		}

		err := runner.EnsurePostgresDatabase(ctx, mockRunner, "/opt/postgres/bin/postgres", 5433, "myuser", "custom_db", "/my/proj")
		require.NoError(t, err)
		assert.Contains(t, recordedArgs, "-h")
		assert.Contains(t, recordedArgs, "127.0.0.1")
		assert.Contains(t, recordedArgs, "-p")
		assert.Contains(t, recordedArgs, "5433")
		assert.Contains(t, recordedArgs, "-U")
		assert.Contains(t, recordedArgs, "myuser")
		assert.Contains(t, recordedArgs, "custom_db")

		var hasLD bool
		for _, e := range recordedEnv {
			if strings.HasPrefix(e, "LD_LIBRARY_PATH=") {
				hasLD = true
			}
		}
		assert.True(t, hasLD, "expected LD_LIBRARY_PATH in env")
	})

	t.Run("idempotent when database already exists", func(t *testing.T) {
		mockRunner := func(ctx context.Context, name string, args, env []string, dir string) ([]byte, error) {
			return []byte(`createdb: error: database creation failed: ERROR: database "custom_db" already exists`), errors.New("exit status 1")
		}

		err := runner.EnsurePostgresDatabase(ctx, mockRunner, "/bin/postgres", 5432, "postgres", "custom_db", "")
		require.NoError(t, err, "already exists should be treated as success")
	})

	t.Run("returns wrapped error on actual failure", func(t *testing.T) {
		mockRunner := func(ctx context.Context, name string, args, env []string, dir string) ([]byte, error) {
			return []byte("FATAL: password authentication failed"), errors.New("exit status 1")
		}

		err := runner.EnsurePostgresDatabase(ctx, mockRunner, "/bin/postgres", 5432, "postgres", "custom_db", "")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "creating postgres database \"custom_db\"")
	})
}

func TestWaitForTCPReady(t *testing.T) {
	t.Run("succeeds when port opens", func(t *testing.T) {
		l, err := net.Listen("tcp", "127.0.0.1:0")
		require.NoError(t, err)
		defer l.Close()

		addr := l.Addr().String()
		err = runner.WaitForTCPReady(context.Background(), addr, 1*time.Second)
		require.NoError(t, err)
	})

	t.Run("fails on timeout when port closed", func(t *testing.T) {
		err := runner.WaitForTCPReady(context.Background(), "127.0.0.1:1", 200*time.Millisecond)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "timeout waiting for")
	})
}
