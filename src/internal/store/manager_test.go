// Copyright 2026 Jordan Sherer <hi@jordansherer.com>
// SPDX-License-Identifier: gpl

package store

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
)

// mockBackend is a minimal in-memory Backend used to drive Manager
// tests without bbolt's file lock. We can't reuse LocalBackend here
// because it requires a unique file path per instance; the Manager
// tests want to observe Close calls and identity comparisons cleanly.
type mockBackend struct {
	id     string
	closed atomic.Bool
}

func (m *mockBackend) Get(string) (string, error)      { return "", ErrNotFound }
func (m *mockBackend) Set(string, string) error        { return nil }
func (m *mockBackend) Delete(string) error             { return nil }
func (m *mockBackend) List() ([]string, error)         { return nil, nil }
func (m *mockBackend) Search(string) ([]string, error) { return nil, nil }
func (m *mockBackend) Close() error                    { m.closed.Store(true); return nil }

func TestManagerCachedReturnsNilWhenEmpty(t *testing.T) {
	m := NewManager(nil)
	if got := m.Cached("project"); got != nil {
		t.Errorf("expected nil for empty cache, got %T", got)
	}
}

func TestManagerGetOrOpenInvokesOpenerOnce(t *testing.T) {
	var calls int32
	expected := &mockBackend{id: "x"}
	m := NewManager(func(scope string) (Backend, error) {
		atomic.AddInt32(&calls, 1)
		return expected, nil
	})
	for i := 0; i < 3; i++ {
		got, err := m.GetOrOpen("project")
		if err != nil {
			t.Fatal(err)
		}
		if got != expected {
			t.Errorf("call %d: got different backend", i)
		}
	}
	if c := atomic.LoadInt32(&calls); c != 1 {
		t.Errorf("opener called %d times, want 1", c)
	}
}

func TestManagerGetOrOpenPropagatesError(t *testing.T) {
	openErr := errors.New("nope")
	m := NewManager(func(string) (Backend, error) { return nil, openErr })
	_, err := m.GetOrOpen("project")
	if !errors.Is(err, openErr) {
		t.Errorf("got %v, want %v", err, openErr)
	}
}

// TestManagerGetOrOpenDoesNotCacheFailures: a failed open must be
// retryable. Without this, a transient error (e.g. concurrent
// process holding the bbolt lock for too long) would poison the
// scope's cache forever in this process.
func TestManagerGetOrOpenDoesNotCacheFailures(t *testing.T) {
	var calls int
	m := NewManager(func(string) (Backend, error) {
		calls++
		if calls == 1 {
			return nil, errors.New("first try")
		}
		return &mockBackend{id: "y"}, nil
	})
	if _, err := m.GetOrOpen("project"); err == nil {
		t.Fatal("expected first call to error")
	}
	if _, err := m.GetOrOpen("project"); err != nil {
		t.Errorf("retry should succeed, got %v", err)
	}
	if calls != 2 {
		t.Errorf("opener calls = %d, want 2", calls)
	}
}

