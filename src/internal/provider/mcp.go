package provider

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/factorly-hq/factorly-cli/internal"
	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
)

const mcpTimeout = 30 * time.Second

// MCPServerDef defines a child MCP server to connect to.
type MCPServerDef struct {
	Command string            // stdio transport: executable to spawn
	Args    []string          // stdio transport: arguments
	Env     map[string]string // stdio transport: environment variables
	URL     string            // http transport: server URL
}

// DiscoveredTool is a tool discovered from a child MCP server.
type DiscoveredTool struct {
	Name        string
	Description string
	Parameters  []DiscoveredParam
	ServerKey   string // config name of the server that owns this tool
	RemoteName  string // original tool name on the child server
}

// DiscoveredParam is a parameter from a discovered MCP tool.
type DiscoveredParam struct {
	Name        string
	Description string
	Required    bool
}

type mcpConn struct {
	client     *client.Client
	serverKey  string
	remoteName map[string]string // factorly tool name → remote tool name
}

// MCPProvider wraps child MCP servers and forwards tool calls.
type MCPProvider struct {
	servers map[string]*mcpConn // keyed by config name
	toolMap map[string]string   // factorly tool name → config name
	defs    map[string]MCPServerDef
}

// NewMCP creates an MCP provider with the given server definitions.
func NewMCP(servers map[string]MCPServerDef) *MCPProvider {
	return &MCPProvider{
		servers: make(map[string]*mcpConn),
		toolMap: make(map[string]string),
		defs:    servers,
	}
}

// Setup connects to all configured MCP servers and initializes them.
func (p *MCPProvider) Setup() error {
	for name, def := range p.defs {
		c, err := ConnectMCP(def)
		if err != nil {
			// Clean up already-connected servers
			for _, conn := range p.servers {
				conn.client.Close()
			}
			return fmt.Errorf("mcp provider: connecting to %q: %w", name, err)
		}
		p.servers[name] = &mcpConn{
			client:     c,
			serverKey:  name,
			remoteName: make(map[string]string),
		}
	}
	return nil
}

func ConnectMCP(def MCPServerDef) (*client.Client, error) {
	var c *client.Client
	var err error

	if def.URL != "" {
		// HTTP transport
		c, err = client.NewStreamableHttpClient(def.URL)
	} else {
		// Stdio transport — only pass PATH + explicitly configured env vars
		env := []string{"PATH=" + os.Getenv("PATH")}
		for k, v := range def.Env {
			env = append(env, k+"="+v)
		}
		c, err = client.NewStdioMCPClient(def.Command, env, def.Args...)
	}
	if err != nil {
		return nil, fmt.Errorf("creating client: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), mcpTimeout)
	defer cancel()

	initReq := mcp.InitializeRequest{}
	initReq.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
	initReq.Params.ClientInfo = mcp.Implementation{
		Name:    internal.AppName,
		Version: internal.Version,
	}

	if _, err := c.Initialize(ctx, initReq); err != nil {
		c.Close()
		return nil, fmt.Errorf("initializing: %w", err)
	}

	return c, nil
}

// DiscoverTools queries all connected MCP servers for their tools.
// Call after Setup(). Returns tools namespaced as serverName.toolName.
func (p *MCPProvider) DiscoverTools() ([]DiscoveredTool, error) {
	var all []DiscoveredTool

	for name, conn := range p.servers {
		ctx, cancel := context.WithTimeout(context.Background(), mcpTimeout)
		result, err := conn.client.ListTools(ctx, mcp.ListToolsRequest{})
		cancel()
		if err != nil {
			return nil, fmt.Errorf("mcp provider: listing tools from %q: %w", name, err)
		}

		for _, tool := range result.Tools {
			factorlyName := name + "." + tool.Name
			conn.remoteName[factorlyName] = tool.Name
			p.toolMap[factorlyName] = name

			dt := DiscoveredTool{
				Name:        factorlyName,
				Description: tool.Description,
				ServerKey:   name,
				RemoteName:  tool.Name,
			}

			// Extract parameters from JSON Schema
			if tool.InputSchema.Properties != nil {
				required := make(map[string]bool)
				for _, r := range tool.InputSchema.Required {
					required[r] = true
				}
				for paramName, prop := range tool.InputSchema.Properties {
					dp := DiscoveredParam{
						Name:     paramName,
						Required: required[paramName],
					}
					// Extract description from property schema
					if propMap, ok := prop.(map[string]any); ok {
						if desc, ok := propMap["description"].(string); ok {
							dp.Description = desc
						}
					}
					dt.Parameters = append(dt.Parameters, dp)
				}
			}

			all = append(all, dt)
		}
	}

	return all, nil
}

// Execute forwards a tool call to the appropriate child MCP server.
func (p *MCPProvider) Execute(toolName string, params map[string]string) (*Result, error) {
	serverName, ok := p.toolMap[toolName]
	if !ok {
		return nil, fmt.Errorf("mcp provider: tool %q not registered", toolName)
	}

	conn, ok := p.servers[serverName]
	if !ok {
		return nil, fmt.Errorf("mcp provider: server %q not connected", serverName)
	}

	remoteName, ok := conn.remoteName[toolName]
	if !ok {
		return nil, fmt.Errorf("mcp provider: no remote mapping for %q", toolName)
	}

	// Convert string params to map[string]any for MCP
	args := make(map[string]any, len(params))
	for k, v := range params {
		args[k] = v
	}

	callReq := mcp.CallToolRequest{}
	callReq.Params.Name = remoteName
	callReq.Params.Arguments = args

	ctx, cancel := context.WithTimeout(context.Background(), mcpTimeout)
	defer cancel()

	start := time.Now()
	result, err := conn.client.CallTool(ctx, callReq)
	duration := time.Since(start)

	if err != nil {
		return &Result{
			Error:    err.Error(),
			ExitCode: 1,
			Duration: duration,
		}, nil
	}

	output := contentToString(result.Content)

	if result.IsError {
		return &Result{
			Output:   output,
			Error:    output,
			ExitCode: 1,
			Duration: duration,
		}, nil
	}

	return &Result{
		Output:   output,
		Duration: duration,
	}, nil
}

// Teardown closes all child MCP server connections.
func (p *MCPProvider) Teardown() error {
	for _, conn := range p.servers {
		conn.client.Close()
	}
	return nil
}

// contentToString extracts text from MCP Content items.
func contentToString(content []mcp.Content) string {
	var parts []string
	for _, c := range content {
		if tc, ok := c.(mcp.TextContent); ok {
			parts = append(parts, tc.Text)
		}
	}
	return strings.Join(parts, "\n")
}
