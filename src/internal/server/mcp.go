// Copyright 2026 Jordan Sherer <hi@jordansherer.com>
// SPDX-License-Identifier: gpl

package server

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/factorly-dev/factorly/internal"
	"github.com/factorly-dev/factorly/internal/agent"
	"github.com/factorly-dev/factorly/internal/config"
	"github.com/factorly-dev/factorly/internal/configyaml"
	"github.com/factorly-dev/factorly/internal/proxy"
	"github.com/factorly-dev/factorly/internal/registry"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

var sensitiveParamNames = []string{"token", "secret", "password", "key", "auth", "credential"}

func redactSensitiveParams(params map[string]string) map[string]string {
	redacted := make(map[string]string, len(params))
	for k, v := range params {
		lower := strings.ToLower(k)
		sensitive := false
		for _, s := range sensitiveParamNames {
			if strings.Contains(lower, s) {
				sensitive = true
				break
			}
		}
		if sensitive {
			redacted[k] = "[REDACTED]"
		} else {
			redacted[k] = v
		}
	}
	return redacted
}

// Verbose is an optional log function for debug output. Set by the CLI --verbose flag.
var Verbose func(format string, args ...any)

func vlog(format string, args ...any) {
	if Verbose != nil {
		Verbose(format, args...)
	}
}

// slog logs with a session prefix: [session-id] message
func slog(ctx context.Context, format string, args ...any) {
	if Verbose == nil {
		return
	}
	sid := ""
	if session := server.ClientSessionFromContext(ctx); session != nil {
		sid = session.SessionID()
	}
	if sid != "" {
		Verbose("[%s] "+format, append([]any{sid}, args...)...)
	} else {
		Verbose(format, args...)
	}
}

// New creates an MCP server with all registry tools exposed plus YAML
// resources for tools, workflows, and installed blueprints. cfg and cfgPath
// can be zero values (nil / "") in contexts where resource discovery isn't
// wanted — only tools will be registered in that case.
// An optional agent.Registry can be passed to track connected agents.
func New(reg *registry.Registry, p *proxy.Proxy, cfg *config.Config, cfgPath string, agentReg ...*agent.Registry) *server.MCPServer {
	var ar *agent.Registry
	if len(agentReg) > 0 {
		ar = agentReg[0]
	}

	hooks := &server.Hooks{}
	hooks.AddOnRegisterSession(func(ctx context.Context, session server.ClientSession) {
		vlog("[%s] client connected", session.SessionID())
	})
	hooks.AddAfterInitialize(func(ctx context.Context, id any, message *mcp.InitializeRequest, result *mcp.InitializeResult) {
		slog(ctx, "client initialized: %s %s (protocol: %s)",
			message.Params.ClientInfo.Name,
			message.Params.ClientInfo.Version,
			message.Params.ProtocolVersion)

		// Register agent identity
		if ar != nil {
			if session := server.ClientSessionFromContext(ctx); session != nil {
				ar.Register(&agent.Info{
					ID:      session.SessionID(),
					Name:    message.Params.ClientInfo.Name,
					Version: message.Params.ClientInfo.Version,
				})
			}
		}
	})
	hooks.AddAfterListTools(func(ctx context.Context, id any, message *mcp.ListToolsRequest, result *mcp.ListToolsResult) {
		slog(ctx, "client listed tools (%d tools)", len(result.Tools))
	})

	s := server.NewMCPServer(
		internal.AppName,
		internal.Version,
		server.WithToolCapabilities(false),
		server.WithResourceCapabilities(false, true),
		server.WithElicitation(),
		server.WithHooks(hooks),
	)

	for _, tool := range reg.ListVisible() {
		// Hide denied tools from MCP clients
		if p.Shadow() != nil && p.Shadow().IsDenied(tool.Name) {
			continue
		}
		mcpTool := convertTool(tool)
		handler := makeHandler(p, tool.Name, ar)
		s.AddTool(mcpTool, handler)
	}

	if cfg != nil {
		registerResources(s, cfg, cfgPath)
	}

	return s
}

// URI prefixes for the three resource kinds.
const (
	resourceURITool      = "factorly://tools/"
	resourceURIWorkflow  = "factorly://workflows/"
	resourceURIBlueprint = "factorly://blueprints/"
)

// registerResources walks the current config + installed blueprints and
// registers one MCP resource per item. Each resource handler closes over
// cfg/cfgPath so subsequent reads pick up the live state — important
// because RefreshResources mutates the registered set in place but the
// handler closures stay valid.
func registerResources(s *server.MCPServer, cfg *config.Config, cfgPath string) {
	for name, tc := range cfg.Tools {
		uri, kind := resourceURIForTool(name, tc)
		_ = kind // reserved for future use (e.g., labels)
		res := mcp.NewResource(uri, name, mcp.WithMIMEType("application/yaml"))
		nameCopy, tcCopy := name, tc
		s.AddResource(res, func(ctx context.Context, request mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
			data, err := configyaml.RenderTool(nameCopy, tcCopy)
			if err != nil {
				return nil, err
			}
			return []mcp.ResourceContents{mcp.TextResourceContents{
				URI:      request.Params.URI,
				MIMEType: "application/yaml",
				Text:     string(data),
			}}, nil
		})
	}

	if cfgPath == "" {
		return
	}
	for _, name := range installedBlueprintNames(cfgPath) {
		uri := resourceURIBlueprint + name
		res := mcp.NewResource(uri, name, mcp.WithMIMEType("application/yaml"))
		bpName, bpCfgPath := name, cfgPath
		s.AddResource(res, func(ctx context.Context, request mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
			data, err := configyaml.RenderBlueprint(bpCfgPath, bpName)
			if err != nil {
				return nil, err
			}
			return []mcp.ResourceContents{mcp.TextResourceContents{
				URI:      request.Params.URI,
				MIMEType: "application/yaml",
				Text:     string(data),
			}}, nil
		})
	}
}

