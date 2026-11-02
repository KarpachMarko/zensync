//go:build !linux

package app

import "os/exec"

func protectChild(cmd *exec.Cmd) {}
