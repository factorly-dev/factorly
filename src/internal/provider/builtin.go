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

// BuiltinHandler executes a built-in tool in-process. Handlers receive
// the caller's context so deadlines and cancellation propagate (e.g.,
// an outer code-tool timeout cancels the inner shell command).
// Handlers may layer their own timeout on top via context.WithTimeout.
type BuiltinHandler func(ctx context.Context, params map[string]string) (*Result, error)

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

// Execute satisfies provider.Provider. Uses context.Background() — the
// proxy's ExecuteWithContext path on the BuiltinProvider plumbs the
// real caller context through to handlers; this no-ctx entry point
// stays here for compatibility with the existing interface.
func (bp *BuiltinProvider) Execute(toolName string, params map[string]string) (*Result, error) {
	return bp.ExecuteWithContext(context.Background(), toolName, params)
}

// ExecuteWithContext runs the builtin handler for toolName with the
// caller-supplied context. Handlers honor ctx — deadlines, cancellation,
// and AfterFunc all propagate.
func (bp *BuiltinProvider) ExecuteWithContext(ctx context.Context, toolName string, params map[string]string) (*Result, error) {
	h, ok := bp.handlers[toolName]
	if !ok {
		return nil, fmt.Errorf("builtin: unknown tool %q", toolName)
	}
	start := time.Now()
	result, err := h(ctx, params)
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
// ctx is checked before the read so an already-canceled caller doesn't
// pay the I/O cost; stdlib file I/O is itself uncancellable mid-read.
func (bp *BuiltinProvider) builtinReadFile(ctx context.Context, params map[string]string) (*Result, error) {
	if err := ctx.Err(); err != nil {
		return &Result{Error: err.Error(), ExitCode: 1}, nil
	}
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
// Same ctx-pre-check pattern as builtinReadFile.
func (bp *BuiltinProvider) builtinWriteFile(ctx context.Context, params map[string]string) (*Result, error) {
	if err := ctx.Err(); err != nil {
		return &Result{Error: err.Error(), ExitCode: 1}, nil
	}
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

// builtinShell executes a shell command with a 30s default timeout.
// The caller's ctx is honored — a caller deadline (e.g., from an outer
// code-tool timeout) preempts the shell's own 30s when shorter.
func builtinShell(ctx context.Context, params map[string]string) (*Result, error) {
	command := params["command"]
	if command == "" {
		return &Result{Error: "command is required", ExitCode: 1}, nil
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	shell, flag := shellCmd()
	cmd := exec.CommandContext(ctx, shell, flag, command)
	// Run the shell + its children in a new process group on Unix so
	// ctx cancellation kills the whole group (sh + any child like sleep).
	// Without this, `sh -c "sleep 5"` survives ctx cancellation because
	// SIGKILL goes to sh but not to its sleep child.
	setShellProcessGroup(cmd)
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	exitCode := 0
	if err != nil {
		// Check ctx first: when the parent ctx is canceled, exec.Cmd.Run
		// returns a "signal: killed" *exec.ExitError. Falling into the
		// generic ExitError branch would mask the ctx cancellation as a
		// normal nonzero exit. Surface the cancel reason instead.
		if ctxErr := ctx.Err(); ctxErr != nil {
			msg := "command canceled"
			if ctxErr == context.DeadlineExceeded {
				msg = "command timed out"
			}
			return &Result{Error: msg + ": " + ctxErr.Error(), ExitCode: 124}, nil
		}
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
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

// builtinFetch performs an HTTP GET using net/http. Honors the caller's
// ctx in addition to a 30s default request timeout — whichever fires
// first cancels the request.
func builtinFetch(ctx context.Context, params map[string]string) (*Result, error) {
	url := params["url"]
	if url == "" {
		return &Result{Error: "url is required", ExitCode: 1}, nil
	}
	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		return &Result{Error: "url must start with http:// or https://", ExitCode: 1}, nil
	}

	reqCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, url, nil)
	if err != nil {
		return &Result{Error: err.Error(), ExitCode: 1}, nil
	}
	client := &http.Client{}
	resp, err := client.Do(req)
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

// builtinClipboard copies text to the system clipboard. Honors the
// caller's ctx so cancellation cuts a stuck clipboard process short.
func builtinClipboard(ctx context.Context, params map[string]string) (*Result, error) {
	text := params["text"]
	if text == "" {
		return &Result{Error: "text is required", ExitCode: 1}, nil
	}

	cmd, args := clipboardCmd()
	c := exec.CommandContext(ctx, cmd, args...)
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
