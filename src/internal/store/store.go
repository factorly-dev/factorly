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

import "errors"

// ErrNotFound is returned when a key does not exist in the backend.
// Aliased so the package is self-contained even though it equals
// vault.ErrNotFound's semantics.
var ErrNotFound = errors.New("store key not found")

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
