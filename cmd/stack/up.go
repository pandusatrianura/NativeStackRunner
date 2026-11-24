package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/pandusatrianura/NativeStackRunner/internal/binary"
	"github.com/pandusatrianura/NativeStackRunner/internal/config"
	"github.com/pandusatrianura/NativeStackRunner/internal/runner"
	"github.com/spf13/cobra"
)

func newUpCmd() *cobra.Command {
	var configFile string

	cmd := &cobra.Command{
		Use:   "up",
		Short: "Start all dependency services declared in stack.yaml or docker-compose.yml",
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
					return fmt.Errorf("services are already running for this project: %s\nRun 'stack status' to inspect or 'stack down' to stop them first", strings.Join(runningServices, ", "))
				}
			}

			slog.Info("initializing native stack runner", "projectDir", projectDir)

			targetConfig := configFile
			if targetConfig != "" && !filepath.IsAbs(targetConfig) {
				targetConfig = filepath.Join(projectDir, targetConfig)
			} else if targetConfig == "" {
				// Auto discovery in projectDir
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

			cfg, err := config.Load(targetConfig)
			if err != nil {
				return fmt.Errorf("loading configuration: %w", err)
			}

			slog.Info("configuration loaded", "name", cfg.Name, "services", len(cfg.Services))

			out := cmd.OutOrStdout()
			binMgr, err := binary.NewManager("", nil)
			if err != nil {
				return fmt.Errorf("initializing binary manager: %w", err)
			}
			binMgr.SetProgressOutput(out)

			ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
			defer stop()

			supervisor := runner.NewSupervisor(cfg, projectDir, binMgr, nil, out)
			return supervisor.Start(ctx)
		},
	}

	cmd.Flags().StringVarP(&configFile, "config", "c", "", "Path to stack.yaml or docker-compose.yml")
	return cmd
}
