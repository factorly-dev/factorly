package proxy

import (
	"fmt"
	"os"
	"time"

	"github.com/factorly-dev/factorly/internal/logger"
	"github.com/factorly-dev/factorly/internal/provider"
	"github.com/factorly-dev/factorly/internal/registry"
)

type Proxy struct {
	registry  *registry.Registry
	providers map[string]provider.Provider
	logger    logger.Logger
}

func New(reg *registry.Registry, providers map[string]provider.Provider, log logger.Logger) *Proxy {
	return &Proxy{
		registry:  reg,
		providers: providers,
		logger:    log,
	}
}

func (p *Proxy) Execute(toolName string, params map[string]string, iface string) (*provider.Result, error) {
	tool, err := p.registry.Get(toolName)
	if err != nil {
		return nil, err
	}

	prov, ok := p.providers[tool.ProviderKey]
	if !ok {
		return nil, fmt.Errorf("no provider for tool %q (key: %s)", toolName, tool.ProviderKey)
	}

	result, err := prov.Execute(toolName, params)
	if err != nil {
		return nil, fmt.Errorf("executing %q: %w", toolName, err)
	}

	// Log the call (non-fatal on error)
	entry := &logger.Entry{
		Timestamp:  time.Now(),
		Interface:  iface,
		Tool:       toolName,
		Params:     params,
		DurationMs: result.Duration.Milliseconds(),
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
