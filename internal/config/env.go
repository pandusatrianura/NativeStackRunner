package config

import (
	"fmt"
	"strconv"
)

// EnvEntry represents an environment variable generated from a service configuration.
type EnvEntry struct {
	Key     string `json:"key"`
	Value   string `json:"value"`
	Service string `json:"service"`
}

// BuildEnvEntries extracts ready-to-use connection environment variables from the declared services.
func BuildEnvEntries(cfg *StackConfig) []EnvEntry {
	if cfg == nil {
		return nil
	}

	var entries []EnvEntry

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
			EnvEntry{Service: "postgres", Key: "DATABASE_URL", Value: dbURL},
			EnvEntry{Service: "postgres", Key: "PGHOST", Value: "localhost"},
			EnvEntry{Service: "postgres", Key: "PGPORT", Value: strconv.Itoa(pg.Port)},
			EnvEntry{Service: "postgres", Key: "PGUSER", Value: user},
			EnvEntry{Service: "postgres", Key: "PGDATABASE", Value: db},
		)
		if pg.Password != "" {
			entries = append(entries, EnvEntry{Service: "postgres", Key: "PGPASSWORD", Value: pg.Password})
		}
	}

	if redis, ok := cfg.Services["redis"]; ok {
		redisURL := fmt.Sprintf("redis://localhost:%d", redis.Port)
		entries = append(entries,
			EnvEntry{Service: "redis", Key: "REDIS_URL", Value: redisURL},
			EnvEntry{Service: "redis", Key: "REDIS_HOST", Value: "localhost"},
			EnvEntry{Service: "redis", Key: "REDIS_PORT", Value: strconv.Itoa(redis.Port)},
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
			EnvEntry{Service: "minio", Key: "S3_ENDPOINT", Value: s3Endpoint},
			EnvEntry{Service: "minio", Key: "AWS_ACCESS_KEY_ID", Value: accessKey},
			EnvEntry{Service: "minio", Key: "AWS_SECRET_ACCESS_KEY", Value: secretKey},
			EnvEntry{Service: "minio", Key: "AWS_ENDPOINT_URL_S3", Value: s3Endpoint},
			EnvEntry{Service: "minio", Key: "AWS_REGION", Value: "us-east-1"},
		)
	}

	return entries
}

// BuildEnvSlice returns the environment variables as a slice of "KEY=VALUE" strings suitable for child processes.
func BuildEnvSlice(cfg *StackConfig) []string {
	entries := BuildEnvEntries(cfg)
	res := make([]string, len(entries))
	for i, e := range entries {
		res[i] = fmt.Sprintf("%s=%s", e.Key, e.Value)
	}
	return res
}
