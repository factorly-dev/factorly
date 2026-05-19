// Copyright 2026 Jordan Sherer <hi@jordansherer.com>
// SPDX-License-Identifier: gpl

package vault

import (
	"fmt"
	"sync"
)

// Manager owns the per-scope Backend cache for the running process.
// Both the CLI (cmd/factorly) and the UI (internal/ui) consult one
// shared Manager so they observe the same opened-tier state — without
// a Manager, the CLI's process-wide cache and the UI's Server-local
// cache could diverge (e.g. the user unlocks the project vault on the
// CLI prompt, then the UI insists the project vault is still locked).
//
// Scope keys identify which tier a backend opens:
//
//   - "" — the startup chain (what getCachedVault used to return)
//   - "project" — the project vault alone
//   - "global" — the global vault alone
//   - "workspace:<name>" — a specific workspace's chain (workspace → project → global)
//
// The Manager doesn't know how to *open* a backend — chain composition
// lives in cmd/factorly where the vaultTier abstraction sits. Instead
// it takes two opener functions at construction:
//
//   - chainOpener: how to open scope X non-interactively from the CLI's
//     password-resolution chain (env vars → keyfile → prompt).
//   - passwordOpener: how to open scope X with an explicit password
//     supplied through the UI's unlock dialog.
//
// Both openers receive the scope string so the Manager doesn't have to
// dispatch on scope kinds itself.
type Manager struct {
	mu             sync.Mutex
	cache          map[string]Backend
	chainOpener    func(scope string) (Backend, error)
	passwordOpener func(scope string, password Secret) (Backend, error)
}

// NewManager constructs a Manager with the supplied openers. Either
// may be nil — passing nil means "this Manager doesn't support that
// operation" and GetOrOpen/OpenWithPassword will return a "not
// configured" error. In practice both are set in the CLI bootstrap.
func NewManager(
	chainOpener func(scope string) (Backend, error),
	passwordOpener func(scope string, password Secret) (Backend, error),
) *Manager {
	return &Manager{
		cache:          make(map[string]Backend),
		chainOpener:    chainOpener,
		passwordOpener: passwordOpener,
	}
}

// Cached returns the cached backend for scope, or nil when nothing has
// been opened yet. Safe to call concurrently. Useful for "is this
// already unlocked?" checks where the UI doesn't want to trigger an
// open just to find out.
func (m *Manager) Cached(scope string) Backend {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.cache[scope]
}

// GetOrOpen returns the cached backend for scope, or opens it via the
// injected chainOpener and caches the result.
//
// Concurrency: under the lock, we check the cache; if missed, we
// release the lock before calling the opener (which may block on a
// stdin password prompt), then re-acquire to store. If another
// goroutine stored first, that backend wins and the second open is
// discarded. Worst case: two concurrent first-time callers each
// trigger one opener invocation. This matches the FallbackBackend
// concurrency contract.
func (m *Manager) GetOrOpen(scope string) (Backend, error) {
	m.mu.Lock()
	if b, ok := m.cache[scope]; ok {
		m.mu.Unlock()
		return b, nil
	}
	opener := m.chainOpener
	m.mu.Unlock()

	if opener == nil {
		return nil, fmt.Errorf("vault manager: no chain opener configured for scope %q", scope)
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
		// Another goroutine raced us; theirs wins. We don't close the
		// loser because the opener closure may have shared underlying
		// resources (e.g. a FallbackBackend nested under a different
		// outer chain). Leaving the loser to GC is the safe default.
		return existing, nil
	}
	m.cache[scope] = b
	return b, nil
}

// OpenWithPassword opens scope using an explicit password supplied
// through the UI unlock dialog (bypassing the env/keyfile/prompt
// resolution chain). Does NOT cache automatically — the caller decides
// whether to Put() the result, since a failed-but-non-erroring open
// shouldn't be remembered.
//
// The caller owns password and is responsible for zeroing it. The
// opener may pass it to vault.OpenLocalAt which also doesn't zero;
// `defer password.Zero()` at the call site is the canonical pattern.
func (m *Manager) OpenWithPassword(scope string, password Secret) (Backend, error) {
	m.mu.Lock()
	opener := m.passwordOpener
	m.mu.Unlock()

	if opener == nil {
		return nil, fmt.Errorf("vault manager: no password opener configured for scope %q", scope)
	}
	return opener(scope, password)
}

// Put stashes a backend under scope, replacing any prior entry. Used
// after a successful UI unlock so subsequent reads find the unlocked
// backend without re-prompting.
func (m *Manager) Put(scope string, b Backend) {
	if b == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cache[scope] = b
}

// Active returns the backend the UI should consult for "current"
// operations: workspace's chain when a workspace is selected, the
// startup chain otherwise. Falls through to whatever was Put under
// the empty scope key (the startup chain) when no workspace match.
func (m *Manager) Active(workspaceName string) Backend {
	m.mu.Lock()
	defer m.mu.Unlock()
	if workspaceName != "" {
		if b, ok := m.cache["workspace:"+workspaceName]; ok && b != nil {
			return b
		}
	}
	return m.cache[""]
}
