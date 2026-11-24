package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/pandusatrianura/NativeStackRunner/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseYAML(t *testing.T) {
	tests := []struct {
		name        string
		yamlData    string
		wantErr     bool
		errContains string
		validate    func(t *testing.T, cfg *config.StackConfig)
	}{
		{
			name: "valid full stack configuration",
			yamlData: `
version: "1"
name: test-app
services:
  postgres:
    version: "16"
    port: 5433
    database: testdb
    user: testuser
    password: secretpassword
  redis:
    version: "7"
    port: 6380
  minio:
    port: 9000
    console_port: 9001
    buckets:
      - uploads
`,
			wantErr: false,
			validate: func(t *testing.T, cfg *config.StackConfig) {
				assert.Equal(t, "test-app", cfg.Name)
				assert.Len(t, cfg.Services, 3)

				pg := cfg.Services["postgres"]
				assert.Equal(t, "postgres", pg.Name)
				assert.Equal(t, "16", pg.Version)
				assert.Equal(t, 5433, pg.Port)
				assert.Equal(t, "testdb", pg.Database)
				assert.Equal(t, "testuser", pg.User)
				assert.Equal(t, "secretpassword", pg.Password)

				redis := cfg.Services["redis"]
				assert.Equal(t, 6380, redis.Port)

				minio := cfg.Services["minio"]
				assert.Equal(t, 9000, minio.Port)
				assert.Equal(t, 9001, minio.ConsolePort)
				assert.Equal(t, "minioadmin", minio.AccessKey) // default
				assert.Equal(t, []string{"uploads"}, minio.Buckets)
			},
		},
		{
			name: "applies default ports and credentials",
			yamlData: `
services:
  postgres: {}
  redis: {}
  minio: {}
`,
			wantErr: false,
			validate: func(t *testing.T, cfg *config.StackConfig) {
				assert.Equal(t, "default-stack", cfg.Name)
				assert.Equal(t, config.DefaultPostgresPort, cfg.Services["postgres"].Port)
				assert.Equal(t, config.DefaultRedisPort, cfg.Services["redis"].Port)
				assert.Equal(t, config.DefaultMinIOPort, cfg.Services["minio"].Port)
			},
		},
		{
			name: "normalizes service aliases postgresql and dragonfly",
			yamlData: `
services:
  postgresql:
    port: 5433
  dragonfly:
    port: 6380
`,
			wantErr: false,
			validate: func(t *testing.T, cfg *config.StackConfig) {
				assert.Contains(t, cfg.Services, "postgres")
				assert.Contains(t, cfg.Services, "redis")
				assert.Equal(t, 5433, cfg.Services["postgres"].Port)
				assert.Equal(t, 6380, cfg.Services["redis"].Port)
			},
		},
		{
			name:        "empty services returns error",
			yamlData:    `version: "1"`,
			wantErr:     true,
			errContains: "no services defined",
		},
		{
			name: "invalid port range",
			yamlData: `
services:
  postgres:
    port: 70000
`,
			wantErr:     true,
			errContains: "out of valid range",
		},
		{
			name: "path traversal in service name is rejected",
			yamlData: `
services:
  "../../evil":
    port: 5432
`,
			wantErr:     true,
			errContains: "contains invalid characters",
		},
		{
			name:        "malformed yaml syntax",
			yamlData:    `:::invalid yaml`,
			wantErr:     true,
			errContains: "malformed YAML",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := config.ParseYAML([]byte(tt.yamlData))
			if tt.wantErr {
				require.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
				return
			}
			require.NoError(t, err)
			require.NotNil(t, cfg)
			if tt.validate != nil {
				tt.validate(t, cfg)
			}
		})
	}
}

func TestParseCompose(t *testing.T) {
	composeData := `
version: "3.8"
services:
  postgres:
    image: postgres:15-alpine
    ports:
      - "127.0.0.1:5433:5432"
    environment:
      POSTGRES_DB: app_development
      POSTGRES_USER: devuser
      POSTGRES_PASSWORD: devpassword
  redis:
    image: redis:7.0
    ports:
      - "0.0.0.0:6380:6379/tcp"
  web:
    image: node:18
    ports:
      - "3000:3000"
`

	cfg, err := config.ParseCompose([]byte(composeData))
	require.NoError(t, err)
	require.NotNil(t, cfg)

	// Web service should be skipped, postgres and redis recognized
	assert.Len(t, cfg.Services, 2)

	pg := cfg.Services["postgres"]
	assert.Equal(t, "15-alpine", pg.Version)
	assert.Equal(t, 5433, pg.Port) // Extracted from 127.0.0.1:5433:5432
	assert.Equal(t, "app_development", pg.Database)
	assert.Equal(t, "devuser", pg.User)
	assert.Equal(t, "devpassword", pg.Password)

	redis := cfg.Services["redis"]
	assert.Equal(t, "7.0", redis.Version)
	assert.Equal(t, 6380, redis.Port) // Extracted from 0.0.0.0:6380:6379/tcp
}

func TestLoad(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "stack.yaml")

	content := `
services:
  redis:
    port: 6379
`
	err := os.WriteFile(configPath, []byte(content), 0o600)
	require.NoError(t, err)

	cfg, err := config.Load(configPath)
	require.NoError(t, err)
	assert.Contains(t, cfg.Services, "redis")

	// Missing file
	_, err = config.Load(filepath.Join(tempDir, "nonexistent.yaml"))
	require.ErrorIs(t, err, config.ErrNotFound)
}
