// Copyright 2026 Jordan Sherer <hi@jordansherer.com>
// SPDX-License-Identifier: gpl

package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseToolArgsBasic(t *testing.T) {
	params := parseToolArgs([]string{"--name", "alice", "--count", "3"})
	if params["name"] != "alice" {
		t.Errorf("expected name=alice, got %q", params["name"])
	}
	if params["count"] != "3" {
		t.Errorf("expected count=3, got %q", params["count"])
	}
}

func TestParseToolArgsBoolFlag(t *testing.T) {
	params := parseToolArgs([]string{"--verbose", "--name", "bob"})
	if params["verbose"] != "true" {
		t.Errorf("expected verbose=true, got %q", params["verbose"])
	}
	if params["name"] != "bob" {
		t.Errorf("expected name=bob, got %q", params["name"])
	}
}

func TestParseToolArgsFileRead(t *testing.T) {
	// Write a temp file
	dir := t.TempDir()
	path := filepath.Join(dir, "input.txt")
	if err := os.WriteFile(path, []byte("file content\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	params := parseToolArgs([]string{"--data", "@" + path})
	if params["data"] != "file content" {
		t.Errorf("expected 'file content', got %q", params["data"])
	}
}

func TestParseToolArgsFileReadMissing(t *testing.T) {
	// Missing file should keep the @path as value (with warning)
	params := parseToolArgs([]string{"--data", "@/nonexistent/file.txt"})
	// On error, value stays as the original @path
	if params["data"] != "@/nonexistent/file.txt" {
		t.Errorf("expected @/nonexistent/file.txt on error, got %q", params["data"])
	}
}

func TestParseToolArgsEscapedAt(t *testing.T) {
	params := parseToolArgs([]string{"--email", "@@user"})
	if params["email"] != "@user" {
		t.Errorf("expected @user, got %q", params["email"])
	}
}

func TestParseToolArgsDoubleAtAlone(t *testing.T) {
	params := parseToolArgs([]string{"--prefix", "@@"})
	if params["prefix"] != "@" {
		t.Errorf("expected @, got %q", params["prefix"])
	}
}

func TestParseToolArgsStdin(t *testing.T) {
	// Redirect stdin from a pipe
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	_, _ = w.WriteString("piped input\n")
	w.Close()

	oldStdin := os.Stdin
	os.Stdin = r
	defer func() { os.Stdin = oldStdin }()

	params := parseToolArgs([]string{"--prompt", "-"})
	if params["prompt"] != "piped input" {
		t.Errorf("expected 'piped input', got %q", params["prompt"])
	}
}

func TestParseToolArgsStdinOnlyOnce(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	_, _ = w.WriteString("first\n")
	w.Close()

	oldStdin := os.Stdin
	os.Stdin = r
	defer func() { os.Stdin = oldStdin }()

	// Second "-" should stay as literal "-" since stdin already consumed
	params := parseToolArgs([]string{"--first", "-", "--second", "-"})
	if params["first"] != "first" {
		t.Errorf("expected 'first', got %q", params["first"])
	}
	if params["second"] != "-" {
		t.Errorf("expected literal '-' for second stdin read, got %q", params["second"])
	}
}

func TestParseToolArgsLiteralAt(t *testing.T) {
	// Regular value starting with @ that's not a file
	// (file read fails, keeps original value)
	params := parseToolArgs([]string{"--value", "not-at-prefixed"})
	if params["value"] != "not-at-prefixed" {
		t.Errorf("expected 'not-at-prefixed', got %q", params["value"])
	}
}

func TestParseToolArgsFileMultiline(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "multi.txt")
	if err := os.WriteFile(path, []byte("line1\nline2\nline3\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	params := parseToolArgs([]string{"--body", "@" + path})
	if params["body"] != "line1\nline2\nline3" {
		t.Errorf("expected multiline content, got %q", params["body"])
	}
}
