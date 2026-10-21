// Copyright 2026 Jordan Sherer <hi@jordansherer.com>
// SPDX-License-Identifier: gpl

package provider

import (
	"strings"
	"testing"
	"time"
)

func TestCLITimeoutReportsExplicitly(t *testing.T) {
	p := NewCLI(map[string]CLIToolDef{
		"slow": {Command: "sleep", Args: []string{"5"}, Timeout: 200 * time.Millisecond},
	})
	res, err := p.Execute("slow", nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if res.ExitCode != ExitCodeTimeout {
		t.Errorf("exit code = %d, want %d", res.ExitCode, ExitCodeTimeout)
	}
	if !strings.Contains(res.Error, "timed out after 200ms") {
		t.Errorf("error = %q, want timed-out message", res.Error)
	}
}

func TestCLINoTimeoutOutlivesDeadline(t *testing.T) {
	// Timeout is set but NoTimeout wins — the command must finish.
	p := NewCLI(map[string]CLIToolDef{
		"slow": {Command: "sleep", Args: []string{"0.4"}, Timeout: 100 * time.Millisecond, NoTimeout: true},
	})
	res, err := p.Execute("slow", nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if res.IsError() {
		t.Errorf("expected success, got exit %d error %q", res.ExitCode, res.Error)
	}
}

func TestCLIExitCodePreserved(t *testing.T) {
	p := NewCLI(map[string]CLIToolDef{
		"fail": {Command: "sh", Args: []string{"-c", "echo oops >&2; exit 3"}},
	})
	res, err := p.Execute("fail", nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if res.ExitCode != 3 {
		t.Errorf("exit code = %d, want 3", res.ExitCode)
	}
	if !strings.Contains(res.Error, "oops") {
		t.Errorf("stderr not preserved: %q", res.Error)
	}
}
