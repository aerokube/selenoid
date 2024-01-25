//go:build !windows
// +build !windows

package service

import (
	"os/exec"
	"syscall"
)

func stopProc(cmd *exec.Cmd) error {
	exitCode := cmd.Process.Signal(syscall.SIGINT)
	_ = cmd.Wait()
	return exitCode
}
