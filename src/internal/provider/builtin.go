// Copyright 2026 Jordan Sherer <hi@jordansherer.com>
// SPDX-License-Identifier: gpl

package provider

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// BuiltinHandler is a function that executes a built-in tool in-process.
type BuiltinHandler func(params map[string]string) (*Result, error)

// BuiltinProvider executes built-in tools in-process without forking subprocesses.
type BuiltinProvider struct {
	handlers map[string]BuiltinHandler
	rootDir  string // project root — file operations are scoped to this directory
}

// NewBuiltinProvider creates a provider with in-process handlers for built-in tools.
// Mode controls which tools are available: "http" excludes local-only tools.
// rootDir scopes file read/write operations to the project directory.
func NewBuiltinProvider(mode string, rootDir string) *BuiltinProvider {
	bp := &BuiltinProvider{
		handlers: make(map[string]BuiltinHandler),
		rootDir:  rootDir,
	}

	// Universal — available in all modes
	bp.handlers["factorly.fetch"] = builtinFetch

	// Local only — stdio mode
	if mode != "http" {
		bp.handlers["factorly.read_file"] = bp.builtinReadFile
		bp.handlers["factorly.write_file"] = bp.builtinWriteFile
		bp.handlers["factorly.shell"] = builtinShell
		bp.handlers["factorly.clipboard"] = builtinClipboard
	}

	return bp
}

func (bp *BuiltinProvider) Setup() error    { return nil }
func (bp *BuiltinProvider) Teardown() error { return nil }

func (bp *BuiltinProvider) Execute(toolName string, params map[string]string) (*Result, error) {
	h, ok := bp.handlers[toolName]
	if !ok {
		return nil, fmt.Errorf("builtin: unknown tool %q", toolName)
	}
	start := time.Now()
	result, err := h(params)
	if err != nil {
		return nil, err
	}
	result.Duration = time.Since(start)
	return result, nil
}

// scopedPath resolves and validates that a path is within the project root.
// Returns the absolute path or an error result if out of bounds.
func (bp *BuiltinProvider) scopedPath(path string) (string, *Result) {
	if path == "" {
		return "", &Result{Error: "path is required", ExitCode: 1}
	}
	// Resolve relative paths against rootDir
	abs := path
	if bp.rootDir != "" && !filepath.IsAbs(path) {
		abs = filepath.Join(bp.rootDir, path)
	}
	abs = filepath.Clean(abs)

	// Verify the resolved path is within rootDir
	if bp.rootDir != "" {
		root := filepath.Clean(bp.rootDir)
		if !strings.HasPrefix(abs, root+string(filepath.Separator)) && abs != root {
			return "", &Result{
				Error:    fmt.Sprintf("path %q is outside project directory %q", path, root),
				ExitCode: 1,
			}
		}
	}
	return abs, nil
}

// builtinReadFile reads a file using os.ReadFile, scoped to project dir.
func (bp *BuiltinProvider) builtinReadFile(params map[string]string) (*Result, error) {
	abs, errResult := bp.scopedPath(params["path"])
	if errResult != nil {
		return errResult, nil
	}
	data, err := os.ReadFile(abs)
	if err != nil {
		return &Result{Error: err.Error(), ExitCode: 1}, nil
	}
	return &Result{Output: string(data)}, nil
}

// builtinWriteFile writes content to a file using os.WriteFile, scoped to project dir.
func (bp *BuiltinProvider) builtinWriteFile(params map[string]string) (*Result, error) {
	abs, errResult := bp.scopedPath(params["path"])
	if errResult != nil {
		return errResult, nil
	}
	content := params["content"]
	if err := os.MkdirAll(filepath.Dir(abs), 0755); err != nil {
		return &Result{Error: err.Error(), ExitCode: 1}, nil
	}
	if err := os.WriteFile(abs, []byte(content), 0644); err != nil {
		return &Result{Error: err.Error(), ExitCode: 1}, nil
	}
	return &Result{Output: fmt.Sprintf("wrote %d bytes to %s", len(content), abs)}, nil
}


// shellCmd returns the platform-appropriate shell and flag for command execution.
func shellCmd() (string, string) {
	if runtime.GOOS == "windows" {
		return "cmd", "/C"
	}
	return "sh", "-c"
}

// builtinShell executes a shell command with a timeout.
func builtinShell(params map[string]string) (*Result, error) {
	command := params["command"]
	if command == "" {
		return &Result{Error: "command is required", ExitCode: 1}, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	shell, flag := shellCmd()
	cmd := exec.CommandContext(ctx, shell, flag, command)
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else if ctx.Err() == context.DeadlineExceeded {
			return &Result{Error: "command timed out after 30s", ExitCode: 124}, nil
		} else {
			return &Result{Error: err.Error(), ExitCode: 1}, nil
		}
	}
	return &Result{
		Output:   stdout.String(),
		Error:    stderr.String(),
		ExitCode: exitCode,
	}, nil
}

// builtinFetch performs an HTTP GET using net/http.
func builtinFetch(params map[string]string) (*Result, error) {
	url := params["url"]
	if url == "" {
		return &Result{Error: "url is required", ExitCode: 1}, nil
	}
	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		return &Result{Error: "url must start with http:// or https://", ExitCode: 1}, nil
	}

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return &Result{Error: err.Error(), ExitCode: 1}, nil
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20)) // 1MB limit
	if err != nil {
		return &Result{Error: err.Error(), ExitCode: 1}, nil
	}

	if resp.StatusCode >= 400 {
		return &Result{
			Output:   string(body),
			Error:    fmt.Sprintf("HTTP %d %s", resp.StatusCode, resp.Status),
			ExitCode: 1,
		}, nil
	}
	return &Result{Output: string(body)}, nil
}

// builtinClipboard copies text to the system clipboard.
func builtinClipboard(params map[string]string) (*Result, error) {
	text := params["text"]
	if text == "" {
		return &Result{Error: "text is required", ExitCode: 1}, nil
	}

	cmd, args := clipboardCmd()
	c := exec.Command(cmd, args...)
	c.Stdin = strings.NewReader(text)

	if err := c.Run(); err != nil {
		return &Result{Error: err.Error(), ExitCode: 1}, nil
	}
	return &Result{Output: fmt.Sprintf("copied %d bytes to clipboard", len(text))}, nil
}

// clipboardCmd returns the platform-specific clipboard command.
func clipboardCmd() (string, []string) {
	switch runtime.GOOS {
	case "darwin":
		return "pbcopy", nil
	case "windows":
		return "clip", nil
	default:
		if _, err := exec.LookPath("xsel"); err == nil {
			return "xsel", []string{"--clipboard", "--input"}
		}
		return "xclip", []string{"-selection", "clipboard"}
	}
}
