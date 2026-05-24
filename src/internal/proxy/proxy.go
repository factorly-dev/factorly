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

// contextKey is the unexported base type for typed context keys this
// package exposes. Using a typed key (rather than a bare string)
// prevents collisions with other packages stashing values on the
// same context.
type contextKey struct{ name string }

// ReplayedFromKey is the context key set by the /history replay
// handler before invoking the proxy. The proxy reads it and stamps
// the resulting audit entry's ReplayedFrom field so the new call
// can be traced back to the entry it was replayed from.
var ReplayedFromKey = contextKey{"replayed_from"}

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
	// WorkflowRunID / WorkflowName mirror logger.Entry. Present on
	// every step call of a workflow run; empty for standalone calls.
	// Lets live feeds (dashboard) coalesce steps under one parent row.
	WorkflowRunID string
	WorkflowName  string
	// Hash is the audit-chain identifier of the logged entry. SSE
	// consumers (dashboard live feed) use it as a stable handle for
	// lazy-loading per-row detail via GET /history/{hash}/detail.
	Hash string
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

	// Resolve {{backend:content}} patterns in param values.
	//
	// Caller-supplied values (this is the call path; the bootstrap
	// path for config-side templates is handled separately at config
	// load time) go through ResolveCallerParam, which:
	//   - Always resolves safe backends ({{env:V}}, {{store:K}},
	//     {{expr:...}}) — they don't expose anything the caller
	//     didn't already have access to.
	//   - Resolves secret backends ({{vault:K}}, {{op:...}}, external
	//     backends) ONLY if the tool's ParamConfig opted that
	//     specific param in via HydrateVaultRefs: true.
	//
	// Without this gating, a literal "{{vault:K}}" in caller-supplied
	// input (e.g. an LLM-generated tool param value) would silently
	// hydrate the secret into the outbound call and the audit log.
	// See vault.IsSafeBackendName for the allowlist and
	// internal/config/config.go ParamConfig.HydrateVaultRefs for the
	// opt-in semantics.
	//
	// secretRefs collects {{vault:K}}-style templates that DID
	// resolve against a secret backend (i.e. the param opted in).
	// It's keyed by param name so the audit-log step below can
	// replace the resolved value with the original template string,
	// keeping the persisted log free of plaintext secrets.
	var secretRefs map[string][]string
	if p.resolver != nil {
		for k, v := range params {
			if !strings.Contains(v, "{{") {
				continue
			}
			allow := paramAllowsSecretBackends(tool, k)
			resolved, refs, err := p.resolver.ResolveCallerParam(v, allow)
			if err != nil {
				continue
			}
			params[k] = resolved
			if len(refs) > 0 {
				if secretRefs == nil {
					secretRefs = make(map[string][]string)
				}
				secretRefs[k] = refs
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
				Params:       p.redactSecretRefsInParams(params, secretRefs),
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
			// Log the denial. Use the redacted-copy for both Params and
			// HighlightParams so a shadow-denied call with a vault ref
			// in its body doesn't leak the secret into the deny log.
			redacted := p.redactSecretRefsInParams(params, secretRefs)
			entry := &logger.Entry{
				Timestamp:    time.Now(),
				Interface:    iface,
				Tool:         toolName,
				Params:       redacted,
				Status:       "blocked",
				ShadowAction: string(action),
				Error:        err.Error(),
			}
			if logParams := p.shadow.LogParamsFor(toolName); len(logParams) > 0 {
				entry.HighlightParams = filterParams(redacted, logParams)
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
				Params:       p.redactSecretRefsInParams(params, secretRefs),
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

	// Prefer the context-aware path when the provider offers it so
	// caller deadlines + cancellation actually propagate into the call.
	// Falls back to the no-ctx Execute for providers that haven't been
	// migrated.
	type ctxExecutor interface {
		ExecuteWithContext(ctx context.Context, toolName string, params map[string]string) (*provider.Result, error)
	}
	var result *provider.Result
	var execErr error
	if ce, ok := prov.(ctxExecutor); ok {
		result, execErr = ce.ExecuteWithContext(ctx, toolName, params)
	} else {
		result, execErr = prov.Execute(toolName, params)
	}
	if execErr != nil {
		return nil, fmt.Errorf("executing %q: %w", toolName, execErr)
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

	// Log the call. Redact resolved secret-backend values BACK to
	// their template strings before persisting; the provider already
	// ran with the resolved values, but the audit log should reflect
	// what the caller actually said ("{{vault:K}}"), not what the
	// secret happened to be.
	entry := &logger.Entry{
		Timestamp:      time.Now(),
		Interface:      iface,
		Tool:           toolName,
		Params:         p.redactSecretRefsInParams(params, secretRefs),
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
	if src, _ := ctx.Value(ReplayedFromKey).(string); src != "" {
		entry.ReplayedFrom = src
	}
	if runID, _ := ctx.Value(provider.WorkflowRunIDKey).(string); runID != "" {
		entry.WorkflowRunID = runID
	}
	if name, _ := ctx.Value(provider.WorkflowNameKey).(string); name != "" {
		entry.WorkflowName = name
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
	// Stamp the source SHA for code-flavored calls so the audit trail
	// can identify exactly which script body ran, without inlining the
	// source itself. Two cases:
	//   - tool.ProviderKey == "code": registered type:code tool. The
	//     code provider holds the stashed source — ask it.
	//   - toolName == "factorly.code": agent-supplied source comes in
	//     as the `code` param. Hash it directly.
	if tool.ProviderKey == "code" {
		if cp, ok := prov.(*codeprov.Provider); ok {
			entry.SourceSHA = cp.SourceSHA(toolName)
		}
	} else if toolName == "factorly.code" {
		if src := params["code"]; src != "" {
			entry.SourceSHA = codeprov.SHA(src)
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
		Timestamp:     entry.Timestamp,
		Tool:          entry.Tool,
		Params:        entry.Params,
		Status:        entry.Status,
		DurationMs:    entry.DurationMs,
		ShadowAction:  entry.ShadowAction,
		AgentID:       entry.AgentID,
		Output:        output,
		Error:         entry.Error,
		SourceSHA:     entry.SourceSHA,
		WorkflowRunID: entry.WorkflowRunID,
		WorkflowName:  entry.WorkflowName,
		Hash:          entry.Hash,
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

// paramAllowsSecretBackends reports whether the tool config opts the
// named param into secret-backend substitution. Default false:
//   - param is not declared in the tool's Parameters list, OR
//   - param is declared but HydrateVaultRefs is false (the default).
//
// True only when there's a declared ParamConfig with HydrateVaultRefs:
// true. This keeps the security default (deny secret-backend resolution
// on caller-supplied values) for any param the operator hasn't
// explicitly trusted.
func paramAllowsSecretBackends(tool *registry.Tool, paramName string) bool {
	if tool == nil {
		return false
	}
	for _, pd := range tool.Parameters {
		if pd.Name == paramName {
			return pd.HydrateVaultRefs
		}
	}
	return false
}

// redactSecretRefsInParams replaces values in the params map that
// came from a secret-backend substitution with the original
// "{{backend:key}}" template string. Used right before building a
// logger.Entry so the audit log persists template strings, not
// plaintext secrets. secretRefs is keyed by param name; the value
// is the list of templates that successfully resolved during this
// call (from Resolver.ResolveCallerParam).
//
// Mutates a shallow copy of params so the caller's params map (which
// is being passed to the provider) is not affected. The audit log
// gets the redacted copy; the provider gets the resolved values.
//
// No-op when secretRefs is empty (the common case). Resolver lookups
// in RedactToTemplate fall back to no-op when the backend can no
// longer return the value — the resolved value stays in the audit
// log in that degenerate case, which is documented.
func (p *Proxy) redactSecretRefsInParams(params map[string]string, secretRefs map[string][]string) map[string]string {
	if len(secretRefs) == 0 || p.resolver == nil {
		return params
	}
	out := make(map[string]string, len(params))
	for k, v := range params {
		if refs, ok := secretRefs[k]; ok {
			v = p.resolver.RedactToTemplate(v, refs)
		}
		out[k] = v
	}
	return out
}
