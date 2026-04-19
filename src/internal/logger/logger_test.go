// Copyright 2026 Jordan Sherer <hi@jordansherer.com>
// SPDX-License-Identifier: gpl

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
	if read.PrevHash != ZeroHash {
		t.Errorf("expected prev_hash=%s for first entry, got %s", ZeroHash, read.PrevHash)
	}
	if read.Hash == "" {
		t.Error("expected non-empty hash")
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
	var entries []Entry
	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "" {
			continue
		}
		var entry Entry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			t.Fatalf("line %d: %v", len(entries), err)
		}
		entries = append(entries, entry)
	}
	if len(entries) != 5 {
		t.Fatalf("expected 5 entries, got %d", len(entries))
	}

	// Verify hash chain
	for i, e := range entries {
		if i == 0 {
			if e.PrevHash != ZeroHash {
				t.Errorf("entry 0: expected prev_hash=%s, got %s", ZeroHash, e.PrevHash)
			}
		} else {
			if e.PrevHash != entries[i-1].Hash {
				t.Errorf("entry %d: prev_hash doesn't link to previous hash", i)
			}
		}
		if e.Hash == "" {
			t.Errorf("entry %d: empty hash", i)
		}
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

func TestHashChainAcrossRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.jsonl")

	// First logger: write 2 entries
	l1, err := NewJSONL(path)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err := l1.Log(&Entry{
			Timestamp: time.Now(),
			Interface: "cli",
			Tool:      "test",
			Params:    map[string]string{},
			Status:    "success",
		}); err != nil {
			t.Fatal(err)
		}
	}
	l1.Close()

	// Second logger: write 1 more entry (should chain from last)
	l2, err := NewJSONL(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := l2.Log(&Entry{
		Timestamp: time.Now(),
		Interface: "cli",
		Tool:      "test",
		Params:    map[string]string{},
		Status:    "success",
	}); err != nil {
		t.Fatal(err)
	}
	l2.Close()

	// Verify the full chain
	verified, skipped, err := VerifyChain(path)
	if err != nil {
		t.Fatalf("chain verification failed: %v", err)
	}
	if verified != 3 {
		t.Errorf("expected 3 verified entries, got %d", verified)
	}
	if skipped != 0 {
		t.Errorf("expected 0 skipped entries, got %d", skipped)
	}
}

func TestVerifyChainDetectsTampering(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.jsonl")

	l, err := NewJSONL(path)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		if err := l.Log(&Entry{
			Timestamp: time.Now(),
			Interface: "cli",
			Tool:      "test",
			Params:    map[string]string{},
			Status:    "success",
			Output:    "original output",
		}); err != nil {
			t.Fatal(err)
		}
	}
	l.Close()

	// Verify clean chain first
	verified, _, err := VerifyChain(path)
	if err != nil {
		t.Fatalf("clean chain should verify: %v", err)
	}
	if verified != 3 {
		t.Fatalf("expected 3 verified, got %d", verified)
	}

	// Tamper with the file: change "original output" to "altered  output" in second line
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	lines[1] = strings.Replace(lines[1], "original output", "altered  output", 1)
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	// Verify should fail
	_, _, err = VerifyChain(path)
	if err == nil {
		t.Fatal("expected chain verification to fail after tampering")
	}
}

func TestGenesisFromEmptyFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.jsonl")

	l, err := NewJSONL(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := l.Log(&Entry{
		Timestamp: time.Now(),
		Interface: "cli",
		Tool:      "test",
		Params:    map[string]string{},
		Status:    "success",
	}); err != nil {
		t.Fatal(err)
	}
	l.Close()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	var read Entry
	if err := json.Unmarshal(data, &read); err != nil {
		t.Fatal(err)
	}

	if read.PrevHash != ZeroHash {
		t.Errorf("genesis entry should have prev_hash=%s, got %s", ZeroHash, read.PrevHash)
	}
	if read.Hash == "" {
		t.Error("genesis entry should have non-empty hash")
	}
}

func TestReadLastHashFromNonexistentFile(t *testing.T) {
	hash := ReadLastHash(filepath.Join(t.TempDir(), "does-not-exist.jsonl"))
	if hash != ZeroHash {
		t.Errorf("expected ZeroHash for nonexistent file, got %s", hash)
	}
}

