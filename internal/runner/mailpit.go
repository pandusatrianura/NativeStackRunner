package runner

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/pandusatrianura/NativeStackRunner/internal/config"
)

// BuildMailpitCmd constructs the command specification for running Mailpit.
func BuildMailpitCmd(binPath string, svc config.ServiceConfig, projectDir string) (SpecCmd, error) {
	dataDir := filepath.Join(projectDir, ".stack", "data", "mailpit")
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return SpecCmd{}, fmt.Errorf("creating mailpit data dir %s: %w", dataDir, err)
	}

	uiPort := svc.UIPort
	if uiPort == 0 {
		uiPort = config.DefaultMailpitUIPort
	}
	port := svc.Port
	if port == 0 {
		port = config.DefaultMailpitPort
	}

	dbFile := filepath.Join(dataDir, "mailpit.db")
	spec := SpecCmd{
		Service: "mailpit",
		BinPath: binPath,
		WorkDir: projectDir,
		Args: []string{
			"--smtp-listen", fmt.Sprintf("127.0.0.1:%d", port),
			"--listen", fmt.Sprintf("127.0.0.1:%d", uiPort),
			"--db-file", dbFile,
		},
	}

	for k, v := range svc.Env {
		spec.Env = append(spec.Env, fmt.Sprintf("%s=%s", k, v))
	}

	return spec, nil
}
