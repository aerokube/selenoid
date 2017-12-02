// +build windows

package service

import (
	"os/exec"
)

func stopProc(cmd *exec.Cmd) error {
	return cmd.Process.Kill()
}
