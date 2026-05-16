// Copyright 2026 Jordan Sherer <hi@jordansherer.com>
// SPDX-License-Identifier: gpl

package provider

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestBuiltinReadFile(t *testing.T) {
	tmp := filepath.Join(t.TempDir(), "test.txt")
	if err := os.WriteFile(tmp, []byte("hello world"), 0644); err != nil {
		t.Fatal(err)
	}

	bp := NewBuiltinProvider("stdio", "")
	result, err := bp.Execute("factorly.read_file", map[string]string{"path": tmp})
	if err != nil {
		t.Fatal(err)
	}
	if result.Output != "hello world" {
		t.Errorf("got %q, want %q", result.Output, "hello world")
	}
	if result.ExitCode != 0 {
		t.Errorf("got exit code %d, want 0", result.ExitCode)
	}
}

func TestBuiltinReadFile_NotFound(t *testing.T) {
	bp := NewBuiltinProvider("stdio", "")
	result, err := bp.Execute("factorly.read_file", map[string]string{"path": "/nonexistent/file.txt"})
	if err != nil {
		t.Fatal(err)
	}
	if result.ExitCode != 1 {
		t.Errorf("got exit code %d, want 1", result.ExitCode)
	}
	if result.Error == "" {
		t.Error("expected error message")
	}
}

