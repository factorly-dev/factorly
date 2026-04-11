package naming

import (
	"net/url"
	"path"
	"strings"
)

// DeriveNameFromCommand extracts a human-friendly name from a command and its args.
// e.g., "npx @modelcontextprotocol/server-github" → "github"
// e.g., "uvx mcp-server-fetch" → "fetch"
// e.g., "python -m my_server" → "my_server"
func DeriveNameFromCommand(args []string) string {
	// Look at the last meaningful argument
	for i := len(args) - 1; i >= 0; i-- {
		arg := args[i]
		// Skip flags
		if strings.HasPrefix(arg, "-") {
			continue
		}
		// Handle scoped npm packages: @org/name → name
		if strings.Contains(arg, "/") {
			parts := strings.Split(arg, "/")
			arg = parts[len(parts)-1]
		}
		// Clean up common prefixes/suffixes
		name := strings.TrimPrefix(arg, "mcp-server-")
		name = strings.TrimPrefix(name, "server-")
		name = strings.TrimSuffix(name, ".py")
		name = strings.TrimSuffix(name, ".js")
		if name != "" && name != "npx" && name != "uvx" && name != "node" && name != "python" && name != "python3" {
			return Sanitize(name)
		}
	}
	return "wrapped"
}

// DeriveNameFromURL extracts a name from an HTTP URL.
// e.g., "http://localhost:3001/mcp" → "mcp"
// e.g., "http://my-server:8080" → "my-server"
func DeriveNameFromURL(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "wrapped"
	}
	host := u.Hostname()
	if host == "" {
		return "wrapped"
	}
	// Use path if it's meaningful
	if p := path.Base(u.Path); p != "" && p != "/" && p != "." {
		return Sanitize(p)
	}
	return Sanitize(host)
}

// Sanitize cleans a string for use as a tool config key.
func Sanitize(s string) string {
	s = strings.ReplaceAll(s, " ", "-")
	s = strings.ReplaceAll(s, ".", "-")
	s = strings.ToLower(s)
	return s
}
