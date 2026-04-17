// Copyright 2026 Jordan Sherer <hi@jordansherer.com>
// SPDX-License-Identifier: gpl

package provider

import "time"

type Result struct {
	Output   string
	Error    string
	ExitCode int
	Duration time.Duration
}

func (r *Result) IsError() bool {
	return r.ExitCode != 0 || r.Error != ""
}

type Provider interface {
	Setup() error
	Execute(toolName string, params map[string]string) (*Result, error)
	Teardown() error
}
