package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

const defaultStackYAML = `# Native Stack Runner (NSR) Configuration
# Documentation: https://github.com/pandusatrianura/NativeStackRunner
version: "1"
name: "local-stack"

services:
  postgres:
    version: "16"
    port: 5432
    database: "postgres"
    user: "postgres"
    password: "password"

  redis:
    version: "7"
    port: 6379

  minio:
    version: "latest"
    port: 9000
    console_port: 9001
    access_key: "minioadmin"
    secret_key: "minioadmin"
    buckets:
      - "uploads"
`

func newInitCmd() *cobra.Command {
	var force bool

	cmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize a new stack.yaml template in the project directory",
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			targetPath := filepath.Join(projectDir, "stack.yaml")

			if _, err := os.Stat(targetPath); err == nil && !force {
				return fmt.Errorf("configuration file %s already exists; use --force to overwrite", targetPath)
			} else if err != nil && !errors.Is(err, os.ErrNotExist) {
				return fmt.Errorf("checking target path: %w", err)
			}

			if err := os.WriteFile(targetPath, []byte(defaultStackYAML), 0o644); err != nil {
				return fmt.Errorf("writing configuration template: %w", err)
			}

			fmt.Fprintf(out, "Initialized %s with PostgreSQL, Redis, and MinIO.\nRun 'stack up' to start the stack.\n", targetPath)
			return nil
		},
	}

	cmd.Flags().BoolVarP(&force, "force", "f", false, "Overwrite existing stack.yaml if present")
	return cmd
}
