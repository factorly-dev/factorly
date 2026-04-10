package server

import (
	"context"
	"fmt"
	"strings"

	"github.com/factorly-dev/factorly-cli/internal"
	"github.com/factorly-dev/factorly-cli/internal/agent"
	"github.com/factorly-dev/factorly-cli/internal/proxy"
	"github.com/factorly-dev/factorly-cli/internal/registry"
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

// New creates an MCP server with all registry tools exposed.
// An optional agent.Registry can be passed to track connected agents.
func New(reg *registry.Registry, p *proxy.Proxy, agentReg ...*agent.Registry) *server.MCPServer {
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
		server.WithElicitation(),
		server.WithHooks(hooks),
	)

	for _, tool := range reg.List() {
		// Hide denied tools from MCP clients
		if p.Shadow() != nil && p.Shadow().IsDenied(tool.Name) {
			continue
		}
		mcpTool := convertTool(tool)
		handler := makeHandler(p, tool.Name, ar)
		s.AddTool(mcpTool, handler)
	}

	return s
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
