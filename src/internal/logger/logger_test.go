package logger

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestJSONLLoggerWriteAndRead(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.jsonl")
	l, err := NewJSONL(path)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()

	entry := &Entry{
		Timestamp:  time.Now(),
		Interface:  "cli",
		Tool:       "web.fetch",
		Params:     map[string]string{"url": "https://example.com"},
		Status:     "success",
		DurationMs: 150,
		Output:     "html content",
	}

	if err := l.Log(entry); err != nil {
		t.Fatal(err)
	}

	// Read back
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	var read Entry
	if err := json.Unmarshal(data, &read); err != nil {
		t.Fatal(err)
	}

	if read.Tool != "web.fetch" {
		t.Errorf("expected tool web.fetch, got %s", read.Tool)
	}
	if read.Status != "success" {
		t.Errorf("expected status success, got %s", read.Status)
	}
	if read.Params["url"] != "https://example.com" {
		t.Errorf("expected url param, got %v", read.Params)
	}
	if read.DurationMs != 150 {
		t.Errorf("expected 150ms, got %d", read.DurationMs)
	}
}

func TestJSONLLoggerMultipleEntries(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.jsonl")
	l, err := NewJSONL(path)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()

	for i := 0; i < 5; i++ {
		if err := l.Log(&Entry{
			Timestamp:  time.Now(),
			Interface:  "cli",
			Tool:       "test",
			Params:     map[string]string{},
			Status:     "success",
			DurationMs: int64(i),
		}); err != nil {
			t.Fatal(err)
		}
	}

	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	count := 0
	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "" {
			continue
		}
		var entry Entry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			t.Fatalf("line %d: %v", count, err)
		}
		count++
	}
	if count != 5 {
		t.Errorf("expected 5 entries, got %d", count)
	}
}

func TestJSONLLoggerTruncatesOutput(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.jsonl")
	l, err := NewJSONL(path)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()

	longOutput := strings.Repeat("x", 1000)
	entry := &Entry{
		Timestamp: time.Now(),
		Interface: "cli",
		Tool:      "test",
		Params:    map[string]string{},
		Status:    "success",
		Output:    longOutput,
	}

	if err := l.Log(entry); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	var read Entry
	if err := json.Unmarshal(data, &read); err != nil {
		t.Fatal(err)
	}

	// 500 chars + "..."
	if len(read.Output) != 503 {
		t.Errorf("expected truncated output of 503 chars, got %d", len(read.Output))
	}
	if !strings.HasSuffix(read.Output, "...") {
		t.Error("expected truncated output to end with '...'")
	}
}

func TestJSONLLoggerTruncatesError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.jsonl")
	l, err := NewJSONL(path)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()

	longError := strings.Repeat("e", 1000)
	entry := &Entry{
		Timestamp: time.Now(),
		Interface: "cli",
		Tool:      "test",
		Params:    map[string]string{},
		Status:    "error",
		Error:     longError,
	}

	if err := l.Log(entry); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	var read Entry
	if err := json.Unmarshal(data, &read); err != nil {
		t.Fatal(err)
	}

	// 500 chars + "..."
	if len(read.Error) != 503 {
		t.Errorf("expected truncated error of 503 chars, got %d", len(read.Error))
	}
	if !strings.HasSuffix(read.Error, "...") {
		t.Error("expected truncated error to end with '...'")
	}
}

func TestJSONLLoggerCreatesDir(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "deep", "test.jsonl")

	l, err := NewJSONL(path)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()

	if err := l.Log(&Entry{
		Timestamp: time.Now(),
		Interface: "cli",
		Tool:      "test",
		Params:    map[string]string{},
		Status:    "success",
	}); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Error("expected log file to be created")
	}
}

func TestJSONLLoggerErrorEntry(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.jsonl")
	l, err := NewJSONL(path)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()

	if err := l.Log(&Entry{
		Timestamp: time.Now(),
		Interface: "cli",
		Tool:      "test",
		Params:    map[string]string{},
		Status:    "error",
		Error:     "command not found",
	}); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	var read Entry
	if err := json.Unmarshal(data, &read); err != nil {
		t.Fatal(err)
	}

	if read.Status != "error" {
		t.Errorf("expected status error, got %s", read.Status)
	}
	if read.Error != "command not found" {
		t.Errorf("expected error message, got %q", read.Error)
	}
}

func TestJSONLLoggerFilePermissions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.jsonl")
	l, err := NewJSONL(path)
	if err != nil {
		t.Fatal(err)
	}
	_ = l.Log(&Entry{Timestamp: time.Now(), Tool: "test", Params: map[string]string{}, Status: "success"})
	l.Close()

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	perm := info.Mode().Perm()
	if perm != 0o600 {
		t.Errorf("expected log file permissions 0600, got %04o", perm)
	}
}

func TestNopLogger(t *testing.T) {
	l := NopLogger{}
	if err := l.Log(&Entry{}); err != nil {
		t.Fatal(err)
	}
	if err := l.Close(); err != nil {
		t.Fatal(err)
	}
}
