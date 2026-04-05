package server

import (
	"context"
	"fmt"

	"github.com/factorly-dev/factorly/internal"
	"github.com/factorly-dev/factorly/internal/proxy"
	"github.com/factorly-dev/factorly/internal/registry"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// Verbose is an optional log function for debug output. Set by the CLI --verbose flag.
var Verbose func(format string, args ...any)

func vlog(format string, args ...any) {
	if Verbose != nil {
		Verbose(format, args...)
	}
}

// New creates an MCP server with all registry tools exposed.
func New(reg *registry.Registry, p *proxy.Proxy) *server.MCPServer {
	hooks := &server.Hooks{}
	hooks.AddOnRegisterSession(func(ctx context.Context, session server.ClientSession) {
		vlog("client connected")
	})
	hooks.AddAfterInitialize(func(ctx context.Context, id any, message *mcp.InitializeRequest, result *mcp.InitializeResult) {
		vlog("client initialized: %s %s (protocol: %s)",
			message.Params.ClientInfo.Name,
			message.Params.ClientInfo.Version,
			message.Params.ProtocolVersion)
	})
	hooks.AddAfterListTools(func(ctx context.Context, id any, message *mcp.ListToolsRequest, result *mcp.ListToolsResult) {
		vlog("client listed tools (%d tools)", len(result.Tools))
	})

	s := server.NewMCPServer(
		internal.AppName,
		internal.Version,
		server.WithToolCapabilities(false),
		server.WithHooks(hooks),
	)

	for _, tool := range reg.List() {
		mcpTool := convertTool(tool)
		handler := makeHandler(p, tool.Name)
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
func makeHandler(p *proxy.Proxy, toolName string) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		params := make(map[string]string)
		if request.GetArguments() != nil {
			for key, val := range request.GetArguments() {
				params[key] = fmt.Sprintf("%v", val)
			}
		}

		vlog("mcp call: %s params=%v", toolName, params)

		result, err := p.Execute(toolName, params, "mcp")
		if err != nil {
			vlog("mcp call %s: error: %v", toolName, err)
			return mcp.NewToolResultError(err.Error()), nil
		}

		if result.IsError() {
			vlog("mcp call %s: failed (exit %d, %s)", toolName, result.ExitCode, result.Duration)
			msg := result.Error
			if msg == "" {
				msg = result.Output
			}
			return mcp.NewToolResultError(msg), nil
		}

		vlog("mcp call %s: success (%s, %d bytes)", toolName, result.Duration, len(result.Output))
		return mcp.NewToolResultText(result.Output), nil
	}
}
