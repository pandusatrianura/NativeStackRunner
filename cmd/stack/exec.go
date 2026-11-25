package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/pandusatrianura/NativeStackRunner/internal/binary"
	"github.com/pandusatrianura/NativeStackRunner/internal/config"
	"github.com/pandusatrianura/NativeStackRunner/internal/runner"
	"github.com/spf13/cobra"
)

var osExit = os.Exit

func newExecCmd() *cobra.Command {
	var (
		configFile string
		timeout    time.Duration
		quiet      bool
	)

	cmd := &cobra.Command{
		Use:   "exec [flags] -- <command> [args...]",
		Short: "Run a command with live stack dependencies and ephemeral teardown",
		Long: `Boot all stack dependencies, wait for them to be ready, inject connection
environment variables (DATABASE_URL, REDIS_URL, S3_ENDPOINT, etc.), run the specified
command, and automatically shut down all services when the command finishes.

The CLI exits with the exact same exit code returned by the inner command.

Examples:
  stack exec -- go test -v ./...
  stack exec -- npm test
  stack exec -- pytest tests/
  stack exec -c test-stack.yaml --quiet -- cargo test`,
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return errors.New("no command specified. Provide a command to run after '--', e.g. 'stack exec -- go test ./...'")
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			pidsDir := filepath.Join(projectDir, ".stack", "pids")
			activePIDs, err := runner.ListActivePIDs(pidsDir)
			if err == nil && len(activePIDs) > 0 {
				var runningServices []string
				for svc, pid := range activePIDs {
					if runner.IsProcessAlive(pid) {
						runningServices = append(runningServices, fmt.Sprintf("%s (PID %d)", svc, pid))
					}
				}
				if len(runningServices) > 0 {
					return fmt.Errorf("services are already running for this project: %s\nRun 'stack status' to inspect or 'stack down' to stop them before using 'stack exec'", strings.Join(runningServices, ", "))
				}
			}

			targetConfig := configFile
			if targetConfig != "" && !filepath.IsAbs(targetConfig) {
				targetConfig = filepath.Join(projectDir, targetConfig)
			} else if targetConfig == "" {
				candidates := []string{
					filepath.Join(projectDir, "stack.yaml"),
					filepath.Join(projectDir, "stack.yml"),
					filepath.Join(projectDir, "docker-compose.yml"),
					filepath.Join(projectDir, "docker-compose.yaml"),
					filepath.Join(projectDir, "compose.yaml"),
				}
				for _, c := range candidates {
					if _, err := os.Stat(c); err == nil {
						targetConfig = c
						break
					}
				}
			}

			if targetConfig == "" {
				return fmt.Errorf("no stack.yaml or docker-compose.yml found in %s", projectDir)
			}

			cfg, err := config.Load(targetConfig)
			if err != nil {
				return fmt.Errorf("loading configuration: %w", err)
			}

			slog.Info("configuration loaded for exec", "name", cfg.Name, "services", len(cfg.Services))

			out := cmd.OutOrStdout()
			var supOut io.Writer = out
			if quiet && !verbose {
				supOut = io.Discard
			}

			binMgr, err := binary.NewManager("", nil)
			if err != nil {
				return fmt.Errorf("initializing binary manager: %w", err)
			}
			if !quiet || verbose {
				binMgr.SetProgressOutput(out)
			}

			ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
			defer stop()

			supervisor := runner.NewSupervisor(cfg, projectDir, binMgr, nil, supOut)

			if !quiet {
				fmt.Fprintln(out, "Booting stack dependencies in background...")
			}

			stack, err := supervisor.StartAsync(ctx)
			if err != nil {
				return fmt.Errorf("booting dependencies: %w", err)
			}
			defer stack.Stop()

			if !quiet {
				fmt.Fprintln(out, "Waiting for services to become ready...")
			}

			if err := stack.WaitReady(ctx, timeout); err != nil {
				return fmt.Errorf("waiting for services to become ready: %w", err)
			}

			if !quiet {
				fmt.Fprintf(out, "Services ready. Executing: %s\n\n", strings.Join(args, " "))
			}

			childCmd := exec.CommandContext(ctx, args[0], args[1:]...)
			childCmd.Dir = projectDir
			childCmd.Env = append(os.Environ(), config.BuildEnvSlice(cfg)...)
			childCmd.Stdin = cmd.InOrStdin()
			childCmd.Stdout = cmd.OutOrStdout()
			childCmd.Stderr = cmd.ErrOrStderr()

			childErr := childCmd.Run()

			// Clean up stack before reporting exit
			stack.Stop()

			if childErr != nil {
				var exitErr *exec.ExitError
				if errors.As(childErr, &exitErr) {
					osExit(exitErr.ExitCode())
					return nil
				}
				return fmt.Errorf("executing %q: %w", args[0], childErr)
			}

			return nil
		},
	}

	cmd.Flags().StringVarP(&configFile, "config", "c", "", "Path to stack.yaml or docker-compose.yml")
	cmd.Flags().DurationVar(&timeout, "timeout", 30*time.Second, "Maximum duration to wait for all services to become ready")
	cmd.Flags().BoolVarP(&quiet, "quiet", "q", false, "Suppress supervisor and banner output, showing only command output")
	return cmd
}
