// Copyright 2026 Jordan Sherer <hi@jordansherer.com>
// SPDX-License-Identifier: gpl

package code

import "time"

// Result is the value returned by factorly.Call inside a script. It
// intentionally mirrors provider.Result but is declared in this package
// so scripts can import "factorly" and reference factorly.Result without
// pulling the provider package into the interpreter's symbol set.
type Result struct {
	Output   string
	Error    string
	ExitCode int
	Duration time.Duration
}

// IsError reports whether the call failed (nonzero exit or non-empty error).
func (r *Result) IsError() bool {
	if r == nil {
		return true
	}
	return r.ExitCode != 0 || r.Error != ""
}
