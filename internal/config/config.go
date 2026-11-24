package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

var (
	ErrNotFound          = errors.New("configuration file not found")
	ErrInvalidConfig     = errors.New("invalid configuration")
	ErrUnsupportedFormat = errors.New("unsupported configuration format")
)

// Default service ports
const (
	DefaultPostgresPort = 5432
	DefaultRedisPort    = 6379
	DefaultMinIOPort    = 9000
)

// ServiceConfig defines configuration for an individual service dependency.
type ServiceConfig struct {
	Name        string            `yaml:"-"`
	Version     string            `yaml:"version"`
	Port        int               `yaml:"port"`
	Database    string            `yaml:"database,omitempty"`
	User        string            `yaml:"user,omitempty"`
	Password    string            `yaml:"password,omitempty"`
	Env         map[string]string `yaml:"env,omitempty"`
	ConsolePort int               `yaml:"console_port,omitempty"`
	AccessKey   string            `yaml:"access_key,omitempty"`
	SecretKey   string            `yaml:"secret_key,omitempty"`
	Buckets     []string          `yaml:"buckets,omitempty"`
}

// StackConfig represents the top-level configuration of Native Stack Runner.
type StackConfig struct {
	Version  string                   `yaml:"version"`
	Name     string                   `yaml:"name"`
	Services map[string]ServiceConfig `yaml:"services"`
}

// Load loads configuration from the given path.
// If path is empty, it attempts auto-discovery in the current directory:
// 1. stack.yaml / stack.yml
// 2. docker-compose.yml / compose.yaml
func Load(path string) (*StackConfig, error) {
	resolvedPath, err := resolveConfigPath(path)
	if err != nil {
		return nil, fmt.Errorf("resolving config path: %w", err)
	}

	data, err := os.ReadFile(resolvedPath)
	if err != nil {
		return nil, fmt.Errorf("reading config file %s: %w", resolvedPath, err)
	}

	baseName := strings.ToLower(filepath.Base(resolvedPath))
	if strings.Contains(baseName, "compose") {
		return ParseCompose(data)
	}

	return ParseYAML(data)
}

func resolveConfigPath(path string) (string, error) {
	if path != "" {
		if _, err := os.Stat(path); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return "", fmt.Errorf("%w: %s", ErrNotFound, path)
			}
			return "", err
		}
		return path, nil
	}

	candidates := []string{
		"stack.yaml",
		"stack.yml",
		"docker-compose.yml",
		"docker-compose.yaml",
		"compose.yaml",
		"compose.yml",
	}

	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c, nil
		}
	}

	return "", fmt.Errorf("%w: checked %s", ErrNotFound, strings.Join(candidates, ", "))
}

// ParseYAML parses raw YAML bytes conforming to the native stack.yaml schema.
func ParseYAML(data []byte) (*StackConfig, error) {
	var cfg StackConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("%w: malformed YAML: %w", ErrInvalidConfig, err)
	}

	if len(cfg.Services) == 0 {
		return nil, fmt.Errorf("%w: no services defined", ErrInvalidConfig)
	}

	if cfg.Name == "" {
		cfg.Name = "default-stack"
	}

	normalizedServices := make(map[string]ServiceConfig, len(cfg.Services))
	for name, svc := range cfg.Services {
		svc.Name = normalizeServiceName(name)
		if svc.Name == "" {
			return nil, fmt.Errorf("%w: service name cannot be empty", ErrInvalidConfig)
		}

		applyDefaultServiceSettings(&svc)

		if err := validateService(&svc); err != nil {
			return nil, fmt.Errorf("validating service %q: %w", svc.Name, err)
		}

		normalizedServices[svc.Name] = svc
	}
	cfg.Services = normalizedServices

	return &cfg, nil
}

// normalizeServiceName maps common aliases to canonical service identifiers.
func normalizeServiceName(name string) string {
	clean := strings.ToLower(strings.TrimSpace(name))
	switch clean {
	case "postgresql", "pg":
		return "postgres"
	case "dragonfly":
		return "redis"
	default:
		return clean
	}
}

// rawCompose defines a minimal docker-compose struct for fallback parsing.
type rawCompose struct {
	Version  string                            `yaml:"version"`
	Services map[string]rawComposeService      `yaml:"services"`
}

type rawComposeService struct {
	Image       string                 `yaml:"image"`
	Ports       []string               `yaml:"ports"`
	Environment yaml.Node              `yaml:"environment"`
}

