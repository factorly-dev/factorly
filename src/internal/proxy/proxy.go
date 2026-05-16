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
	codeprov "github.com/factorly-dev/factorly/internal/provider/code"
	"github.com/factorly-dev/factorly/internal/registry"
	"github.com/factorly-dev/factorly/internal/shadow"
	"github.com/factorly-dev/factorly/internal/vault"
)

// Option configures a Proxy.
type Option func(*Proxy)

// WithShadow adds a shadow oversight policy to the proxy.
func WithShadow(s *shadow.Policy) Option {
	return func(p *Proxy) { p.shadow = s }
}

// WithOnCall sets a callback that fires after each tool call.
func WithOnCall(fn func(CallEvent)) Option {
	return func(p *Proxy) { p.onCall = fn }
}

// WithResolver sets a vault resolver for resolving {{backend:content}}
// patterns in param values at call time.
func WithResolver(r *vault.Resolver) Option {
	return func(p *Proxy) { p.resolver = r }
}

// SetOnCall sets the call event callback after proxy creation.
func (p *Proxy) SetOnCall(fn func(CallEvent)) {
	p.onCall = fn
}

// SetOnWorkflowStep sets a callback for real-time workflow step events.
// The callback is also remembered on the proxy so that a workflow provider
// registered later (e.g., lazy-created when the first workflow is added via
// the UI) can be wired up too.
func (p *Proxy) SetOnWorkflowStep(fn func(workflow string, event provider.StepEvent)) {
	p.onWorkflowStep = fn
	if wp, ok := p.providers["workflow"].(*provider.WorkflowProvider); ok {
		wp.OnStep = fn
	}
}

// CallEvent is emitted after each tool call for live activity feeds.
type CallEvent struct {
	Timestamp    time.Time
	Tool         string
	Params       map[string]string
	Status       string // "success", "error", "blocked"
	DurationMs   int64
	ShadowAction string
	AgentID      string
	Output       string
	Error        string
	// SourceSHA mirrors logger.Entry.SourceSHA — present when the tool
	// that ran was a code tool (or, in V2, the factorly.code builtin).
	SourceSHA string
}

type Proxy struct {
	registry       *registry.Registry
	providers      map[string]provider.Provider
	logger         logger.Logger
	shadow         *shadow.Policy
	onCall         func(CallEvent)
	onWorkflowStep func(workflow string, event provider.StepEvent)
	resolver       *vault.Resolver
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

// RegisterProvider adds a provider after proxy creation. If a workflow
// provider is registered and an OnWorkflowStep callback was previously
// set, it is applied to the new provider so step events are still
// broadcast.
func (p *Proxy) RegisterProvider(key string, prov provider.Provider) {
	p.providers[key] = prov
	if key == "workflow" && p.onWorkflowStep != nil {
		if wp, ok := prov.(*provider.WorkflowProvider); ok {
			wp.OnStep = p.onWorkflowStep
		}
	}
}

// Provider returns a registered provider by key, or nil if not found.
func (p *Proxy) Provider(key string) provider.Provider {
	return p.providers[key]
}

// Shadow returns the shadow policy, or nil if none is configured.
func (p *Proxy) Shadow() *shadow.Policy {
	return p.shadow
}

// ListVisibleToolsForScript satisfies the optional toolLister interface
// the code provider uses to surface factorly.ListTools() into scripts.
// Returns non-hidden tools only; mirrors what MCP's tools/list and the
// `factorly tools` CLI show. Snapshot at call time; the caller (code
// provider) snapshots again at script start to keep the in-script view
// stable.
func (p *Proxy) ListVisibleToolsForScript() []codeprov.ToolInfo {
	tools := p.registry.ListVisible()
	out := make([]codeprov.ToolInfo, 0, len(tools))
	for _, t := range tools {
		// Hide tools blocked outright by shadow policy — they aren't
		// callable, so the script shouldn't be advertised them.
		if p.shadow != nil && p.shadow.IsDenied(t.Name) {
			continue
		}
		ti := codeprov.ToolInfo{
			Name:        t.Name,
			Description: t.Description,
		}
		for _, par := range t.Parameters {
			pType := par.Type
			if pType == "" {
				pType = "string"
			}
			ti.Parameters = append(ti.Parameters, codeprov.ParamInfo{
				Name:        par.Name,
				Type:        pType,
				Required:    par.Required,
				Description: par.Description,
				Default:     par.Default,
			})
		}
		out = append(out, ti)
	}
	return out
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

	// Resolve {{backend:content}} patterns in param values (e.g., {{expr:now()}})
	if p.resolver != nil {
		for k, v := range params {
			if strings.Contains(v, "{{") {
				if resolved, err := p.resolver.Resolve(v); err == nil {
					params[k] = resolved
				}
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
			p.emitCallEvent(entry)
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
			p.emitCallEvent(entry)
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
			p.emitCallEvent(entry)
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
		VaultKeys:      tool.VaultKeys,
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
	// Stamp the source SHA for code-tool calls so the audit trail can
	// identify exactly which script body ran, without inlining the
	// source itself.
	if tool.ProviderKey == "code" {
		if cp, ok := prov.(*codeprov.Provider); ok {
			entry.SourceSHA = cp.SourceSHA(toolName)
		}
	}
	if logErr := p.logger.Log(entry); logErr != nil {
		fmt.Fprintf(os.Stderr, "warning: failed to log call: %v\n", logErr)
	}

	p.emitCallEvent(entry)

	return result, nil
}

func (p *Proxy) emitCallEvent(entry *logger.Entry) {
	if p.onCall == nil {
		return
	}
	output := entry.Output
	if len(output) > 500 {
		output = output[:500] + "..."
	}
	p.onCall(CallEvent{
		Timestamp:    entry.Timestamp,
		Tool:         entry.Tool,
		Params:       entry.Params,
		Status:       entry.Status,
		DurationMs:   entry.DurationMs,
		ShadowAction: entry.ShadowAction,
		AgentID:      entry.AgentID,
		Output:       output,
		Error:        entry.Error,
		SourceSHA:    entry.SourceSHA,
	})
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
