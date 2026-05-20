// Copyright 2026 Jordan Sherer <hi@jordansherer.com>
// SPDX-License-Identifier: gpl

package store

import (
	"fmt"
	"sync"
)

// Manager owns the per-scope Backend cache for the running process,
// mirroring vault.Manager's shape minus the password concept.
//
// Store has no encryption, no per-scope locked state, no
// password-resolution chain — so OpenWithPassword and LockedErr
// machinery don't apply. The cache + opener pattern is otherwise
// identical, which keeps the mental model consistent: both CLI and
// UI consult the same Manager, both see the same opened state.
//
// Scope keys identify which tier a backend covers:
//
//   - "project"               — .factorly/store.db
//   - "workspace:<name>"      — .factorly/workspaces/<name>/store.db
//   - "global"                — ~/.config/factorly/store.db
type Manager struct {
	mu          sync.Mutex
	cache       map[string]Backend
	chainOpener func(scope string) (Backend, error)
}

// NewManager constructs a Manager with the supplied opener. The
// opener may be nil (e.g. in tests that only exercise Put/Cached);
// GetOrOpen will then surface a clear error rather than blowing up.
func NewManager(chainOpener func(scope string) (Backend, error)) *Manager {
	return &Manager{
		cache:       make(map[string]Backend),
		chainOpener: chainOpener,
	}
}

// Cached returns the backend already opened for scope, or nil when
// nothing has been opened yet. Safe to call concurrently. Useful
// for "is this scope already open?" checks where the UI doesn't
// want to trigger a real open just to find out.
func (m *Manager) Cached(scope string) Backend {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.cache[scope]
}

// GetOrOpen returns the cached backend for scope, or opens it via
// the injected chainOpener and caches the result.
//
// Concurrency: under the lock we check the cache; on miss we
// release the lock before calling the opener (which can do I/O),
// then re-acquire to store. If another goroutine stored first, that
// backend wins and the second open is discarded. The two-open
// outcome under heavy contention is benign for bbolt — opening
// twice produces two handles, but bbolt's file lock serializes
// them. Loser handle is closed below.
func (m *Manager) GetOrOpen(scope string) (Backend, error) {
	m.mu.Lock()
	if b, ok := m.cache[scope]; ok {
		m.mu.Unlock()
		return b, nil
	}
	opener := m.chainOpener
	m.mu.Unlock()

	if opener == nil {
		return nil, fmt.Errorf("store manager: no chain opener configured for scope %q", scope)
	}
	b, err := opener(scope)
	if err != nil {
		return nil, err
	}
	if b == nil {
		return nil, nil
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if existing, ok := m.cache[scope]; ok {
		// Lost the race; close our loser handle so the bbolt lock is
		// released for the cached winner to actually serve requests.
		// Without this we'd leak file descriptors and serialize all
		// reads on the loser's still-held lock.
		_ = b.Close()
		return existing, nil
	}
	m.cache[scope] = b
	return b, nil
}

// Put stashes a backend under scope, replacing any prior entry.
// Used after a direct open (e.g. UI startup pre-warms the project
// scope) so subsequent GetOrOpen finds it.
//
// The previous entry for scope, if any, is NOT closed — callers
// who Put expect to own the lifecycle of any earlier handle. (The
// common case is Put-once at startup; replacing is rare.)
func (m *Manager) Put(scope string, b Backend) {
	if b == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cache[scope] = b
}

// Active returns the backend the UI should consult for "current"
// operations: the workspace's backend when a workspace is selected,
// falling back to the project scope. Returns nil if neither has
// been opened.
//
// Mirrors vault.Manager.Active so UI code that does "show me the
// effective store for this session" reads identically to the vault
// case.
func (m *Manager) Active(workspaceName string) Backend {
	m.mu.Lock()
	defer m.mu.Unlock()
	if workspaceName != "" {
		if b, ok := m.cache["workspace:"+workspaceName]; ok && b != nil {
			return b
		}
	}
	return m.cache["project"]
}

// CloseAll closes every cached backend. Used at process shutdown so
// bbolt's file locks are released cleanly. Errors are coalesced;
// the first non-nil is returned so the operator sees at least one
// concrete failure if something went wrong.
func (m *Manager) CloseAll() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	var firstErr error
	for scope, b := range m.cache {
		if err := b.Close(); err != nil && firstErr == nil {
			firstErr = fmt.Errorf("closing %q: %w", scope, err)
		}
		delete(m.cache, scope)
	}
	return firstErr
}