// ParseCompose parses a subset of docker-compose.yml and converts recognized services (postgres, redis, minio)
// into a native StackConfig.
func ParseCompose(data []byte) (*StackConfig, error) {
	var compose rawCompose
	if err := yaml.Unmarshal(data, &compose); err != nil {
		return nil, fmt.Errorf("%w: malformed compose YAML: %w", ErrInvalidConfig, err)
	}

	if len(compose.Services) == 0 {
		return nil, fmt.Errorf("%w: no services defined in compose file", ErrInvalidConfig)
	}

	cfg := &StackConfig{
		Version:  "1",
		Name:     "compose-fallback-stack",
		Services: make(map[string]ServiceConfig),
	}

	for name, rawSvc := range compose.Services {
		normalizedName := strings.ToLower(name)
		serviceType, version := extractServiceAndVersion(normalizedName, rawSvc.Image)

		if serviceType == "" {
			// Skip services not recognized as native dependencies
			continue
		}

		svc := ServiceConfig{
			Name:    serviceType,
			Version: version,
			Env:     extractEnvMap(&rawSvc.Environment),
		}

		// Parse port mapping (e.g. "5432:5432" or "5432")
		for _, portMapping := range rawSvc.Ports {
			if hostPort := extractHostPort(portMapping); hostPort > 0 {
				svc.Port = hostPort
				break
			}
		}

		// Extract known credentials from environment
		if serviceType == "postgres" {
			if db, ok := svc.Env["POSTGRES_DB"]; ok {
				svc.Database = db
			}
			if user, ok := svc.Env["POSTGRES_USER"]; ok {
				svc.User = user
			}
			if pass, ok := svc.Env["POSTGRES_PASSWORD"]; ok {
				svc.Password = pass
			}
		} else if serviceType == "minio" {
			if user, ok := svc.Env["MINIO_ROOT_USER"]; ok {
				svc.AccessKey = user
			}
			if pass, ok := svc.Env["MINIO_ROOT_PASSWORD"]; ok {
				svc.SecretKey = pass
			}
		}

		applyDefaultServiceSettings(&svc)
		if err := validateService(&svc); err != nil {
			return nil, fmt.Errorf("validating compose service %q: %w", svc.Name, err)
		}

		cfg.Services[serviceType] = svc
	}

	if len(cfg.Services) == 0 {
		return nil, fmt.Errorf("%w: no supported database or storage services found in compose file", ErrInvalidConfig)
	}

	return cfg, nil
}

func extractServiceAndVersion(name, image string) (string, string) {
	target := name
	if image != "" {
		target = image
	}

	parts := strings.Split(target, "/")
	imageName := parts[len(parts)-1]
	tagParts := strings.Split(imageName, ":")

	base := strings.ToLower(tagParts[0])
	version := "latest"
	if len(tagParts) > 1 && tagParts[1] != "" {
		version = tagParts[1]
	}

	switch {
	case strings.Contains(base, "postgres"):
		return "postgres", version
	case strings.Contains(base, "redis") || strings.Contains(base, "dragonfly"):
		return "redis", version
	case strings.Contains(base, "minio"):
		return "minio", version
	default:
		return "", ""
	}
}

func extractHostPort(portStr string) int {
	clean := strings.TrimSpace(portStr)
	clean = strings.Trim(clean, `"'`)
	// Strip protocol suffix e.g. /tcp, /udp
	if slashIdx := strings.Index(clean, "/"); slashIdx != -1 {
		clean = clean[:slashIdx]
	}

	parts := strings.Split(clean, ":")
	var p string
	if len(parts) >= 2 {
		// Handles both "5432:5432" and "127.0.0.1:5432:5432"
		p = parts[len(parts)-2]
	} else if len(parts) == 1 {
		p = parts[0]
	}
	val, err := strconv.Atoi(strings.TrimSpace(p))
	if err != nil {
		return 0
	}
	return val
}

func extractEnvMap(node *yaml.Node) map[string]string {
	result := make(map[string]string)
	if node == nil || node.Kind == 0 {
		return result
	}

	// Case 1: Mapping (key: value)
	if node.Kind == yaml.MappingNode {
		for i := 0; i < len(node.Content); i += 2 {
			k := node.Content[i].Value
			v := node.Content[i+1].Value
			result[k] = v
		}
	} else if node.Kind == yaml.SequenceNode {
		// Case 2: Sequence ("KEY=VALUE")
		for _, item := range node.Content {
			parts := strings.SplitN(item.Value, "=", 2)
			if len(parts) == 2 {
				result[parts[0]] = parts[1]
			}
		}
	}
	return result
}

func applyDefaultServiceSettings(svc *ServiceConfig) {
	if svc.Version == "" {
		svc.Version = "latest"
	}

	if svc.Port == 0 {
		switch svc.Name {
		case "postgres":
			svc.Port = DefaultPostgresPort
		case "redis":
			svc.Port = DefaultRedisPort
		case "minio":
			svc.Port = DefaultMinIOPort
		}
	}

	if svc.Name == "postgres" {
		if svc.User == "" {
			svc.User = "postgres"
		}
		if svc.Database == "" {
			svc.Database = "postgres"
		}
	} else if svc.Name == "minio" {
		if svc.ConsolePort == 0 {
			svc.ConsolePort = svc.Port + 1
		}
		if svc.AccessKey == "" {
			svc.AccessKey = "minioadmin"
		}
		if svc.SecretKey == "" {
			svc.SecretKey = "minioadmin"
		}
	}
}

func validateService(svc *ServiceConfig) error {
	if svc.Name == "" {
		return fmt.Errorf("%w: service name cannot be empty", ErrInvalidConfig)
	}

	for _, ch := range svc.Name {
		if !((ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') || ch == '_' || ch == '-') {
			return fmt.Errorf("%w: service name %q contains invalid characters (must be alphanumeric, hyphen, or underscore)", ErrInvalidConfig, svc.Name)
		}
	}

	if svc.Port < 1 || svc.Port > 65535 {
		return fmt.Errorf("%w: port %d out of valid range (1-65535)", ErrInvalidConfig, svc.Port)
	}

	if svc.ConsolePort != 0 && (svc.ConsolePort < 1 || svc.ConsolePort > 65535) {
		return fmt.Errorf("%w: console port %d out of valid range (1-65535)", ErrInvalidConfig, svc.ConsolePort)
	}

	return nil
}
