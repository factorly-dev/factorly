// Copyright 2026 Jordan Sherer <hi@jordansherer.com>
// SPDX-License-Identifier: gpl

//go:build windows

package provider

import "os/exec"

// setShellProcessGroup is a no-op on Windows. exec.CommandContext on
// Windows already terminates the process on ctx cancel; child-process
// orphaning that affects Unix `sh -c "sleep 5"` doesn't apply the same
// way to `cmd /C` semantics. Worth revisiting if real Windows usage
// surfaces stuck child processes.
func setShellProcessGroup(cmd *exec.Cmd) {}
