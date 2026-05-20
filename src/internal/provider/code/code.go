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
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/factorly-dev/factorly/internal/provider"
)

// SHA returns the SHA-256 (hex) of a script source. Public so the proxy
// can compute the same hash for log entries when needed.
func SHA(src string) string {
	sum := sha256.Sum256([]byte(src))
	return hex.EncodeToString(sum[:])
}

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

// entry is one registered code tool's compiled state. If compileErr is
// non-nil the script failed validation and Execute should return the
// stored error instead of attempting to run.
type entry struct {
	src        string
	maxCalls   int
	compileErr error
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

// Validate compiles the script in a sandboxed yaegi interpreter
// without running Run. Returns nil on success, or the parse/compile
// error if the source is malformed. Used by the promote pipeline to
// reject a tool YAML before writing it to disk — landing a broken
// `type: code` tool helps nobody.
//
// Behavior matches what RegisterCode would check, minus the
// side-effect of stashing the script under a tool name.
func Validate(src string) error {
	return validateScript(src)
}

// RegisterCode validates the script and stashes it for later execution.
// Validation runs yaegi against the source so syntax errors, missing
// `Run`, wrong signature, and denied imports all surface here.
//
// When validation fails, the script is still stashed with its compile
// error so a later Execute call returns a helpful "script failed to
// compile" message rather than the proxy's generic "no provider"
// failure mode. RegisterCode still returns the error so the caller can
// log a warning at registration time too.
func (p *Provider) RegisterCode(name, src string, maxCalls int) error {
	if maxCalls <= 0 {
		maxCalls = DefaultMaxCalls
	}
	compileErr := validateScript(src)
	p.mu.Lock()
	p.scripts[name] = &entry{src: src, maxCalls: maxCalls, compileErr: compileErr}
	p.mu.Unlock()
	return compileErr
}

// SourceSHA returns the SHA-256 of the registered script for toolName,
// or "" if the tool isn't registered. Used by the proxy to stamp call
// events / audit log entries with a stable identifier for the source
// that actually ran.
func (p *Provider) SourceSHA(toolName string) string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	e, ok := p.scripts[toolName]
	if !ok {
		return ""
	}
	return SHA(e.src)
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
	// Script was registered but failed yaegi validation. Surface the
	// compile error as a script-level failure (ExitCode 1) so callers see
	// the underlying message and can fix the source.
	if e.compileErr != nil {
		return &provider.Result{
			Error:    fmt.Sprintf("script failed to compile: %v", e.compileErr),
			ExitCode: 1,
		}, nil
	}
	return p.Run(ctx, e.src, params, e.maxCalls)
}

// Run compiles + executes an arbitrary script source. Unlike
// ExecuteWithContext it doesn't go through the registered-scripts map —
// the caller supplies the source directly. Used by ExecuteWithContext
// for registered type:code tools, and intended to be used by a future
// factorly.code builtin that takes agent-supplied source at call time.
//
// maxCalls = 0 → DefaultMaxCalls. Returns (*Result, nil) for every
// outcome (success, script error, panic, compile error). Returns an
// infrastructure error only for unrecoverable plumbing failures.
func (p *Provider) Run(ctx context.Context, src string, params map[string]string, maxCalls int) (*provider.Result, error) {
	if maxCalls <= 0 {
		maxCalls = DefaultMaxCalls
	}

	start := time.Now()

	// Per-execution call counter. Wrapping captures maxCalls so each
	// script invocation gets its own budget without sharing state.
	var (
		callCount int
		callMu    sync.Mutex
	)
	call := func(name string, callParams map[string]string) (*Result, error) {
		callMu.Lock()
		callCount++
		current := callCount
		callMu.Unlock()
		if current > maxCalls {
			return nil, fmt.Errorf("script exceeded max_calls (%d) — increase max_calls on the tool definition or reduce the script's call volume", maxCalls)
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

	val, runErr := runScript(ctx, src, params, call, p.tools())
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

// tools returns the current snapshot of registered tools for the
// in-script factorly.ListTools() helper. Excludes hidden tools. Returns
// an empty slice if the executor doesn't expose a tool inventory.
func (p *Provider) tools() []ToolInfo {
	lister, ok := p.executor.(toolLister)
	if !ok {
		return nil
	}
	return lister.ListVisibleToolsForScript()
}

// toolLister is the optional capability ToolExecutor implementations
// can expose to surface tool metadata into the in-script SDK. The proxy
// implements this via ListVisibleToolsForScript on *proxy.Proxy.
type toolLister interface {
	ListVisibleToolsForScript() []ToolInfo
}
