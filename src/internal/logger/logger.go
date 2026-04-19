// Copyright 2026 Jordan Sherer <hi@jordansherer.com>
// SPDX-License-Identifier: gpl

package logger

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// ZeroHash is the prev_hash for the first entry in a chain.
const ZeroHash = "0000000000000000000000000000000000000000000000000000000000000000"

type Entry struct {
	Timestamp       time.Time         `json:"timestamp"`
	Interface       string            `json:"interface"`
	Tool            string            `json:"tool"`
	Params          map[string]string `json:"params"`
	Status          string            `json:"status"`
	DurationMs      int64             `json:"duration_ms"`
	Output          string            `json:"output,omitempty"`
	Error           string            `json:"error,omitempty"`
	ShadowAction    string            `json:"shadow_action,omitempty"`
	HighlightParams map[string]string `json:"highlight_params,omitempty"`
	AgentID         string            `json:"agent_id,omitempty"`
	OriginalBytes   int               `json:"original_bytes,omitempty"`
	ProcessedBytes  int               `json:"processed_bytes,omitempty"`
	PrevHash        string            `json:"prev_hash,omitempty"`
	Hash            string            `json:"hash,omitempty"`
}

type Logger interface {
	Log(entry *Entry) error
	Close() error
}

type JSONLLogger struct {
	f        *os.File
	mu       sync.Mutex
	prevHash string
}

func NewJSONL(path string) (*JSONLLogger, error) {
	if path == "" {
		path = DefaultLogPath()
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}

	// Read last hash before opening in write-only append mode
	prevHash := ReadLastHash(path)

	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, err
	}

	return &JSONLLogger{f: f, prevHash: prevHash}, nil
}

func (l *JSONLLogger) Log(entry *Entry) error {
	// Truncate output and error to 500 chars
	if len(entry.Output) > 500 {
		entry.Output = entry.Output[:500] + "..."
	}
	if len(entry.Error) > 500 {
		entry.Error = entry.Error[:500] + "..."
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	// Set prev_hash, clear hash for deterministic payload
	entry.PrevHash = l.prevHash
	entry.Hash = ""

	payload, err := json.Marshal(entry)
	if err != nil {
		return err
	}

	hash := ComputeHash(entry.PrevHash, payload)
	entry.Hash = hash

	data, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	data = append(data, '\n')

	_, err = l.f.Write(data)
	if err != nil {
		return err
	}
	l.prevHash = hash
	return nil
}

func (l *JSONLLogger) Close() error {
	return l.f.Close()
}

func DefaultLogPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "factorly-calls.jsonl"
	}
	return filepath.Join(home, ".config", "factorly", "calls.jsonl")
}

// ComputeHash returns the hex-encoded SHA-256 of prevHash + payload.
func ComputeHash(prevHash string, payload []byte) string {
	h := sha256.New()
	h.Write([]byte(prevHash))
	h.Write(payload)
	return hex.EncodeToString(h.Sum(nil))
}

// ReadLastHash reads the last entry's hash from the log file.
// Returns ZeroHash if the file doesn't exist, is empty, or the last entry has no hash.
func ReadLastHash(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ZeroHash
	}
	defer f.Close()

	fi, err := f.Stat()
	if err != nil || fi.Size() == 0 {
		return ZeroHash
	}

	// Seek to last 4KB — generous for a single JSONL line
	offset := fi.Size() - 4096
	if offset < 0 {
		offset = 0
	}
	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		return ZeroHash
	}

	var lastLine string
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 512*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) != "" {
			lastLine = line
		}
	}
	if lastLine == "" {
		return ZeroHash
	}

	var entry Entry
	if err := json.Unmarshal([]byte(lastLine), &entry); err != nil || entry.Hash == "" {
		return ZeroHash
	}
	return entry.Hash
}

// VerifyChain validates the hash chain in the log file.
// Returns the number of verified and skipped (pre-upgrade) entries.
func VerifyChain(path string) (verified, skipped int, err error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, 0, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 512*1024)

	lineNum := 0
	prevHash := ZeroHash

	for scanner.Scan() {
		lineNum++
		line := scanner.Bytes()

		var entry Entry
		if err := json.Unmarshal(line, &entry); err != nil {
			continue
		}

		// Pre-upgrade entries without hash — skip
		if entry.Hash == "" {
			skipped++
			continue
		}

		if entry.PrevHash != prevHash {
			return verified, skipped, &ChainError{
				Line:     lineNum,
				Message:  "chain broken",
				Expected: prevHash,
				Got:      entry.PrevHash,
			}
		}

		// Recompute hash
		recordedHash := entry.Hash
		entry.Hash = ""
		payload, err := json.Marshal(&entry)
		if err != nil {
			return verified, skipped, &ChainError{
				Line:    lineNum,
				Message: "marshal error: " + err.Error(),
			}
		}

		computed := ComputeHash(entry.PrevHash, payload)
		if computed != recordedHash {
			return verified, skipped, &ChainError{
				Line:     lineNum,
				Message:  "hash mismatch",
				Expected: computed,
				Got:      recordedHash,
			}
		}

		prevHash = recordedHash
		verified++
	}

	if err := scanner.Err(); err != nil {
		return verified, skipped, err
	}
	return verified, skipped, nil
}

// ChainError describes a hash chain verification failure.
type ChainError struct {
	Line     int
	Message  string
	Expected string
	Got      string
}

func (e *ChainError) Error() string {
	if e.Expected != "" {
		return fmt.Sprintf("line %d: %s: expected %s, got %s", e.Line, e.Message, e.Expected, e.Got)
	}
	return fmt.Sprintf("line %d: %s", e.Line, e.Message)
}

// NopLogger is a no-op logger for testing or when logging is disabled.
type NopLogger struct{}

func (NopLogger) Log(*Entry) error { return nil }
func (NopLogger) Close() error     { return nil }
