//go:build linux

package app

import (
	"os/exec"
	"syscall"
)

// Do not leave a downloader writing staging after its owning service is killed.
func protectChild(cmd *exec.Cmd) { cmd.SysProcAttr = &syscall.SysProcAttr{Pdeathsig: syscall.SIGKILL} }
