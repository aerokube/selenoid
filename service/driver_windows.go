//go:build windows
// +build windows

package service

import (
	"os/exec"
)

func stopProc(cmd *exec.Cmd) error {
	error := cmd.Process.Kill()
	_ = cmd.Wait()
	return error
}
