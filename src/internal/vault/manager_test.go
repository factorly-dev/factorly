// Copyright 2026 Jordan Sherer <hi@jordansherer.com>
// SPDX-License-Identifier: gpl

package vault

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestManagerCachedReturnsNilWhenEmpty(t *testing.T) {
	m := NewManager(nil, nil)
	if got := m.Cached("project"); got != nil {
		t.Errorf("expected nil for empty cache, got %T", got)
	}
}

func TestManagerGetOrOpenCallsOpenerOnce(t *testing.T) {
	calls := 0
	expected := newFBMock(map[string]string{"K": "v"})
	m := NewManager(func(scope string) (Backend, error) {
		if scope != "project" {
			t.Errorf("opener got unexpected scope %q", scope)
		}
		calls++
		return expected, nil
	}, nil)

	for i := 0; i < 3; i++ {
		got, err := m.GetOrOpen("project")
		if err != nil {
			t.Fatalf("call %d: %v", i, err)
		}
		if got != expected {
			t.Errorf("call %d: got %T, want the cached backend", i, got)
		}
	}
	if calls != 1 {
		t.Errorf("opener called %d times; want 1", calls)
	}
}

func TestManagerGetOrOpenPropagatesError(t *testing.T) {
	openErr := errors.New("nope")
	m := NewManager(func(scope string) (Backend, error) {
		return nil, openErr
	}, nil)

	_, err := m.GetOrOpen("project")
	if !errors.Is(err, openErr) {
		t.Errorf("got %v, want %v", err, openErr)
	}
}

func TestManagerGetOrOpenDoesNotCacheErrors(t *testing.T) {
	// Failed opens should be retryable. The Manager only caches
	// successful backends. (Contrast with FallbackBackend which
	// memoizes its lazy SecondaryOpen failure on purpose — there the
	// opener can only fire once per FallbackBackend instance.)
	calls := 0
	m := NewManager(func(scope string) (Backend, error) {
		calls++
		if calls == 1 {
			return nil, errors.New("first try fails")
		}
		return newFBMock(map[string]string{"K": "v"}), nil
	}, nil)

	if _, err := m.GetOrOpen("project"); err == nil {
		t.Fatal("expected first call to error")
	}
	if _, err := m.GetOrOpen("project"); err != nil {
		t.Errorf("expected retry to succeed, got %v", err)
	}
	if calls != 2 {
		t.Errorf("opener called %d times; want 2 (retry-after-failure)", calls)
	}
}

func TestManagerGetOrOpenNoOpenerErrors(t *testing.T) {
	m := NewManager(nil, nil)
	_, err := m.GetOrOpen("project")
	if err == nil {
		t.Error("expected error when no chainOpener configured")
	}
}

func TestManagerPutReplacesCache(t *testing.T) {
	first := newFBMock(map[string]string{"K1": "v1"})
	second := newFBMock(map[string]string{"K2": "v2"})
	m := NewManager(nil, nil)

	m.Put("project", first)
	if got := m.Cached("project"); got != first {
		t.Error("Put didn't store the backend")
	}
	m.Put("project", second)
	if got := m.Cached("project"); got != second {
		t.Error("Put didn't replace the backend")
	}
}

func TestManagerPutIgnoresNil(t *testing.T) {
	first := newFBMock(map[string]string{"K": "v"})
	m := NewManager(nil, nil)
	m.Put("project", first)
	m.Put("project", nil)
	if got := m.Cached("project"); got != first {
		t.Error("Put(nil) should not overwrite a cached backend")
	}
}

func TestManagerOpenWithPasswordDelegates(t *testing.T) {
	got := struct {
		scope    string
		password string
	}{}
	expected := newFBMock(map[string]string{"K": "v"})
	m := NewManager(nil, func(scope string, pw []byte) (Backend, error) {
		got.scope = scope
		got.password = string(pw)
		return expected, nil
	})

	b, err := m.OpenWithPassword("workspace:staging", []byte("secret"))
	if err != nil {
		t.Fatal(err)
	}
	if b != expected {
		t.Error("OpenWithPassword returned the wrong backend")
	}
	if got.scope != "workspace:staging" {
		t.Errorf("opener got scope %q", got.scope)
	}
	if got.password != "secret" {
		t.Errorf("opener got password %q", got.password)
	}
}

func TestManagerOpenWithPasswordDoesNotCache(t *testing.T) {
	expected := newFBMock(map[string]string{"K": "v"})
	m := NewManager(nil, func(scope string, pw []byte) (Backend, error) {
		return expected, nil
	})
	if _, err := m.OpenWithPassword("project", []byte("pw")); err != nil {
		t.Fatal(err)
	}
	if got := m.Cached("project"); got != nil {
		t.Error("OpenWithPassword should not auto-cache; caller decides via Put")
	}
}

func TestManagerActiveResolvesWorkspaceOrFallsBack(t *testing.T) {
	ws := newFBMock(map[string]string{"K": "ws"})
	startup := newFBMock(map[string]string{"K": "startup"})
	m := NewManager(nil, nil)
	m.Put("workspace:staging", ws)
	m.Put("", startup)

	if got := m.Active("staging"); got != ws {
		t.Errorf("workspace active: got %v, want ws backend", got)
	}
	if got := m.Active("unknown"); got != startup {
		t.Errorf("unknown workspace: got %v, want startup fallback", got)
	}
	if got := m.Active(""); got != startup {
		t.Errorf("no workspace: got %v, want startup fallback", got)
	}
}

// TestManagerConcurrentGetOrOpen verifies the concurrency contract:
// the opener may fire more than once under heavy contention but
// callers always see the *same* cached backend afterwards. (Strict
// once-only firing would require an explicit per-scope mutex; the
// race-tolerant design fits how the opener is used — re-running it
// is cheap, deduplicating the *result* is what matters.)
func TestManagerConcurrentGetOrOpen(t *testing.T) {
	const N = 50
	start := make(chan struct{})
	var calls int64
	m := NewManager(func(scope string) (Backend, error) {
		<-start
		atomic.AddInt64(&calls, 1)
		return newFBMock(map[string]string{"K": "v"}), nil
	}, nil)

	var wg sync.WaitGroup
	results := make([]Backend, N)
	errs := make([]error, N)
	wg.Add(N)
	for i := 0; i < N; i++ {
		i := i
		go func() {
			defer wg.Done()
			results[i], errs[i] = m.GetOrOpen("project")
		}()
	}
	time.Sleep(20 * time.Millisecond)
	close(start)
	wg.Wait()

	// All callers must see the same cached backend.
	first := results[0]
	for i, b := range results {
		if errs[i] != nil {
			t.Errorf("goroutine %d errored: %v", i, errs[i])
		}
		if b != first {
			t.Errorf("goroutine %d got a different backend (%T vs %T)", i, b, first)
		}
	}
	if atomic.LoadInt64(&calls) < 1 {
		t.Errorf("opener never fired; got %d calls", calls)
	}
	// Sanity: the cache settled on one entry.
	if m.Cached("project") != first {
		t.Error("cached backend doesn't match what callers received")
	}
}
