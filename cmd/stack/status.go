package main

import (
	"fmt"
	"path/filepath"
	"text/tabwriter"

	"github.com/fatih/color"
	"github.com/pandusatrianura/NativeStackRunner/internal/runner"
	"github.com/spf13/cobra"
)

func newStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Display status of all project dependency services",
		RunE: func(cmd *cobra.Command, args []string) error {
			pidsDir := filepath.Join(projectDir, ".stack", "pids")
			pids, err := runner.ListActivePIDs(pidsDir)
			if err != nil {
				return fmt.Errorf("reading service pids: %w", err)
			}

			w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 3, ' ', 0)
			fmt.Fprintln(w, "SERVICE\tPID\tSTATUS")
			fmt.Fprintln(w, "-------\t---\t------")

			green := color.New(color.FgGreen).SprintFunc()
			red := color.New(color.FgRed).SprintFunc()

			if len(pids) == 0 {
				fmt.Fprintln(w, "(none)\t-\tNo active services")
			} else {
				for service, pid := range pids {
					status := red("STOPPED (orphaned PID)")
					if runner.IsProcessAlive(pid) {
						status = green("RUNNING")
					}
					fmt.Fprintf(w, "%s\t%d\t%s\n", service, pid, status)
				}
			}
			return w.Flush()
		},
	}
}
