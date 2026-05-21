// Copyright 2026 Jordan Sherer <hi@jordansherer.com>
// SPDX-License-Identifier: gpl

package builtins

import (
	"fmt"
	"net"
	"net/url"
	"path/filepath"
	"strings"
)

// CheckGuard validates parameters against hardcoded safety rules for built-in tools.
// Returns an error if the call should be blocked. Passing allowOverrides lets
// users whitelist specific patterns that would otherwise be denied.
func CheckGuard(toolName string, params map[string]string, allowOverrides []string) error {
	switch toolName {
	case "factorly.shell":
		return checkShellGuard(params["command"], allowOverrides)
	case "factorly.file.read":
		return checkPathGuard(params["path"], deniedReadPaths, allowOverrides)
	case "factorly.file.write":
		return checkPathGuard(params["path"], deniedWritePaths, allowOverrides)
	case "factorly.fetch":
		return checkURLGuard(params["url"], allowOverrides)
	}
	return nil
}

// IsBuiltinTool returns true if the tool name is a factorly built-in.
func IsBuiltinTool(name string) bool {
	return strings.HasPrefix(name, "factorly.")
}

// --- Shell guard ---

var deniedShellPatterns = []string{
	// Destructive file operations
	"rm -rf /",
	"rm -rf ~",
	"rm -rf .",
	"rm -rf *",
	// Disk destruction
	"mkfs",
	"dd if=",
	"> /dev/sd",
	"> /dev/nvme",
	// Permission escalation
	"chmod -R 777 /",
	"chmod 777 /",
	// Remote code execution
	"curl | sh",
	"curl | bash",
	"wget | sh",
	"wget | bash",
	"curl|sh",
	"curl|bash",
	// Fork bomb
	":(){ :|:& };:",
	// SQL destruction
	"DROP TABLE",
	"DROP DATABASE",
	"TRUNCATE TABLE",
	// System control
	"shutdown",
	"reboot",
	"init 0",
	"init 6",
	"systemctl poweroff",
	"systemctl reboot",
}

func checkShellGuard(command string, allowOverrides []string) error {
	if command == "" {
		return nil
	}
	upper := strings.ToUpper(command)
	for _, pattern := range deniedShellPatterns {
		if strings.Contains(upper, strings.ToUpper(pattern)) {
			// Check allow overrides
			if isAllowed(command, allowOverrides) {
				return nil
			}
			return fmt.Errorf("command blocked by safety guard: matches denied pattern %q", pattern)
		}
	}
	return nil
}

// --- Path guard ---

var deniedReadPaths = []string{
	"/etc/shadow",
	"/etc/passwd",
	"~/.ssh/",
	".ssh/id_",
	".ssh/config",
	".pem",
	".key",
	".env",
	"credentials.json",
	"service-account.json",
	"service_account.json",
}

var deniedWritePaths = append([]string{
	"/etc/",
	"/usr/",
	"/bin/",
	"/sbin/",
	"/boot/",
	"/sys/",
	"/proc/",
	"~/.bashrc",
	"~/.zshrc",
	"~/.profile",
	"~/.bash_profile",
}, deniedReadPaths...)

func checkPathGuard(path string, deniedPatterns []string, allowOverrides []string) error {
	if path == "" {
		return nil
	}
	// Normalize path
	clean := filepath.Clean(path)
	// Expand ~ for matching
	expandedPatterns := make([]string, len(deniedPatterns))
	copy(expandedPatterns, deniedPatterns)

	for _, pattern := range expandedPatterns {
		if matchesPath(clean, path, pattern) {
			if isAllowed(path, allowOverrides) {
				return nil
			}
			return fmt.Errorf("path blocked by safety guard: matches denied pattern %q", pattern)
		}
	}
	return nil
}

func matchesPath(clean, original, pattern string) bool {
	// Check suffix match (e.g., ".env", ".pem", ".key")
	if strings.HasPrefix(pattern, ".") && !strings.HasPrefix(pattern, "./") {
		base := filepath.Base(clean)
		if strings.HasSuffix(base, pattern) || strings.HasPrefix(base, pattern) {
			return true
		}
	}
	// Check prefix match (e.g., "/etc/", "~/.ssh/")
	if strings.HasSuffix(pattern, "/") {
		if strings.HasPrefix(clean, pattern) || strings.HasPrefix(original, pattern) {
			return true
		}
	}
	// Check contains match
	if strings.Contains(clean, pattern) || strings.Contains(original, pattern) {
		return true
	}
	return false
}

// --- URL guard ---

func checkURLGuard(rawURL string, allowOverrides []string) error {
	if rawURL == "" {
		return nil
	}

	// Check allow overrides first
	if isAllowed(rawURL, allowOverrides) {
		return nil
	}

	// Block file:// protocol
	if strings.HasPrefix(strings.ToLower(rawURL), "file://") {
		return fmt.Errorf("URL blocked by safety guard: file:// protocol not allowed")
	}

	parsed, err := url.Parse(rawURL)
	if err != nil {
		return nil // let it fail naturally
	}

	host := parsed.Hostname()

	// Block cloud metadata endpoints
	if host == "169.254.169.254" || host == "metadata.google.internal" {
		return fmt.Errorf("URL blocked by safety guard: cloud metadata endpoint")
	}

	// Block localhost/loopback
	if host == "localhost" || host == "127.0.0.1" || host == "0.0.0.0" || host == "::1" {
		return fmt.Errorf("URL blocked by safety guard: localhost access denied (use allow_urls to override)")
	}

	// Block private networks
	ip := net.ParseIP(host)
	if ip != nil {
		if ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() {
			return fmt.Errorf("URL blocked by safety guard: private network access denied")
		}
	}

	return nil
}

// --- Allow override ---

func isAllowed(value string, allowOverrides []string) bool {
	for _, allowed := range allowOverrides {
		if strings.Contains(value, allowed) {
			return true
		}
	}
	return false
}
