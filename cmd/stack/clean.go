package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/pandusatrianura/NativeStackRunner/internal/runner"
	"github.com/spf13/cobra"
)

func newCleanCmd() *cobra.Command {
	var force bool

	cmd := &cobra.Command{
		Use:   "clean",
		Short: "Delete local project data directory (.stack/data) for a clean database state",
		RunE: func(cmd *cobra.Command, args []string) error {
			pidsDir := filepath.Join(projectDir, ".stack", "pids")
			activePIDs, err := runner.ListActivePIDs(pidsDir)
			if err == nil {
				for svc, pid := range activePIDs {
					if runner.IsProcessAlive(pid) {
						return fmt.Errorf("cannot clean: service %q (PID %d) is still running; run 'stack down' first", svc, pid)
					}
				}
			}

			dataDir := filepath.Join(projectDir, ".stack", "data")
			_, dataErr := os.Stat(dataDir)
			_, pidsErr := os.Stat(pidsDir)
			if os.IsNotExist(dataErr) && os.IsNotExist(pidsErr) {
				fmt.Println("No project data or runtime directory found to clean.")
				return nil
			}

			if !force {
				fmt.Print("Are you sure you want to delete local project database data (.stack/data)? [y/N]: ")
				reader := bufio.NewReader(os.Stdin)
				input, err := reader.ReadString('\n')
				if err != nil {
					return fmt.Errorf("reading confirmation input: %w", err)
				}
				input = strings.TrimSpace(strings.ToLower(input))
				if input != "y" && input != "yes" {
					fmt.Println("Clean operation canceled.")
					return nil
				}
			}

			if !os.IsNotExist(dataErr) {
				if err := os.RemoveAll(dataDir); err != nil {
					return fmt.Errorf("removing data directory %s: %w", dataDir, err)
				}
			}

			if !os.IsNotExist(pidsErr) {
				_ = os.RemoveAll(pidsDir)
			}

			fmt.Println("Project data and runtime files (.stack/data, .stack/pids) removed successfully.")
			return nil
		},
	}

	cmd.Flags().BoolVarP(&force, "force", "f", false, "Force clean without interactive confirmation")
	return cmd
}