func TestComputeHashDeterminism(t *testing.T) {
	payload := []byte(`{"tool":"test","status":"success"}`)
	h1 := ComputeHash(ZeroHash, payload)
	h2 := ComputeHash(ZeroHash, payload)
	if h1 != h2 {
		t.Error("ComputeHash should be deterministic")
	}
	if len(h1) != 64 {
		t.Errorf("expected 64-char hex hash, got %d chars", len(h1))
	}

	// Different prev_hash produces different output
	h3 := ComputeHash("aaaa", payload)
	if h1 == h3 {
		t.Error("different prev_hash should produce different hash")
	}

	// Different payload produces different output
	h4 := ComputeHash(ZeroHash, []byte(`{"tool":"other"}`))
	if h1 == h4 {
		t.Error("different payload should produce different hash")
	}
}

func TestHashRecomputation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.jsonl")
	l, err := NewJSONL(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := l.Log(&Entry{
		Timestamp: time.Now(),
		Interface: "cli",
		Tool:      "test",
		Params:    map[string]string{"key": "value"},
		Status:    "success",
	}); err != nil {
		t.Fatal(err)
	}
	l.Close()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	var entry Entry
	if err := json.Unmarshal(data, &entry); err != nil {
		t.Fatal(err)
	}

	// Recompute: clear hash, marshal, compute
	recordedHash := entry.Hash
	entry.Hash = ""
	payload, err := json.Marshal(&entry)
	if err != nil {
		t.Fatal(err)
	}
	computed := ComputeHash(entry.PrevHash, payload)
	if computed != recordedHash {
		t.Errorf("recomputed hash %s != recorded %s", computed, recordedHash)
	}
}

func TestVerifyChainPreUpgradeEntries(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.jsonl")

	// Write pre-upgrade entries (no hash fields) manually
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		line, _ := json.Marshal(map[string]any{
			"timestamp":   time.Now(),
			"interface":   "cli",
			"tool":        "test",
			"params":      map[string]string{},
			"status":      "success",
			"duration_ms": i,
		})
		_, _ = f.Write(append(line, '\n'))
	}
	f.Close()

	// Now open logger and write a post-upgrade entry
	l, err := NewJSONL(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := l.Log(&Entry{
		Timestamp: time.Now(),
		Interface: "cli",
		Tool:      "test",
		Params:    map[string]string{},
		Status:    "success",
	}); err != nil {
		t.Fatal(err)
	}
	l.Close()

	verified, skipped, err := VerifyChain(path)
	if err != nil {
		t.Fatalf("verification should pass with mixed entries: %v", err)
	}
	if skipped != 3 {
		t.Errorf("expected 3 skipped pre-upgrade entries, got %d", skipped)
	}
	if verified != 1 {
		t.Errorf("expected 1 verified entry, got %d", verified)
	}
}

func TestVerifyChainDeletedEntry(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.jsonl")

	l, err := NewJSONL(path)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 4; i++ {
		if err := l.Log(&Entry{
			Timestamp: time.Now(),
			Interface: "cli",
			Tool:      "test",
			Params:    map[string]string{},
			Status:    "success",
		}); err != nil {
			t.Fatal(err)
		}
	}
	l.Close()

	// Remove the second entry
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	tampered := append([]string{lines[0]}, lines[2:]...)
	if err := os.WriteFile(path, []byte(strings.Join(tampered, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	_, _, err = VerifyChain(path)
	if err == nil {
		t.Fatal("expected verification to fail when entry is deleted")
	}
}

func TestVerifyChainEmptyFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.jsonl")
	if err := os.WriteFile(path, []byte{}, 0o600); err != nil {
		t.Fatal(err)
	}

	verified, skipped, err := VerifyChain(path)
	if err != nil {
		t.Fatalf("empty file should verify cleanly: %v", err)
	}
	if verified != 0 || skipped != 0 {
		t.Errorf("expected 0/0 for empty file, got %d/%d", verified, skipped)
	}
}

func TestVerifyChainNonexistentFile(t *testing.T) {
	_, _, err := VerifyChain(filepath.Join(t.TempDir(), "nope.jsonl"))
	if err == nil {
		t.Error("expected error for nonexistent file")
	}
}
