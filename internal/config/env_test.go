package config_test

import (
	"testing"

	"github.com/pandusatrianura/NativeStackRunner/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildEnvEntries(t *testing.T) {
	t.Run("returns nil on nil config", func(t *testing.T) {
		assert.Nil(t, config.BuildEnvEntries(nil))
		assert.Empty(t, config.BuildEnvSlice(nil))
	})

	t.Run("generates complete environment entries for all declared services", func(t *testing.T) {
		cfg := &config.StackConfig{
			Name: "test-stack",
			Services: map[string]config.ServiceConfig{
				"postgres": {
					Name:     "postgres",
					Port:     5432,
					User:     "myuser",
					Password: "secretpassword",
					Database: "customdb",
				},
				"redis": {
					Name: "redis",
					Port: 6379,
				},
				"minio": {
					Name:      "minio",
					Port:      9000,
					AccessKey: "customaccess",
					SecretKey: "customsecret",
				},
				"mailpit": {
					Name:   "mailpit",
					Port:   1025,
					UIPort: 8025,
				},
			},
		}

		entries := config.BuildEnvEntries(cfg)
		require.NotEmpty(t, entries)

		envMap := make(map[string]string)
		for _, e := range entries {
			envMap[e.Key] = e.Value
		}

		// Postgres assertions
		assert.Equal(t, "postgres://myuser:secretpassword@localhost:5432/customdb?sslmode=disable", envMap["DATABASE_URL"])
		assert.Equal(t, "localhost", envMap["PGHOST"])
		assert.Equal(t, "5432", envMap["PGPORT"])
		assert.Equal(t, "myuser", envMap["PGUSER"])
		assert.Equal(t, "customdb", envMap["PGDATABASE"])
		assert.Equal(t, "secretpassword", envMap["PGPASSWORD"])

		// Redis assertions
		assert.Equal(t, "redis://localhost:6379", envMap["REDIS_URL"])
		assert.Equal(t, "localhost", envMap["REDIS_HOST"])
		assert.Equal(t, "6379", envMap["REDIS_PORT"])

		// MinIO assertions
		assert.Equal(t, "http://localhost:9000", envMap["S3_ENDPOINT"])
		assert.Equal(t, "customaccess", envMap["AWS_ACCESS_KEY_ID"])
		assert.Equal(t, "customsecret", envMap["AWS_SECRET_ACCESS_KEY"])
		assert.Equal(t, "http://localhost:9000", envMap["AWS_ENDPOINT_URL_S3"])
		assert.Equal(t, "us-east-1", envMap["AWS_REGION"])

		// Mailpit assertions
		assert.Equal(t, "localhost", envMap["SMTP_HOST"])
		assert.Equal(t, "1025", envMap["SMTP_PORT"])
		assert.Equal(t, "http://localhost:8025", envMap["MAILPIT_URL"])
		assert.Equal(t, "smtp://localhost:1025", envMap["MAILPIT_SMTP_URL"])

		// Test slice format
		slice := config.BuildEnvSlice(cfg)
		assert.Contains(t, slice, "DATABASE_URL=postgres://myuser:secretpassword@localhost:5432/customdb?sslmode=disable")
		assert.Contains(t, slice, "REDIS_URL=redis://localhost:6379")
		assert.Contains(t, slice, "S3_ENDPOINT=http://localhost:9000")
		assert.Contains(t, slice, "SMTP_PORT=1025")
	})
}
