package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"

	"github.com/pandusatrianura/NativeStackRunner/internal/config"
	"github.com/spf13/cobra"
)

type envEntry struct {
	Key     string `json:"key"`
	Value   string `json:"value"`
	Service string `json:"service"`
}

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

			entries := buildEnvEntries(cfg)

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
			serviceGroups := make(map[string][]envEntry)
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

func buildEnvEntries(cfg *config.StackConfig) []envEntry {
	var entries []envEntry

	if pg, ok := cfg.Services["postgres"]; ok {
		user := pg.User
		if user == "" {
			user = "postgres"
		}
		db := pg.Database
		if db == "" {
			db = "postgres"
		}
		auth := user
		if pg.Password != "" {
			auth = fmt.Sprintf("%s:%s", user, pg.Password)
		}
		dbURL := fmt.Sprintf("postgres://%s@localhost:%d/%s?sslmode=disable", auth, pg.Port, db)

		entries = append(entries,
			envEntry{Service: "postgres", Key: "DATABASE_URL", Value: dbURL},
			envEntry{Service: "postgres", Key: "PGHOST", Value: "localhost"},
			envEntry{Service: "postgres", Key: "PGPORT", Value: strconv.Itoa(pg.Port)},
			envEntry{Service: "postgres", Key: "PGUSER", Value: user},
			envEntry{Service: "postgres", Key: "PGDATABASE", Value: db},
		)
		if pg.Password != "" {
			entries = append(entries, envEntry{Service: "postgres", Key: "PGPASSWORD", Value: pg.Password})
		}
	}

	if redis, ok := cfg.Services["redis"]; ok {
		redisURL := fmt.Sprintf("redis://localhost:%d", redis.Port)
		entries = append(entries,
			envEntry{Service: "redis", Key: "REDIS_URL", Value: redisURL},
			envEntry{Service: "redis", Key: "REDIS_HOST", Value: "localhost"},
			envEntry{Service: "redis", Key: "REDIS_PORT", Value: strconv.Itoa(redis.Port)},
		)
	}

	if minio, ok := cfg.Services["minio"]; ok {
		s3Endpoint := fmt.Sprintf("http://localhost:%d", minio.Port)
		accessKey := minio.AccessKey
		if accessKey == "" {
			accessKey = "minioadmin"
		}
		secretKey := minio.SecretKey
		if secretKey == "" {
			secretKey = "minioadmin"
		}
		entries = append(entries,
			envEntry{Service: "minio", Key: "S3_ENDPOINT", Value: s3Endpoint},
			envEntry{Service: "minio", Key: "AWS_ACCESS_KEY_ID", Value: accessKey},
			envEntry{Service: "minio", Key: "AWS_SECRET_ACCESS_KEY", Value: secretKey},
			envEntry{Service: "minio", Key: "AWS_ENDPOINT_URL_S3", Value: s3Endpoint},
			envEntry{Service: "minio", Key: "AWS_REGION", Value: "us-east-1"},
		)
	}

	return entries
}
