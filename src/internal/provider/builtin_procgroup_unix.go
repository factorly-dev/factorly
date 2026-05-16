// Copyright 2026 Jordan Sherer <hi@jordansherer.com>
// SPDX-License-Identifier: gpl

//go:build !windows

package provider

import (
	"os/exec"
	"syscall"
)

// setShellProcessGroup puts the shell command in its own process group
// so SIGKILL (delivered by exec.CommandContext on ctx cancel) targets
// the entire group instead of just the parent shell. Without this,
// shell children like `sleep` survive cancellation.
//
// cmd.Cancel is set to send SIGKILL to -pid (the negative pid signals
// the whole process group on Unix).
func setShellProcessGroup(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Setpgid = true
	cmd.Cancel = func() error {
		// Negative pid sends to the whole process group.
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
}
