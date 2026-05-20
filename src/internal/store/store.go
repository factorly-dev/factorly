// Copyright 2026 Jordan Sherer <hi@jordansherer.com>
// SPDX-License-Identifier: gpl

// Package store is the agent-writable workspace state primitive.
//
// Store sits alongside vault, env, and auth as the fourth flavor of
// workspace data. Where vault holds user-managed secrets and env holds
// user-managed config, store holds *agent*-managed scratchpad —
// cross-run state the agent maintains itself ("I researched these
// URLs already", "the Trello board ID for X is Y", "last successful
// deployment SHA").
//
// Design constraints (deliberately boring):
//   - Key/value strings only. No tags, no namespaces, no hierarchy.
//     Use key prefixes (`research:url:<url>`) for implicit namespacing.
//   - Per-workspace, cascades from project to global.
//   - Default TTL: 30 days, refresh-on-read. Entries the agent
//     re-touches stay alive; truly persistent ones use TTL=0.
//   - bbolt under the hood: pure Go, single-writer/many-readers,
//     ACID, file-locked.
//
// What store explicitly does NOT do:
//   - Vector embeddings, semantic search (use a separate tool)
//   - Rich documents (JSON-encode if you need structure)
//   - Memory framework concepts (episodic/semantic/consolidation)
//   - Cross-workspace sharing (workspaces are isolated)
//
// Store satisfies vault.Backend for the {{store:KEY}} resolver
// registration; callers that want store-specific operations
// (Search, History) use the wider store.Backend interface.
package store

import (
	"errors"
	"time"
)

// ErrNotFound is returned when a key does not exist in the backend.
// Aliased so the package is self-contained even though it equals
// vault.ErrNotFound's semantics.
var ErrNotFound = errors.New("store key not found")

// EntryInfo is the metadata view of a stored entry. Returned by
// Entry(key) on backends that support it (LocalBackend). Used by the
// UI's detail page to show value + TTL remaining + last-read without
// the side effect that Get has (refresh-on-read).
type EntryInfo struct {
	Value      string
	CreatedAt  time.Time
	LastReadAt time.Time     // zero when never read
	TTL        time.Duration // 0 = never expires
}

// Expired reports whether the entry should be treated as deleted at
// the given moment. Lifetime anchors on LastReadAt when set
// (refresh-on-read), otherwise CreatedAt. Mirrors the lazy-expiration
// check Get performs internally.
func (e EntryInfo) Expired(now time.Time) bool {
	if e.TTL == 0 {
		return false
	}
	anchor := e.CreatedAt
	if !e.LastReadAt.IsZero() && e.LastReadAt.After(anchor) {
		anchor = e.LastReadAt
	}
	return now.Sub(anchor) >= e.TTL
}

// Remaining reports how long until the entry expires. Returns
// (0, false) for never-expire entries; (negative duration, true)
// for already-expired entries (callers can decide how to render
// "expired N ago" vs filtering them out).
func (e EntryInfo) Remaining(now time.Time) (time.Duration, bool) {
	if e.TTL == 0 {
		return 0, false
	}
	anchor := e.CreatedAt
	if !e.LastReadAt.IsZero() && e.LastReadAt.After(anchor) {
		anchor = e.LastReadAt
	}
	return e.TTL - now.Sub(anchor), true
}

// Backend is the contract every store implementation satisfies. The
// first five methods overlap vault.Backend so a store backend can be
// registered as a Resolver backend for {{store:KEY}} substitution
// without any adapter. The extra Search method is store-specific.
type Backend interface {
	Get(key string) (string, error)
	Set(key, value string) error
	Delete(key string) error
	List() ([]string, error)
	Search(query string) ([]string, error)
	Close() error
}
