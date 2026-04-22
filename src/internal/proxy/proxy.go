// Copyright 2026 Jordan Sherer <hi@jordansherer.com>
// SPDX-License-Identifier: gpl

package proxy

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/factorly-dev/factorly/internal/agent"
	"github.com/factorly-dev/factorly/internal/builtins"
	"github.com/factorly-dev/factorly/internal/logger"
	"github.com/factorly-dev/factorly/internal/output"
	"github.com/factorly-dev/factorly/internal/provider"
	"github.com/factorly-dev/factorly/internal/registry"
	"github.com/factorly-dev/factorly/internal/shadow"
)

// Option configures a Proxy.
type Option func(*Proxy)

// WithShadow adds a shadow oversight policy to the proxy.
func WithShadow(s *shadow.Policy) Option {
	return func(p *Proxy) { p.shadow = s }
}

type Proxy struct {
	registry  *registry.Registry
	providers map[string]provider.Provider
	logger    logger.Logger
	shadow    *shadow.Policy
}

func New(reg *registry.Registry, providers map[string]provider.Provider, log logger.Logger, opts ...Option) *Proxy {
	p := &Proxy{
		registry:  reg,
		providers: providers,
		logger:    log,
	}
	for _, opt := range opts {
		opt(p)
	}
	return p
}

// RegisterProvider adds a provider after proxy creation.
func (p *Proxy) RegisterProvider(key string, prov provider.Provider) {
	p.providers[key] = prov
}

// Shadow returns the shadow policy, or nil if none is configured.
func (p *Proxy) Shadow() *shadow.Policy {
	return p.shadow
}

// Teardown shuts down all providers (closes child processes, connections).
func (p *Proxy) Teardown() {
	for _, prov := range p.providers {
		_ = prov.Teardown()
	}
}

func (p *Proxy) Execute(toolName string, params map[string]string, iface string) (*provider.Result, error) {
	return p.ExecuteWithContext(context.Background(), toolName, params, iface)
}

func (p *Proxy) ExecuteWithContext(ctx context.Context, toolName string, params map[string]string, iface string) (*provider.Result, error) {
	tool, err := p.registry.Get(toolName)
	if err != nil {
		return nil, err
	}

	// Apply parameter defaults for any missing params
	for _, pd := range tool.Parameters {
		if pd.Default != "" {
			if _, ok := params[pd.Name]; !ok {
				if params == nil {
					params = make(map[string]string)
				}
				params[pd.Name] = pd.Default
			}
		}
	}

	agentID := agent.AgentID(ctx)

	// Parameter validation and coercion
	var validationResult *registry.ValidationResult
	if len(tool.Parameters) > 0 {
		validationResult = tool.ValidateAndCoerce(params)
		if validationResult.HasErrors() {
			entry := &logger.Entry{
				Timestamp:    time.Now(),
				Interface:    iface,
				Tool:         toolName,
				Params:       params,
				Status:       "blocked",
				ShadowAction: string(shadow.ActionInvalid),
				Error:        strings.Join(validationResult.Errors, "; "),
				AgentID:      agentID,
			}
			_ = p.logger.Log(entry)
			return nil, fmt.Errorf("parameter validation failed for %q: %s",
				toolName, strings.Join(validationResult.Errors, "; "))
		}
	}

	// Shadow policy check
	var shadowAction shadow.Action = shadow.ActionAllowed
	if p.shadow != nil {
		action, err := p.shadow.Check(ctx, toolName, params, iface)
		shadowAction = action
		if err != nil {
			// Log the denial
			entry := &logger.Entry{
				Timestamp:    time.Now(),
				Interface:    iface,
				Tool:         toolName,
				Params:       params,
				Status:       "blocked",
				ShadowAction: string(action),
				Error:        err.Error(),
			}
			if logParams := p.shadow.LogParamsFor(toolName); len(logParams) > 0 {
				entry.HighlightParams = filterParams(params, logParams)
			}
			entry.AgentID = agentID
			_ = p.logger.Log(entry)
			return nil, err
		}
	}

	// Built-in tool safety guard
	if builtins.IsBuiltinTool(toolName) {
		var allowOverrides []string
		if tool.AllowOverrides != nil {
			allowOverrides = tool.AllowOverrides
		}
		if err := builtins.CheckGuard(toolName, params, allowOverrides); err != nil {
			entry := &logger.Entry{
				Timestamp:    time.Now(),
				Interface:    iface,
				Tool:         toolName,
				Params:       params,
				Status:       "blocked",
				ShadowAction: "guard_blocked",
				Error:        err.Error(),
				AgentID:      agentID,
			}
			_ = p.logger.Log(entry)
			return nil, err
		}
	}

	prov, ok := p.providers[tool.ProviderKey]
	if !ok {
		return nil, fmt.Errorf("no provider for tool %q (key: %s)", toolName, tool.ProviderKey)
	}

	result, err := prov.Execute(toolName, params)
	if err != nil {
		return nil, fmt.Errorf("executing %q: %w", toolName, err)
	}

	// Apply output processing (compression + truncation)
	var originalBytes, processedBytes int
	if result != nil && result.Output != "" {
		// Determine max output: per-tool config > env var > 0 (unlimited)
		maxOutput := tool.MaxOutput
		if maxOutput == 0 {
			if v := os.Getenv("FACTORLY_MAX_OUTPUT"); v != "" {
				if n, parseErr := strconv.Atoi(v); parseErr == nil && n > 0 {
					maxOutput = n
				}
			}
		}

		// Build compression hints from tool config
		var hints []output.Hint
		for _, h := range tool.Compress {
			hints = append(hints, output.Hint(h))
		}

		// Resolve filter: user-defined > built-in (for shell/exec commands)
		filter := tool.Filter
		if filter == nil {
			if cmd := params["command"]; cmd != "" {
				filter = output.BuiltinFilter(cmd)
			}
		}

		// Apply compression + truncation
		if len(hints) > 0 || maxOutput > 0 || filter != nil {
			originalBytes = len(result.Output)
			result.Output = output.Process(result.Output, maxOutput, filter, hints...)
			processedBytes = len(result.Output)
		}
	}

	// Log the call
	entry := &logger.Entry{
		Timestamp:      time.Now(),
		Interface:      iface,
		Tool:           toolName,
		Params:         params,
		DurationMs:     result.Duration.Milliseconds(),
		ShadowAction:   string(shadowAction),
		AgentID:        agentID,
		OriginalBytes:  originalBytes,
		ProcessedBytes: processedBytes,
	}
	if validationResult != nil && validationResult.WasModified() {
		entry.ShadowAction = string(shadow.ActionModified)
		entry.OriginalParams = validationResult.Modified
	}
	if p.shadow != nil {
		if logParams := p.shadow.LogParamsFor(toolName); len(logParams) > 0 {
			entry.HighlightParams = filterParams(params, logParams)
		}
	}
	if result.IsError() {
		entry.Status = "error"
		entry.Error = result.Error
	} else {
		entry.Status = "success"
		entry.Output = result.Output
	}
	if logErr := p.logger.Log(entry); logErr != nil {
		fmt.Fprintf(os.Stderr, "warning: failed to log call: %v\n", logErr)
	}

	return result, nil
}

func filterParams(params map[string]string, keys []string) map[string]string {
	result := make(map[string]string, len(keys))
	for _, k := range keys {
		if v, ok := params[k]; ok {
			result[k] = v
		}
	}
	return result
}
