package main

import (
	"fmt"
	"path/filepath"

	"github.com/pandusatrianura/NativeStackRunner/internal/runner"
	"github.com/spf13/cobra"
)

func newDownCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "down",
		Short: "Stop all running dependency services for this project",
		RunE: func(cmd *cobra.Command, args []string) error {
			pidsDir := filepath.Join(projectDir, ".stack", "pids")
			pids, err := runner.ListActivePIDs(pidsDir)
			if err != nil {
				return fmt.Errorf("listing active pids: %w", err)
			}

			if len(pids) == 0 {
				fmt.Println("No active services found for this project.")
				return nil
			}

			fmt.Println("Stopping services...")
			for service, pid := range pids {
				if runner.IsProcessAlive(pid) {
					if err := runner.StopProcess(pid); err != nil {
						fmt.Printf("  - Failed to stop %s (PID %d): %v\n", service, pid, err)
					} else {
						fmt.Printf("  - Stopped %s (PID %d)\n", service, pid)
					}
				}
				_ = runner.RemovePID(pidsDir, service)
			}
			fmt.Println("Done.")
			return nil
		},
	}
}
