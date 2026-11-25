package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/pandusatrianura/NativeStackRunner/internal/config"
	"github.com/spf13/cobra"
)

func newEnvCmd() *cobra.Command {
	var (
		configFile string
		exportFmt  bool
		jsonFmt    bool
	)

	cmd := &cobra.Command{
		Use:   "env",
		Short: "Print environment variables and connection strings for declared services",
		Long: `Print ready-to-use environment variables (DATABASE_URL, REDIS_URL, S3_ENDPOINT, etc.)
for all services declared in stack.yaml or docker-compose.yml.

Examples:
  stack env                      # Print environment variables
  stack env --export             # Format with 'export KEY="VALUE"' for bash/zsh eval
  eval $(stack env --export)     # Load directly into current shell session
  stack env --json               # Format as JSON key-value map`,
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()

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
					filepath.Join(projectDir, "compose.yml"),
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

			entries := config.BuildEnvEntries(cfg)

			if jsonFmt {
				dataMap := make(map[string]string, len(entries))
				for _, e := range entries {
					dataMap[e.Key] = e.Value
				}
				encoder := json.NewEncoder(out)
				encoder.SetIndent("", "  ")
				return encoder.Encode(dataMap)
			}

			if exportFmt {
				for _, e := range entries {
					fmt.Fprintf(out, "export %s=%q\n", e.Key, e.Value)
				}
				return nil
			}

			// Group by service for clean terminal reading
			serviceGroups := make(map[string][]config.EnvEntry)
			for _, e := range entries {
				serviceGroups[e.Service] = append(serviceGroups[e.Service], e)
			}

			var services []string
			for svc := range serviceGroups {
				services = append(services, svc)
			}
			sort.Strings(services)

			fmt.Fprintln(out, "# Native Stack Runner Environment Variables")
			for _, svc := range services {
				fmt.Fprintf(out, "\n# %s\n", svc)
				for _, e := range serviceGroups[svc] {
					fmt.Fprintf(out, "%s=%q\n", e.Key, e.Value)
				}
			}

			return nil
		},
	}

	cmd.Flags().StringVarP(&configFile, "config", "c", "", "Path to stack.yaml or docker-compose.yml")
	cmd.Flags().BoolVarP(&exportFmt, "export", "e", false, "Output with export prefix for shell eval")
	cmd.Flags().BoolVarP(&jsonFmt, "json", "j", false, "Output as JSON key-value map")
	return cmd
}
