// +build !windows

package service

import (
	"os/exec"
	"syscall"
)

func stopProc(cmd *exec.Cmd) error {
		exitCode := cmd.Process.Signal(syscall.SIGINT)
		cmd.Wait()
		return exitCode
}
