package runner

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// CmdRunner abstracts executing an external helper command for testing.
type CmdRunner func(ctx context.Context, name string, args []string, env []string, dir string) ([]byte, error)

// DefaultCmdRunner executes commands using standard OS exec.
func DefaultCmdRunner(ctx context.Context, name string, args []string, env []string, dir string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	if len(env) > 0 {
		cmd.Env = env
	}
	return cmd.CombinedOutput()
}

// WaitForTCPReady polls targetAddr until a TCP connection succeeds or timeout expires.
func WaitForTCPReady(ctx context.Context, targetAddr string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		if time.Now().After(deadline) {
			return fmt.Errorf("timeout waiting for %s to be ready", targetAddr)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		conn, err := net.DialTimeout("tcp", targetAddr, 100*time.Millisecond)
		if err == nil {
			_ = conn.Close()
			return nil
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// EnsurePostgresDatabase ensures the specified database exists on the running PostgreSQL instance.
func EnsurePostgresDatabase(ctx context.Context, runner CmdRunner, binPath string, port int, user, database, projectDir string) error {
	if database == "" || database == "postgres" {
		return nil
	}
	if runner == nil {
		runner = DefaultCmdRunner
	}

	createdbPath := filepath.Join(filepath.Dir(binPath), "createdb")
	if strings.HasSuffix(binPath, ".exe") {
		createdbPath += ".exe"
	}

	libDir := filepath.Join(filepath.Dir(filepath.Dir(binPath)), "lib")
	env := append(os.Environ(),
		fmt.Sprintf("LD_LIBRARY_PATH=%s:%s", libDir, os.Getenv("LD_LIBRARY_PATH")),
		fmt.Sprintf("DYLD_LIBRARY_PATH=%s:%s", libDir, os.Getenv("DYLD_LIBRARY_PATH")),
	)

	args := []string{
		"-h", "127.0.0.1",
		"-p", fmt.Sprintf("%d", port),
		"-U", user,
		database,
	}

	output, err := runner(ctx, createdbPath, args, env, projectDir)
	if err != nil {
		outStr := string(output)
		if strings.Contains(strings.ToLower(outStr), "already exists") {
			return nil
		}
		// If createdb binary does not exist on disk, fallback to pure TCP wire protocol
		if errors.Is(err, os.ErrNotExist) || strings.Contains(err.Error(), "not find the file") || strings.Contains(err.Error(), "executable file not found") {
			return CreatePostgresDatabaseViaTCP(ctx, "127.0.0.1", port, user, database)
		}
		return fmt.Errorf("creating postgres database %q: %w (output: %s)", database, err, outStr)
	}

	return nil
}

// CreatePostgresDatabaseViaTCP connects directly to PostgreSQL via standard v3 wire protocol and executes CREATE DATABASE.
func CreatePostgresDatabaseViaTCP(ctx context.Context, host string, port int, user, database string) error {
	addr := fmt.Sprintf("%s:%d", host, port)
	var d net.Dialer
	conn, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return fmt.Errorf("connecting to postgres on %s: %w", addr, err)
	}
	defer func() {
		_ = conn.Close()
	}()

	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))

	// 1. Send StartupMessage
	// Format: [int32 len][int32 proto 196608]["user\0<user>\0database\0postgres\0\0"]
	buf := new(bytes.Buffer)
	buf.Write([]byte{0, 0, 0, 0}) // placeholder for packet length

	var protoVer [4]byte
	binary.BigEndian.PutUint32(protoVer[:], 196608)
	buf.Write(protoVer[:])

	buf.WriteString("user\x00")
	buf.WriteString(user + "\x00")
	buf.WriteString("database\x00")
	buf.WriteString("postgres\x00\x00")

	msgBytes := buf.Bytes()
	binary.BigEndian.PutUint32(msgBytes[0:4], uint32(len(msgBytes)))

	if _, err := conn.Write(msgBytes); err != nil {
		return fmt.Errorf("sending startup message: %w", err)
	}

	// 2. Read server response until ReadyForQuery ('Z')
	reader := bufio.NewReader(conn)
	for {
		msgType, err := reader.ReadByte()
		if err != nil {
			return fmt.Errorf("reading startup response: %w", err)
		}
		var msgLenBuf [4]byte
		if _, err := io.ReadFull(reader, msgLenBuf[:]); err != nil {
			return fmt.Errorf("reading message length: %w", err)
		}
		msgLen := binary.BigEndian.Uint32(msgLenBuf[:])
		payload := make([]byte, msgLen-4)
		if _, err := io.ReadFull(reader, payload); err != nil {
			return fmt.Errorf("reading message payload: %w", err)
		}

		if msgType == 'E' { // ErrorResponse
			return fmt.Errorf("postgres error during startup: %s", string(payload))
		}
		if msgType == 'Z' { // ReadyForQuery
			break
		}
	}

	// 3. Send Query message: CREATE DATABASE "<database>"
	query := fmt.Sprintf("CREATE DATABASE %q;\x00", database)
	queryLen := 4 + len(query)

	qBuf := make([]byte, 1+queryLen)
	qBuf[0] = 'Q'
	binary.BigEndian.PutUint32(qBuf[1:5], uint32(queryLen))
	copy(qBuf[5:], query)

	if _, err := conn.Write(qBuf); err != nil {
		return fmt.Errorf("sending create database query: %w", err)
	}

	// 4. Read query response until ReadyForQuery ('Z')
	var queryErr error
	for {
		msgType, err := reader.ReadByte()
		if err != nil {
			return fmt.Errorf("reading query response: %w", err)
		}
		var msgLenBuf [4]byte
		if _, err := io.ReadFull(reader, msgLenBuf[:]); err != nil {
			return fmt.Errorf("reading query message length: %w", err)
		}
		msgLen := binary.BigEndian.Uint32(msgLenBuf[:])
		payload := make([]byte, msgLen-4)
		if _, err := io.ReadFull(reader, payload); err != nil {
			return fmt.Errorf("reading query payload: %w", err)
		}

		if msgType == 'E' { // ErrorResponse
			errStr := string(payload)
			// PostgreSQL error 42P04 is duplicate_database ("already exists")
			if strings.Contains(strings.ToLower(errStr), "already exists") || strings.Contains(errStr, "42P04") {
				queryErr = nil
			} else {
				queryErr = fmt.Errorf("postgres query error: %s", errStr)
			}
		}
		if msgType == 'Z' { // ReadyForQuery
			break
		}
	}

	// 5. Send Terminate message ('X' with length 4)
	_, _ = conn.Write([]byte{'X', 0, 0, 0, 4})

	return queryErr
}
