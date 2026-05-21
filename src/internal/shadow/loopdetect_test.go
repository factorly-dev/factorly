// Copyright 2026 Jordan Sherer <hi@jordansherer.com>
// SPDX-License-Identifier: gpl

package shadow

import (
	"sync"
	"testing"
	"time"
)

func TestLoopDetectNormal(t *testing.T) {
	ld := NewLoopDetector(0)
	params := map[string]string{"repo": "myrepo", "branch": "main"}

	for i := 0; i < 3; i++ {
		status := ld.Check("github.list_repos", params)
		if status != LoopNormal {
			t.Errorf("call %d: expected LoopNormal, got %d", i+1, status)
		}
	}
}

func TestLoopDetectWarning(t *testing.T) {
	ld := NewLoopDetector(0)
	params := map[string]string{"query": "select *"}

	for i := 0; i < 4; i++ {
		ld.Check("db.query", params)
	}

	status := ld.Check("db.query", params)
	if status != LoopWarning {
		t.Errorf("expected LoopWarning, got %d", status)
	}
}

func TestLoopDetectBlocked(t *testing.T) {
	ld := NewLoopDetector(0)
	params := map[string]string{"file": "/etc/passwd"}

	for i := 0; i < 11; i++ {
		ld.Check("file.read", params)
	}

	status := ld.Check("file.read", params)
	if status != LoopBlocked {
		t.Errorf("expected LoopBlocked, got %d", status)
	}
}

func TestLoopDetectDifferentArgs(t *testing.T) {
	ld := NewLoopDetector(0)

	for i := 0; i < 20; i++ {
		params := map[string]string{"id": string(rune('a' + i))}
		status := ld.Check("api.call", params)
		if status != LoopNormal {
			t.Errorf("call %d: expected LoopNormal for unique args, got %d", i+1, status)
		}
	}
}

func TestLoopDetectWindowExpiry(t *testing.T) {
	ld := NewLoopDetector(50 * time.Millisecond)
	params := map[string]string{"x": "1"}

	// Make 8 calls to get into warning range
	for i := 0; i < 8; i++ {
		ld.Check("tool", params)
	}

	// Wait for the window to expire
	time.Sleep(80 * time.Millisecond)

	// After expiry, count resets — should be normal
	status := ld.Check("tool", params)
	if status != LoopNormal {
		t.Errorf("expected LoopNormal after window expiry, got %d", status)
	}
}

func TestLoopDetectConcurrent(t *testing.T) {
	ld := NewLoopDetector(0)
	params := map[string]string{"key": "value"}

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ld.Check("concurrent.tool", params)
		}()
	}
	wg.Wait()
	// If we get here without a panic, concurrency is safe.
}
