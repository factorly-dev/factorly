// Copyright 2026 Jordan Sherer <hi@jordansherer.com>
// SPDX-License-Identifier: gpl

package shadow

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// rateEntry is the token bucket state persisted to disk.
type rateEntry struct {
	Tokens     float64   `json:"tokens"`
	LastRefill time.Time `json:"last_refill"`
	Capacity   int       `json:"capacity"`
	RefillRate float64   `json:"refill_rate"` // tokens per second
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
// Returns (allowed, remainingWait, error).
// Uses a token bucket algorithm: tokens refill at limit/window rate,
// capped at limit. Each call consumes one token.
func (s *RateStore) Check(toolName string, limit int, window time.Duration) (bool, time.Duration, error) {
	entries, err := s.load()
	if err != nil {
		return false, 0, fmt.Errorf("loading rate state: %w", err)
	}

	now := time.Now()
	entry, ok := entries[toolName]

	// Calculate refill rate: limit tokens per window
	refillRate := float64(limit) / window.Seconds()

	if !ok || entry.Capacity == 0 {
		// First call or old-format entry — start with (limit-1) tokens (consumed one)
		entries[toolName] = &rateEntry{
			Tokens:     float64(limit - 1),
			LastRefill: now,
			Capacity:   limit,
			RefillRate: refillRate,
		}
		if err := s.save(entries); err != nil {
			return true, 0, err
		}
		return true, 0, nil
	}

	// Refill based on elapsed time
	elapsed := now.Sub(entry.LastRefill).Seconds()
	entry.Tokens += elapsed * refillRate
	if entry.Tokens > float64(limit) {
		entry.Tokens = float64(limit)
	}
	entry.LastRefill = now
	entry.Capacity = limit
	entry.RefillRate = refillRate

	// Try to consume one token
	if entry.Tokens < 1.0 {
		// Calculate how long until one token is available
		deficit := 1.0 - entry.Tokens
		waitSeconds := deficit / refillRate
		remaining := time.Duration(waitSeconds * float64(time.Second))
		// Save updated state (refill applied even though denied)
		_ = s.save(entries)
		return false, remaining, nil
	}

	entry.Tokens -= 1.0
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
