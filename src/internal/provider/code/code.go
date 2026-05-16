// Copyright 2026 Jordan Sherer <hi@jordansherer.com>
// SPDX-License-Identifier: gpl

// Package code is a Factorly tool provider that runs inline Go scripts
// inside a yaegi interpreter. Scripts can call any other registered
// Factorly tool via factorly.Call(name, params), which routes back
// through the proxy and goes through the same shadow / vault / audit
// machinery as a top-level call.
//
// The provider holds an executor interface satisfied by the proxy. This
// mirrors WorkflowProvider and avoids an import cycle between provider
// and proxy.
package code

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/factorly-dev/factorly/internal/provider"
)

// DefaultMaxCalls is the cap on inner factorly.Call invocations per
// script execution when the tool's max_calls field is unset.
const DefaultMaxCalls = 100

// ToolExecutor is the proxy-shaped interface scripts re-enter to call
// other tools. The proxy.Proxy type satisfies it. Defined here to keep
// this package free of an import cycle with proxy.
type ToolExecutor interface {
	ExecuteWithContext(ctx context.Context, toolName string, params map[string]string, iface string) (*provider.Result, error)
}

// Provider implements provider.Provider for tools whose work is a Go
// script. One Provider per Factorly process; it holds the registered
// scripts and the executor used for inner calls.
type Provider struct {
	executor ToolExecutor
	verbose  bool

	mu      sync.RWMutex
	scripts map[string]*entry
}

// entry is one registered code tool's compiled state.
type entry struct {
	src      string
	maxCalls int
}

// NewProvider creates a code provider wired to the given executor.
func NewProvider(exec ToolExecutor, verbose bool) *Provider {
	return &Provider{
		executor: exec,
		verbose:  verbose,
		scripts:  make(map[string]*entry),
	}
}

// Setup is a no-op; scripts are validated at RegisterCode time.
func (p *Provider) Setup() error { return nil }

// Teardown is a no-op.
func (p *Provider) Teardown() error { return nil }

// RegisterCode validates the script and stashes it for later execution.
// Validation runs yaegi against the source so syntax errors, missing
// `Run`, wrong signature, and denied imports all surface here — before
// any agent tries to invoke the tool. Callers should log a warning and
// skip the tool when this returns an error rather than fail config load.
func (p *Provider) RegisterCode(name, src string, maxCalls int) error {
	if err := validateScript(src); err != nil {
		return err
	}
	if maxCalls <= 0 {
		maxCalls = DefaultMaxCalls
	}
	p.mu.Lock()
	p.scripts[name] = &entry{src: src, maxCalls: maxCalls}
	p.mu.Unlock()
	return nil
}

// RemoveCode unregisters a previously-registered script. Used by the UI
// when a tool is deleted or renamed.
func (p *Provider) RemoveCode(name string) {
	p.mu.Lock()
	delete(p.scripts, name)
	p.mu.Unlock()
}

// Execute satisfies provider.Provider but doesn't carry a context; it
// delegates to ExecuteWithContext with a background context. The proxy
// uses ExecuteWithContext directly for real calls.
func (p *Provider) Execute(toolName string, params map[string]string) (*provider.Result, error) {
	return p.ExecuteWithContext(context.Background(), toolName, params)
}

// ExecuteWithContext runs the registered script for toolName. Wall-clock
// timeout, max_calls budget, panic recovery, and result marshalling all
// happen here. Returns (*Result, nil) for every outcome a script can
// produce — including errors — because the proxy treats provider-level
// errors as infrastructure failures, not script failures.
func (p *Provider) ExecuteWithContext(ctx context.Context, toolName string, params map[string]string) (*provider.Result, error) {
	p.mu.RLock()
	e, ok := p.scripts[toolName]
	p.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("code provider: tool %q not registered", toolName)
	}

	start := time.Now()

	// Per-execution call counter. Wrapping captures e.maxCalls so each
	// script gets its own budget without sharing state across runs.
	var (
		callCount int
		callMu    sync.Mutex
	)
	call := func(name string, callParams map[string]string) (*Result, error) {
		callMu.Lock()
		callCount++
		current := callCount
		callMu.Unlock()
		if current > e.maxCalls {
			return nil, fmt.Errorf("script exceeded max_calls (%d) — increase max_calls on the tool definition or reduce the script's call volume", e.maxCalls)
		}
		res, err := p.executor.ExecuteWithContext(ctx, name, callParams, "code")
		if err != nil {
			return nil, err
		}
		return &Result{
			Output:   res.Output,
			Error:    res.Error,
			ExitCode: res.ExitCode,
			Duration: res.Duration,
		}, nil
	}

	val, runErr := runScript(ctx, e.src, params, call)
	duration := time.Since(start)

	// Map (val, runErr) → provider.Result per the documented contract.
	out := &provider.Result{Duration: duration}

	var panicE panicErr
	switch {
	case errors.As(runErr, &panicE):
		out.Error = fmt.Sprintf("panic: %v", panicE.value)
		out.ExitCode = 2
	case runErr != nil:
		// Distinguish a yaegi-level compile/lookup error (which we treat
		// as a script error too — ExitCode 1) from a user-returned err.
		// Both surface to the caller identically, so the simpler shape
		// wins.
		out.Error = runErr.Error()
		out.ExitCode = 1
	default:
		body, marshalErr := marshalResultOutput(val)
		if marshalErr != nil {
			out.Error = marshalErr.Error()
			out.ExitCode = 1
		} else {
			out.Output = body
		}
	}

	return out, nil
}
