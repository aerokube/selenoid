// +build !windows

package service

import (
	"os/exec"
	"syscall"
)

func stopProc(cmd *exec.Cmd) error {
	return cmd.Process.Signal(syscall.SIGINT)
}
