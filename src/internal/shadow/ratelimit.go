package shadow

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// rateEntry is a single rate limit counter persisted to disk.
type rateEntry struct {
	Count       int       `json:"count"`
	WindowStart time.Time `json:"window_start"`
}

// RateStore persists rate limit state to a JSON file.
type RateStore struct {
	path string
}

// NewRateStore creates a rate store at the given path.
// If path is empty, uses ~/.config/factorly/ratelimit.json.
func NewRateStore(path string) *RateStore {
	if path == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			path = "ratelimit.json"
		} else {
			path = filepath.Join(home, ".config", "factorly", "ratelimit.json")
		}
	}
	return &RateStore{path: path}
}

// Check tests whether a tool call is within the rate limit.
// Returns (allowed, remainingWindow, error).
// If allowed, the counter is incremented and persisted.
func (s *RateStore) Check(toolName string, limit int, window time.Duration) (bool, time.Duration, error) {
	entries, err := s.load()
	if err != nil {
		entries = make(map[string]*rateEntry)
	}

	now := time.Now()
	entry, ok := entries[toolName]

	if !ok || now.Sub(entry.WindowStart) >= window {
		// New window
		entries[toolName] = &rateEntry{Count: 1, WindowStart: now}
		if err := s.save(entries); err != nil {
			return true, 0, err
		}
		return true, 0, nil
	}

	if entry.Count >= limit {
		remaining := window - now.Sub(entry.WindowStart)
		return false, remaining, nil
	}

	entry.Count++
	if err := s.save(entries); err != nil {
		return true, 0, err
	}
	return true, 0, nil
}

func (s *RateStore) load() (map[string]*rateEntry, error) {
	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return make(map[string]*rateEntry), nil
		}
		return nil, err
	}

	var entries map[string]*rateEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		return make(map[string]*rateEntry), nil
	}
	return entries, nil
}

func (s *RateStore) save(entries map[string]*rateEntry) error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	data, err := json.Marshal(entries)
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, data, 0o600)
}
