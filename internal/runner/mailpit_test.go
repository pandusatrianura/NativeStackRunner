package runner

import (
	"path/filepath"
	"testing"

	"github.com/pandusatrianura/NativeStackRunner/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildMailpitCmd(t *testing.T) {
	tempProject := t.TempDir()

	t.Run("default ports and arguments", func(t *testing.T) {
		svc := config.ServiceConfig{
			Name: "mailpit",
			Port: 1025,
		}

		spec, err := BuildMailpitCmd("mailpit.exe", svc, tempProject)
		require.NoError(t, err)

		assert.Equal(t, "mailpit", spec.Service)
		assert.Equal(t, "mailpit.exe", spec.BinPath)
		assert.Equal(t, tempProject, spec.WorkDir)

		expectedDB := filepath.Join(tempProject, ".stack", "data", "mailpit", "mailpit.db")
		expectedArgs := []string{
			"--smtp", "127.0.0.1:1025",
			"--listen", "127.0.0.1:8025",
			"--database", expectedDB,
		}
		assert.Equal(t, expectedArgs, spec.Args)
		assert.DirExists(t, filepath.Join(tempProject, ".stack", "data", "mailpit"))
	})

	t.Run("custom smtp and ui ports", func(t *testing.T) {
		svc := config.ServiceConfig{
			Name:   "mailpit",
			Port:   2525,
			UIPort: 9025,
			Env: map[string]string{
				"MP_MAX_MESSAGES": "500",
			},
		}

		spec, err := BuildMailpitCmd("/usr/local/bin/mailpit", svc, tempProject)
		require.NoError(t, err)

		expectedDB := filepath.Join(tempProject, ".stack", "data", "mailpit", "mailpit.db")
		expectedArgs := []string{
			"--smtp", "127.0.0.1:2525",
			"--listen", "127.0.0.1:9025",
			"--database", expectedDB,
		}
		assert.Equal(t, expectedArgs, spec.Args)
		assert.Contains(t, spec.Env, "MP_MAX_MESSAGES=500")
	})
}
