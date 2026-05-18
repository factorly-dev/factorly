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

	"github.com/factorly-dev/factorly/internal/projectpath"
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
	VaultKeys       []string          `json:"vault_keys,omitempty"`
	OriginalParams  map[string]string `json:"original_params,omitempty"`
	OriginalBytes   int               `json:"original_bytes,omitempty"`
	ProcessedBytes  int               `json:"processed_bytes,omitempty"`
	PrevHash        string            `json:"prev_hash,omitempty"`
	Hash            string            `json:"hash,omitempty"`
	// SourceSHA is the SHA-256 (hex) of a code-tool's script body when
	// the call ran inside the code provider, or the agent-supplied
	// source body when the future factorly.code builtin invoked yaegi.
	// Lets the audit log identify "what code actually ran" without
	// inlining the source itself.
	SourceSHA string `json:"source_sha,omitempty"`
	// Workspace is the name of the active workspace overlay (if any).
	// Empty when the call ran outside a workspace context.
	Workspace string `json:"workspace,omitempty"`
}

type Logger interface {
	Log(entry *Entry) error
	Close() error
}

// WithWorkspace wraps a Logger so every entry it logs carries the
// given workspace name on its Workspace field. Empty name returns
// the inner logger unchanged.
func WithWorkspace(inner Logger, workspace string) Logger {
	if workspace == "" {
		return inner
	}
	return &workspaceLogger{inner: inner, workspace: workspace}
}

type workspaceLogger struct {
	inner     Logger
	workspace string
}

func (w *workspaceLogger) Log(e *Entry) error {
	if e != nil && e.Workspace == "" {
		e.Workspace = w.workspace
	}
	return w.inner.Log(e)
}

func (w *workspaceLogger) Close() error {
	return w.inner.Close()
}

type JSONLLogger struct {
	path     string
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

	return &JSONLLogger{path: path, f: f, prevHash: prevHash}, nil
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

	// Re-read last hash from file to handle multi-process writes.
	// Another process (e.g., CLI while MCP server is running) may have
	// appended entries since we last wrote.
	if fileHash := ReadLastHash(l.path); fileHash != l.prevHash {
		l.prevHash = fileHash
	}

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
		return "factorly-audit.jsonl"
	}
	return filepath.Join(home, ".config", "factorly", "audit.jsonl")
}

// ProjectLogPath returns the log path that pairs with the given config
// path. A project-scoped config (one not under the user's global
// config dir) gets its log under that project's .factorly/ dir, so
// audit history travels with the repo. Anything else falls back to
// DefaultLogPath. Empty cfgPath also yields the global default.
func ProjectLogPath(cfgPath string) string {
	return projectpath.Resolve(cfgPath, "audit.jsonl", DefaultLogPath())
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

	// Collect all entries
	type parsedLine struct {
		lineNum int
		raw     []byte
		entry   *Entry
	}
	var lines []parsedLine
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		raw := make([]byte, len(scanner.Bytes()))
		copy(raw, scanner.Bytes())
		var entry Entry
		if err := json.Unmarshal(raw, &entry); err != nil {
			lines = append(lines, parsedLine{lineNum, raw, nil})
			continue
		}
		lines = append(lines, parsedLine{lineNum, raw, &entry})
	}

	// Find the last chain_reset marker
	lastReset := -1
	for i, l := range lines {
		if l.entry != nil && IsChainReset(l.entry) {
			lastReset = i
		}
	}

	// Determine verification start: after the last reset, or from the beginning
	startIdx := 0
	prevHash := ZeroHash

	if lastReset >= 0 {
		resetEntry := lines[lastReset].entry
		// Verify the reset marker's own hash
		recordedHash := resetEntry.Hash
		resetCopy := *resetEntry
		resetCopy.Hash = ""
		payload, _ := json.Marshal(&resetCopy)
		computed := ComputeHash(resetCopy.PrevHash, payload)
		if computed != recordedHash {
			return 0, 0, &ChainError{
				Line:     lines[lastReset].lineNum,
				Message:  "invalid chain_reset marker",
				Expected: computed,
				Got:      recordedHash,
			}
		}
		prevHash = recordedHash
		verified = 1 // count the reset marker
		startIdx = lastReset + 1
		// Everything before the reset is skipped
		for i := 0; i < lastReset; i++ {
			if lines[i].entry != nil {
				skipped++
			}
		}
	}

	// Verify chain from start point
	for i := startIdx; i < len(lines); i++ {
		l := lines[i]
		if l.entry == nil {
			continue
		}

		// Pre-upgrade entries without hash — skip
		if l.entry.Hash == "" {
			skipped++
			continue
		}

		if l.entry.PrevHash != prevHash {
			return verified, skipped, &ChainError{
				Line:     l.lineNum,
				Message:  "chain broken",
				Expected: prevHash,
				Got:      l.entry.PrevHash,
			}
		}

		// Recompute hash
		recordedHash := l.entry.Hash
		entryCopy := *l.entry
		entryCopy.Hash = ""
		payload, err := json.Marshal(&entryCopy)
		if err != nil {
			return verified, skipped, &ChainError{
				Line:    l.lineNum,
				Message: "marshal error: " + err.Error(),
			}
		}

		computed := ComputeHash(entryCopy.PrevHash, payload)
		if computed != recordedHash {
			return verified, skipped, &ChainError{
				Line:     l.lineNum,
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

// RepairChain appends a chain_reset marker to the log file. The marker's
// prev_hash is a SHA-256 hash of the entire file contents before the marker,
// binding it to the complete history. New entries chain from the marker's hash.
// Existing entries are never modified. Returns true if a reset was appended.
func RepairChain(path string) (bool, error) {
	// Check if repair is needed
	_, _, verifyErr := VerifyChain(path)
	if verifyErr == nil {
		return false, nil
	}

	// Hash the entire file contents
	fileData, err := os.ReadFile(path)
	if err != nil {
		return false, err
	}
	fileHash := sha256.Sum256(fileData)
	fileHashHex := hex.EncodeToString(fileHash[:])

	// Build the reset marker
	resetEntry := &Entry{
		Timestamp: time.Now(),
		Interface: "system",
		Tool:      "chain_reset",
		Status:    "repair",
		Output:    fmt.Sprintf("chain repaired — prev_hash is SHA-256 of file contents (%d bytes)", len(fileData)),
		PrevHash:  fileHashHex,
	}
	resetEntry.Hash = ""
	payload, err := json.Marshal(resetEntry)
	if err != nil {
		return false, fmt.Errorf("marshaling reset entry: %w", err)
	}
	resetEntry.Hash = ComputeHash(fileHashHex, payload)

	line, err := json.Marshal(resetEntry)
	if err != nil {
		return false, fmt.Errorf("marshaling reset entry: %w", err)
	}
	line = append(line, '\n')

	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return false, fmt.Errorf("opening log file: %w", err)
	}
	defer f.Close()

	if _, err := f.Write(line); err != nil {
		return false, fmt.Errorf("writing reset marker: %w", err)
	}

	return true, nil
}

// IsChainReset returns true if the entry is a chain_reset marker.
func IsChainReset(e *Entry) bool {
	return e.Tool == "chain_reset" && e.Status == "repair"
}

// NopLogger is a no-op logger for testing or when logging is disabled.
type NopLogger struct{}

func (NopLogger) Log(*Entry) error { return nil }
func (NopLogger) Close() error     { return nil }
