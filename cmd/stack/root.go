package main

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/spf13/cobra"
)

var (
	projectDir string
	verbose    bool
)

var rootCmd = &cobra.Command{
	Use:   "stack",
	Short: "Native Stack Runner (NSR) — Fast, containerless dependency manager",
	Long: `Native Stack Runner (NSR) downloads, provisions, and runs precompiled
backend dependencies (PostgreSQL, Redis, MinIO) locally in user-space
without Docker Desktop, VMs, or complex Nix setups.`,
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		level := slog.LevelInfo
		if verbose {
			level = slog.LevelDebug
		}
		logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level}))
		slog.SetDefault(logger)
	},
}

func init() {
	cwd, _ := os.Getwd()
	rootCmd.PersistentFlags().StringVarP(&projectDir, "dir", "d", cwd, "Target project directory containing stack.yaml")
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "Enable verbose debug logging")

	rootCmd.AddCommand(newUpCmd())
	rootCmd.AddCommand(newDownCmd())
	rootCmd.AddCommand(newStatusCmd())
	rootCmd.AddCommand(newCleanCmd())
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
