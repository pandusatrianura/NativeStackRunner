//go:build !windows

package runner

import (
	"os/exec"
	"syscall"
)

func setDetachedProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
	}
}
