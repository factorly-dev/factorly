package shadow

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sort"
	"sync"
	"time"
)

// LoopStatus describes the result of a loop detection check.
type LoopStatus int

const (
	LoopNormal LoopStatus = iota
	LoopWarning
	LoopBlocked
)

// CallFingerprint uniquely identifies a tool call by name and argument hash.
type CallFingerprint struct {
	Tool     string
	ArgsHash string
}

// LoopDetector tracks repeated identical tool calls within a sliding window.
type LoopDetector struct {
	mu      sync.Mutex
	window  time.Duration
	history map[CallFingerprint][]time.Time
}

// NewLoopDetector creates a loop detector. If window is 0, defaults to 300s.
func NewLoopDetector(window time.Duration) *LoopDetector {
	if window == 0 {
		window = 300 * time.Second
	}
	return &LoopDetector{
		window:  window,
		history: make(map[CallFingerprint][]time.Time),
	}
}

// Check records a tool call and returns the loop status based on how many
// identical calls have occurred within the detection window.
// Normal (<=3), Warning (4-8), Blocked (>=12).
func (ld *LoopDetector) Check(toolName string, params map[string]string) LoopStatus {
	fp := fingerprint(toolName, params)
	now := time.Now()

	ld.mu.Lock()
	defer ld.mu.Unlock()

	// Prune expired entries for this fingerprint
	cutoff := now.Add(-ld.window)
	times := ld.history[fp]
	pruned := times[:0]
	for _, t := range times {
		if t.After(cutoff) {
			pruned = append(pruned, t)
		}
	}

	// Record this call
	pruned = append(pruned, now)
	ld.history[fp] = pruned

	count := len(pruned)
	switch {
	case count >= 12:
		return LoopBlocked
	case count >= 4:
		return LoopWarning
	default:
		return LoopNormal
	}
}

// fingerprint computes a CallFingerprint from a tool name and params.
func fingerprint(toolName string, params map[string]string) CallFingerprint {
	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	canonical := make(map[string]string, len(params))
	for _, k := range keys {
		canonical[k] = params[k]
	}

	data, _ := json.Marshal(canonical)
	raw := fmt.Sprintf("%s:%s", toolName, data)
	hash := sha256.Sum256([]byte(raw))

	return CallFingerprint{
		Tool:     toolName,
		ArgsHash: fmt.Sprintf("%x", hash),
	}
}
