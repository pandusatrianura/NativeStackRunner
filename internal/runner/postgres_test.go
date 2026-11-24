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

	t.Run("falls back to TCP wire protocol when createdb binary is not found", func(t *testing.T) {
		// Mock TCP server to receive fallback
		l, err := net.Listen("tcp", "127.0.0.1:0")
		require.NoError(t, err)
		defer l.Close()

		port := l.Addr().(*net.TCPAddr).Port

		go func() {
			conn, err := l.Accept()
			if err != nil {
				return
			}
			defer conn.Close()

			// Read StartupMessage
			var lenBuf [4]byte
			_, _ = conn.Read(lenBuf[:])
			startupLen := int(lenBuf[0])<<24 | int(lenBuf[1])<<16 | int(lenBuf[2])<<8 | int(lenBuf[3])
			if startupLen > 4 {
				rest := make([]byte, startupLen-4)
				_, _ = conn.Read(rest)
			}

			// Send AuthOk ('R', len 8, code 0) and ReadyForQuery ('Z', len 5, 'I')
			_, _ = conn.Write([]byte{'R', 0, 0, 0, 8, 0, 0, 0, 0})
			_, _ = conn.Write([]byte{'Z', 0, 0, 0, 5, 'I'})

			// Read Query ('Q')
			qType := make([]byte, 1)
			_, _ = conn.Read(qType)
			_, _ = conn.Read(lenBuf[:])
			qLen := int(lenBuf[0])<<24 | int(lenBuf[1])<<16 | int(lenBuf[2])<<8 | int(lenBuf[3])
			if qLen > 4 {
				qPayload := make([]byte, qLen-4)
				_, _ = conn.Read(qPayload)
			}

			// Send CommandComplete ('C') and ReadyForQuery ('Z')
			cmdComplete := "CREATE DATABASE\x00"
			respLen := 4 + len(cmdComplete)
			_, _ = conn.Write(append([]byte{'C', 0, 0, 0, byte(respLen)}, []byte(cmdComplete)...))
			_, _ = conn.Write([]byte{'Z', 0, 0, 0, 5, 'I'})
		}()

		mockRunner := func(ctx context.Context, name string, args, env []string, dir string) ([]byte, error) {
			return nil, errors.New("exec: \"createdb\": executable file not found in %PATH%")
		}

		err = runner.EnsurePostgresDatabase(ctx, mockRunner, "/bin/postgres", port, "postgres", "custom_db", "")
		require.NoError(t, err)
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

func TestCreatePostgresDatabaseViaTCP(t *testing.T) {
	t.Run("handles duplicate database 42P04 idempotently", func(t *testing.T) {
		l, err := net.Listen("tcp", "127.0.0.1:0")
		require.NoError(t, err)
		defer l.Close()

		port := l.Addr().(*net.TCPAddr).Port

		go func() {
			conn, err := l.Accept()
			if err != nil {
				return
			}
			defer conn.Close()

			// Consume StartupMessage
			var lenBuf [4]byte
			_, _ = conn.Read(lenBuf[:])
			startupLen := int(lenBuf[0])<<24 | int(lenBuf[1])<<16 | int(lenBuf[2])<<8 | int(lenBuf[3])
			if startupLen > 4 {
				rest := make([]byte, startupLen-4)
				_, _ = conn.Read(rest)
			}

			// Send AuthOk ('R') and ReadyForQuery ('Z')
			_, _ = conn.Write([]byte{'R', 0, 0, 0, 8, 0, 0, 0, 0})
			_, _ = conn.Write([]byte{'Z', 0, 0, 0, 5, 'I'})

			// Read Query ('Q')
			qType := make([]byte, 1)
			_, _ = conn.Read(qType)
			_, _ = conn.Read(lenBuf[:])
			qLen := int(lenBuf[0])<<24 | int(lenBuf[1])<<16 | int(lenBuf[2])<<8 | int(lenBuf[3])
			if qLen > 4 {
				qPayload := make([]byte, qLen-4)
				_, _ = conn.Read(qPayload)
			}

			// Send ErrorResponse ('E') with "ERROR: database \"custom_db\" already exists" and ReadyForQuery ('Z')
			errResp := "SERROR\x00C42P04\x00Mdatabase \"custom_db\" already exists\x00\x00"
			respLen := 4 + len(errResp)
			_, _ = conn.Write(append([]byte{'E', 0, 0, 0, byte(respLen)}, []byte(errResp)...))
			_, _ = conn.Write([]byte{'Z', 0, 0, 0, 5, 'I'})
		}()

		err = runner.CreatePostgresDatabaseViaTCP(context.Background(), "127.0.0.1", port, "postgres", "custom_db")
		require.NoError(t, err)
	})
}