// RefreshResources reconciles the server's registered resources against the
// live config. URIs that no longer correspond to a tool/workflow/blueprint
// are removed; new ones are added. mcp-go auto-emits
// notifications/resources/list_changed on DeleteResources when listChanged
// capability is enabled, so connected MCP clients see the updated set
// without reconnecting.
func RefreshResources(s *server.MCPServer, cfg *config.Config, cfgPath string) {
	desired := desiredResourceURIs(cfg, cfgPath)
	current := s.ListResources()
	var toRemove []string
	for uri := range current {
		if !strings.HasPrefix(uri, "factorly://") {
			continue
		}
		if _, keep := desired[uri]; !keep {
			toRemove = append(toRemove, uri)
		}
	}
	if len(toRemove) > 0 {
		s.DeleteResources(toRemove...)
	}
	// Register any new/updated resources. AddResource is idempotent on URI
	// in the underlying library (last write wins), so unchanged URIs simply
	// get their handler closure refreshed.
	registerResources(s, cfg, cfgPath)
}

// resourceURIForTool returns the canonical URI for a tool/workflow plus a
// label identifying which kind it is (mostly for callers that want both).
func resourceURIForTool(name string, tc config.ToolConfig) (uri, kind string) {
	if tc.Type == "workflow" {
		return resourceURIWorkflow + name, "workflow"
	}
	return resourceURITool + name, "tool"
}

// desiredResourceURIs returns the set of URIs that should be registered for
// the given config + blueprints directory.
func desiredResourceURIs(cfg *config.Config, cfgPath string) map[string]struct{} {
	out := make(map[string]struct{}, len(cfg.Tools))
	for name, tc := range cfg.Tools {
		uri, _ := resourceURIForTool(name, tc)
		out[uri] = struct{}{}
	}
	if cfgPath != "" {
		for _, name := range installedBlueprintNames(cfgPath) {
			out[resourceURIBlueprint+name] = struct{}{}
		}
	}
	return out
}

// installedBlueprintNames returns the basenames (without extension) of every
// .yaml file in the configured blueprints directory. Missing dir → empty.
func installedBlueprintNames(cfgPath string) []string {
	dir := configyaml.BlueprintsDir(cfgPath)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		ext := filepath.Ext(e.Name())
		if ext != ".yaml" && ext != ".yml" {
			continue
		}
		names = append(names, strings.TrimSuffix(e.Name(), ext))
	}
	return names
}

// convertTool maps a registry.Tool to an mcp.Tool with JSON Schema.
// All parameters are declared as strings since Proxy.Execute takes map[string]string.
func convertTool(t *registry.Tool) mcp.Tool {
	opts := []mcp.ToolOption{
		mcp.WithDescription(t.Description),
	}
	for _, param := range t.Parameters {
		paramOpts := []mcp.PropertyOption{
			mcp.Description(param.Description),
		}
		if param.Required {
			paramOpts = append(paramOpts, mcp.Required())
		}
		opts = append(opts, mcp.WithString(param.Name, paramOpts...))
	}
	return mcp.NewTool(t.Name, opts...)
}

// makeHandler returns an MCP handler that delegates to Proxy.Execute.
func makeHandler(p *proxy.Proxy, toolName string, ar *agent.Registry) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		// Inject agent ID from MCP session
		if session := server.ClientSessionFromContext(ctx); session != nil {
			ctx = agent.WithAgentID(ctx, session.SessionID())
			if ar != nil {
				ar.Touch(session.SessionID())
			}
		}

		params := make(map[string]string)
		if request.GetArguments() != nil {
			for key, val := range request.GetArguments() {
				params[key] = fmt.Sprintf("%v", val)
			}
		}

		slog(ctx, "mcp call: %s params=%v", toolName, redactSensitiveParams(params))

		result, err := p.ExecuteWithContext(ctx, toolName, params, "mcp")
		if err != nil {
			slog(ctx, "mcp call %s: error: %v", toolName, err)
			return mcp.NewToolResultError(err.Error()), nil
		}

		if result.IsError() {
			slog(ctx, "mcp call %s: failed (exit %d, %s)", toolName, result.ExitCode, result.Duration)
			msg := result.Error
			if msg == "" {
				msg = result.Output
			}
			return mcp.NewToolResultError(msg), nil
		}

		slog(ctx, "mcp call %s: success (%s, %d bytes)", toolName, result.Duration, len(result.Output))
		return mcp.NewToolResultText(result.Output), nil
	}
}