// TestManagerGetOrOpenClosesRaceLoser is the critical concurrency
// contract for bbolt: if two goroutines race to open the same
// scope, the loser's handle must be Closed so the bbolt file lock
// it holds is released. Without this fix the winner would be
// permanently unable to serve writes (the loser keeps the
// exclusive lock; the winner just sits in the cache map but every
// transaction blocks).
func TestManagerGetOrOpenClosesRaceLoser(t *testing.T) {
	const N = 20
	// Synchronize all goroutines to enter the opener at once. The
	// opener returns a distinct backend each time and records it so
	// we can verify the losers were closed even though callers never
	// observe them (Manager returns the cached winner instead).
	gate := make(chan struct{})
	var openedMu sync.Mutex
	var openedBackends []*mockBackend
	m := NewManager(func(string) (Backend, error) {
		<-gate
		mb := &mockBackend{id: "opened"}
		openedMu.Lock()
		openedBackends = append(openedBackends, mb)
		openedMu.Unlock()
		return mb, nil
	})

	var wg sync.WaitGroup
	results := make([]Backend, N)
	wg.Add(N)
	for i := 0; i < N; i++ {
		i := i
		go func() {
			defer wg.Done()
			b, err := m.GetOrOpen("project")
			if err != nil {
				t.Errorf("goroutine %d: %v", i, err)
				return
			}
			results[i] = b
		}()
	}
	close(gate)
	wg.Wait()

	cached := m.Cached("project")
	if cached == nil {
		t.Fatal("nothing cached after concurrent GetOrOpen")
	}
	// All callers received the same cached winner. (Under contention
	// some opened distinct loser backends but GetOrOpen swapped them
	// for the cached winner before returning.)
	for i, b := range results {
		if b != cached {
			t.Errorf("goroutine %d got a non-cached backend: %T", i, b)
		}
	}
	// Of every backend the opener actually constructed, exactly one
	// survives uncopied in the cache; the rest were closed by the
	// race-loser cleanup.
	openedMu.Lock()
	defer openedMu.Unlock()
	closedCount := 0
	for _, mb := range openedBackends {
		if mb.closed.Load() {
			closedCount++
		} else if Backend(mb) != cached {
			t.Errorf("opened backend was not the winner AND was not closed: %+v", mb)
		}
	}
	wantClosed := len(openedBackends) - 1
	if closedCount != wantClosed {
		t.Errorf("closed %d losers, want %d (opener fired %d times)", closedCount, wantClosed, len(openedBackends))
	}
}

func TestManagerPutAndCached(t *testing.T) {
	first := &mockBackend{id: "first"}
	second := &mockBackend{id: "second"}
	m := NewManager(nil)
	m.Put("project", first)
	if m.Cached("project") != first {
		t.Error("Put didn't store")
	}
	m.Put("project", second)
	if m.Cached("project") != second {
		t.Error("Put didn't replace")
	}
	// Put(nil) is a no-op.
	m.Put("project", nil)
	if m.Cached("project") != second {
		t.Error("Put(nil) should not overwrite")
	}
}

// TestManagerActiveResolution mirrors vault.Manager.Active: workspace
// wins when set, otherwise project. nil if neither cached.
func TestManagerActiveResolution(t *testing.T) {
	ws := &mockBackend{id: "ws"}
	proj := &mockBackend{id: "proj"}
	m := NewManager(nil)

	if got := m.Active("staging"); got != nil {
		t.Errorf("with empty cache: got %T, want nil", got)
	}

	m.Put("workspace:staging", ws)
	m.Put("project", proj)

	if got := m.Active("staging"); got != ws {
		t.Errorf("workspace active: got %v, want ws", got)
	}
	if got := m.Active(""); got != proj {
		t.Errorf("no workspace: got %v, want proj", got)
	}
	if got := m.Active("unknown"); got != proj {
		t.Errorf("unknown workspace: got %v, want proj fallback", got)
	}
}

func TestManagerCloseAllClosesEverything(t *testing.T) {
	a, b, c := &mockBackend{id: "a"}, &mockBackend{id: "b"}, &mockBackend{id: "c"}
	m := NewManager(nil)
	m.Put("project", a)
	m.Put("workspace:staging", b)
	m.Put("global", c)
	if err := m.CloseAll(); err != nil {
		t.Errorf("CloseAll: %v", err)
	}
	for _, mb := range []*mockBackend{a, b, c} {
		if !mb.closed.Load() {
			t.Errorf("backend %s not closed", mb.id)
		}
	}
	// After CloseAll, Cached should return nil — the entries are gone.
	if m.Cached("project") != nil {
		t.Error("cache not cleared by CloseAll")
	}
}

// TestManagerGetOrOpenNoOpenerErrors confirms the friendly error
// message when a Manager has been constructed without an opener and
// a scope it doesn't have cached is requested.
func TestManagerGetOrOpenNoOpenerErrors(t *testing.T) {
	m := NewManager(nil)
	_, err := m.GetOrOpen("project")
	if err == nil {
		t.Fatal("expected error when no opener configured")
	}
}
