package proxy

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/factorly-dev/factorly-cli/internal/agent"
	"github.com/factorly-dev/factorly-cli/internal/logger"
	"github.com/factorly-dev/factorly-cli/internal/output"
	"github.com/factorly-dev/factorly-cli/internal/provider"
	"github.com/factorly-dev/factorly-cli/internal/registry"
	"github.com/factorly-dev/factorly-cli/internal/shadow"
)

// Option configures a Proxy.
type Option func(*Proxy)

// WithShadow adds a shadow governance policy to the proxy.
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

// Shadow returns the shadow policy, or nil if none is configured.
func (p *Proxy) Shadow() *shadow.Policy {
	return p.shadow
}

func (p *Proxy) Execute(toolName string, params map[string]string, iface string) (*provider.Result, error) {
	return p.ExecuteWithContext(context.Background(), toolName, params, iface)
}

func (p *Proxy) ExecuteWithContext(ctx context.Context, toolName string, params map[string]string, iface string) (*provider.Result, error) {
	tool, err := p.registry.Get(toolName)
	if err != nil {
		return nil, err
	}

	agentID := agent.AgentID(ctx)

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

		// Apply compression + truncation
		if len(hints) > 0 || maxOutput > 0 {
			originalBytes = len(result.Output)
			result.Output = output.Process(result.Output, maxOutput, hints...)
			processedBytes = len(result.Output)
		}
	}

	// Log the call
	entry := &logger.Entry{
		Timestamp:    time.Now(),
		Interface:    iface,
		Tool:         toolName,
		Params:       params,
		DurationMs:   result.Duration.Milliseconds(),
		ShadowAction: string(shadowAction),
		AgentID:        agentID,
		OriginalBytes:  originalBytes,
		ProcessedBytes: processedBytes,
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
