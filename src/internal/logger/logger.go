package logger

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type Entry struct {
	Timestamp  time.Time         `json:"timestamp"`
	Interface  string            `json:"interface"`
	Tool       string            `json:"tool"`
	Params     map[string]string `json:"params"`
	Status     string            `json:"status"`
	DurationMs int64             `json:"duration_ms"`
	Output     string            `json:"output,omitempty"`
	Error      string            `json:"error,omitempty"`
}

type Logger interface {
	Log(entry *Entry) error
	Close() error
}

type JSONLLogger struct {
	f  *os.File
	mu sync.Mutex
}

func NewJSONL(path string) (*JSONLLogger, error) {
	if path == "" {
		path = DefaultLogPath()
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}

	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, err
	}

	return &JSONLLogger{f: f}, nil
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

	data, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	data = append(data, '\n')

	_, err = l.f.Write(data)
	return err
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

// NopLogger is a no-op logger for testing or when logging is disabled.
type NopLogger struct{}

func (NopLogger) Log(*Entry) error { return nil }
func (NopLogger) Close() error     { return nil }