func TestBuiltinWriteFile(t *testing.T) {
	tmp := filepath.Join(t.TempDir(), "sub", "out.txt")

	bp := NewBuiltinProvider("stdio", "")
	result, err := bp.Execute("factorly.write_file", map[string]string{
		"path":    tmp,
		"content": "test content",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.ExitCode != 0 {
		t.Errorf("got exit code %d, want 0", result.ExitCode)
	}

	data, err := os.ReadFile(tmp)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "test content" {
		t.Errorf("got %q, want %q", string(data), "test content")
	}
}

func TestBuiltinShell(t *testing.T) {
	bp := NewBuiltinProvider("stdio", "")
	result, err := bp.Execute("factorly.shell", map[string]string{"command": "echo hello"})
	if err != nil {
		t.Fatal(err)
	}
	if result.ExitCode != 0 {
		t.Errorf("got exit code %d, want 0", result.ExitCode)
	}
	if result.Output != "hello\n" {
		t.Errorf("got %q, want %q", result.Output, "hello\n")
	}
}

func TestBuiltinShell_ExitCode(t *testing.T) {
	bp := NewBuiltinProvider("stdio", "")
	result, err := bp.Execute("factorly.shell", map[string]string{"command": "exit 42"})
	if err != nil {
		t.Fatal(err)
	}
	if result.ExitCode != 42 {
		t.Errorf("got exit code %d, want 42", result.ExitCode)
	}
}

func TestBuiltinFetch_BadScheme(t *testing.T) {
	bp := NewBuiltinProvider("stdio", "")
	result, err := bp.Execute("factorly.fetch", map[string]string{"url": "ftp://evil.com"})
	if err != nil {
		t.Fatal(err)
	}
	if result.ExitCode != 1 {
		t.Errorf("got exit code %d, want 1", result.ExitCode)
	}
	if result.Error == "" {
		t.Error("expected error for non-http scheme")
	}
}

func TestBuiltinProvider_HttpMode(t *testing.T) {
	bp := NewBuiltinProvider("http", "")

	// fetch should work
	_, err := bp.Execute("factorly.fetch", map[string]string{"url": "https://example.com"})
	if err != nil {
		t.Errorf("fetch should work in http mode: %v", err)
	}

	// local tools should not be available
	_, err = bp.Execute("factorly.read_file", map[string]string{"path": "/tmp/x"})
	if err == nil {
		t.Error("read_file should not be available in http mode")
	}

	_, err = bp.Execute("factorly.shell", map[string]string{"command": "echo x"})
	if err == nil {
		t.Error("shell should not be available in http mode")
	}
}

func TestBuiltinProvider_UnknownTool(t *testing.T) {
	bp := NewBuiltinProvider("stdio", "")
	_, err := bp.Execute("factorly.nonexistent", map[string]string{})
	if err == nil {
		t.Error("expected error for unknown tool")
	}
}

func TestBuiltinReadFile_ScopedToProject(t *testing.T) {
	root := t.TempDir()
	// Create a file inside project
	inner := filepath.Join(root, "data.txt")
	if err := os.WriteFile(inner, []byte("inside"), 0644); err != nil {
		t.Fatal(err)
	}
	// Create a file outside project
	outside := filepath.Join(t.TempDir(), "secret.txt")
	if err := os.WriteFile(outside, []byte("secret"), 0644); err != nil {
		t.Fatal(err)
	}

	bp := NewBuiltinProvider("stdio", root)

	// Should succeed for file inside project
	result, err := bp.Execute("factorly.read_file", map[string]string{"path": "data.txt"})
	if err != nil {
		t.Fatal(err)
	}
	if result.ExitCode != 0 {
		t.Errorf("expected success, got error: %s", result.Error)
	}
	if result.Output != "inside" {
		t.Errorf("got %q, want %q", result.Output, "inside")
	}

	// Should fail for absolute path outside project
	result, err = bp.Execute("factorly.read_file", map[string]string{"path": outside})
	if err != nil {
		t.Fatal(err)
	}
	if result.ExitCode != 1 {
		t.Error("expected error for path outside project")
	}

	// Should fail for traversal attempt
	result, err = bp.Execute("factorly.read_file", map[string]string{"path": "../../../etc/passwd"})
	if err != nil {
		t.Fatal(err)
	}
	if result.ExitCode != 1 {
		t.Error("expected error for path traversal")
	}
}

func TestBuiltinWriteFile_ScopedToProject(t *testing.T) {
	root := t.TempDir()
	bp := NewBuiltinProvider("stdio", root)

	// Should succeed within project
	result, err := bp.Execute("factorly.write_file", map[string]string{
		"path":    "output.txt",
		"content": "hello",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.ExitCode != 0 {
		t.Errorf("expected success, got error: %s", result.Error)
	}

	// Should fail for path outside project
	result, err = bp.Execute("factorly.write_file", map[string]string{
		"path":    "/tmp/escape.txt",
		"content": "bad",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.ExitCode != 1 {
		t.Error("expected error for path outside project")
	}
}

// TestBuiltinShell_HonorsParentCtxCancel guards the ctx-threading
// foundation: a parent ctx cancellation must preempt the shell's own
// 30s internal timeout. Without ctx-propagation a `sleep 5` invoked
// inside an outer 200ms-deadline call would still run to completion.
func TestBuiltinShell_HonorsParentCtxCancel(t *testing.T) {
	bp := NewBuiltinProvider("stdio", "")
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	start := time.Now()
	result, err := bp.ExecuteWithContext(ctx, "factorly.shell", map[string]string{
		"command": "sleep 5",
	})
	elapsed := time.Since(start)
	if err != nil {
		t.Fatal(err)
	}
	if elapsed > 2*time.Second {
		t.Errorf("ctx cancel didn't propagate; ran for %s", elapsed)
	}
	if result.ExitCode == 0 {
		t.Errorf("expected nonzero exit on cancel, got 0; output=%q", result.Output)
	}
}

// TestBuiltinReadFile_PreChecksCtx guards the cheap pre-check we added
// to file handlers: an already-canceled ctx short-circuits before the
// I/O. Without it, the read would still happen and we'd waste the
// disk hit even though the caller has given up.
func TestBuiltinReadFile_PreChecksCtx(t *testing.T) {
	tmp := filepath.Join(t.TempDir(), "test.txt")
	if err := os.WriteFile(tmp, []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}
	bp := NewBuiltinProvider("stdio", "")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	result, err := bp.ExecuteWithContext(ctx, "factorly.read_file", map[string]string{"path": tmp})
	if err != nil {
		t.Fatal(err)
	}
	if result.ExitCode != 1 {
		t.Errorf("expected nonzero exit on canceled ctx, got %d", result.ExitCode)
	}
	if !strings.Contains(result.Error, "canceled") {
		t.Errorf("expected ctx-canceled error, got %q", result.Error)
	}
}
