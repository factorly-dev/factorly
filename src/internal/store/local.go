// Copyright 2026 Jordan Sherer <hi@jordansherer.com>
// SPDX-License-Identifier: gpl

package store

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	bolt "go.etcd.io/bbolt"
)

// DefaultTTL is the lifetime applied to an entry when the caller
// doesn't specify one. 30 days frames the store as short-term agent
// memory by default; entries the agent re-touches stay alive
// (refresh-on-read), so genuinely persistent state survives without
// the operator having to think about TTL explicitly.
const DefaultTTL = 30 * 24 * time.Hour

// openTimeout caps how long a process waits for bbolt's exclusive
// file lock before giving up. Without this, two factorly processes
// opening the same store.db serialize indefinitely. Two seconds is
// the same patience we want elsewhere — long enough to cover real
// contention, short enough that the user sees "store busy" quickly.
const openTimeout = 2 * time.Second

// bucketName is the single bbolt bucket every LocalBackend uses.
// One bucket per database file keeps the schema flat — workspace /
// project / global cascade is handled by separate files (one per
// scope), not by separate buckets within a shared file. Matches the
// vault per-tier file layout.
var bucketName = []byte("entries")

// entryRecord is the value persisted under each key. Wrapping the
// raw value in JSON lets us carry the timestamps and TTL without a
// separate metadata bucket — fewer transactions per Get, simpler
// reasoning about consistency.
type entryRecord struct {
	Value      string    `json:"v"`
	CreatedAt  time.Time `json:"ct"`
	LastReadAt time.Time `json:"rt,omitempty"`
	TTL        int64     `json:"ttl,omitempty"` // nanoseconds; 0 = never expires
}

// expired reports whether the entry should be treated as deleted at
// the given moment. Lazy expiration: we check on Get instead of
// running a background sweep. Entries with TTL=0 never expire.
//
// The "alive" time anchors on LastReadAt when set (refresh-on-read),
// otherwise CreatedAt. So an entry the agent keeps consulting stays
// alive even past its original TTL window.
func (e *entryRecord) expired(now time.Time) bool {
	if e.TTL <= 0 {
		return false
	}
	anchor := e.CreatedAt
	if !e.LastReadAt.IsZero() && e.LastReadAt.After(anchor) {
		anchor = e.LastReadAt
	}
	return now.Sub(anchor) > time.Duration(e.TTL)
}

// LocalBackend is a bbolt-backed key/value store. One file per scope
// (project / workspace / global); the bbolt file lock serializes
// writes across MCP and CLI processes.
//
// LocalBackend satisfies both store.Backend and vault.Backend
// (modulo the extra Search method), so it can plug directly into
// the Resolver for {{store:KEY}} substitution without an adapter.
type LocalBackend struct {
	path string
	db   *bolt.DB

	// defaultTTL is applied to Set calls that didn't carry an
	// explicit TTL. Captured at Open time so tests can use shorter
	// defaults without messing with package globals.
	defaultTTL time.Duration

	// nowFn is the clock the backend uses for created/read timestamps
	// and TTL checks. Defaults to time.Now; tests inject a stub.
	nowFn func() time.Time

	// closeOnce guards Close so callers can call it idempotently.
	closeOnce sync.Once
}

// OpenLocalAt opens (or creates) the store database at path. The
// directory containing path is created if necessary. The bbolt file
// is opened with a 2s lock timeout — if another process holds the
// exclusive lock for longer than that, OpenLocalAt fails with a
// clear error rather than hanging.
//
// Caller owns the returned backend and must Close() when done.
func OpenLocalAt(path string) (*LocalBackend, error) {
	if path == "" {
		return nil, fmt.Errorf("store: open path is empty")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("store: creating dir for %s: %w", path, err)
	}
	db, err := bolt.Open(path, 0o600, &bolt.Options{Timeout: openTimeout})
	if err != nil {
		return nil, fmt.Errorf("store: opening %s: %w", path, err)
	}
	// Ensure the entries bucket exists. Cheap; idempotent.
	if err := db.Update(func(tx *bolt.Tx) error {
		_, e := tx.CreateBucketIfNotExists(bucketName)
		return e
	}); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("store: initializing bucket in %s: %w", path, err)
	}
	return &LocalBackend{
		path:       path,
		db:         db,
		defaultTTL: DefaultTTL,
		nowFn:      time.Now,
	}, nil
}

// Path returns the on-disk path. Useful for diagnostics and for the
// UI's "where does this scope's data live" display.
func (b *LocalBackend) Path() string { return b.path }

// SetDefaultTTL overrides the default-TTL fallback used when Set is
// called without an explicit duration. Intended for tests; production
// callers leave the constructor default.
func (b *LocalBackend) SetDefaultTTL(d time.Duration) { b.defaultTTL = d }

// Get returns the value stored at key, after checking it isn't
// expired. ErrNotFound is returned for both missing and expired keys
// — callers don't need to distinguish (and shouldn't, since lazy
// expiration means an expired entry is semantically gone).
//
// Refresh-on-read: a successful Get updates LastReadAt so the entry's
// TTL window resets. This is the mechanism that lets the agent keep
// frequently-touched state alive indefinitely without writing back.
func (b *LocalBackend) Get(key string) (string, error) {
	if key == "" {
		return "", fmt.Errorf("store: key is empty")
	}
	now := b.nowFn()

	var rec entryRecord
	var found bool
	if err := b.db.View(func(tx *bolt.Tx) error {
		bk := tx.Bucket(bucketName)
		raw := bk.Get([]byte(key))
		if raw == nil {
			return nil
		}
		if err := json.Unmarshal(raw, &rec); err != nil {
			return fmt.Errorf("store: decoding entry %q: %w", key, err)
		}
		found = true
		return nil
	}); err != nil {
		return "", err
	}
	if !found {
		return "", ErrNotFound
	}
	if rec.expired(now) {
		// Best-effort cleanup: try to delete the expired entry so the
		// file doesn't accumulate tombstones forever. Failure is fine
		// (another process may have read-only access, or the entry may
		// be re-deleted by a sweep later). We don't surface the error
		// because the user-visible state is "key doesn't exist."
		_ = b.deleteEntry(key)
		return "", ErrNotFound
	}

	// Refresh-on-read: bump LastReadAt to extend the lifetime.
	// Best-effort; if the write fails (e.g. read-only mount) we still
	// return the value the caller asked for.
	rec.LastReadAt = now
	if encoded, err := json.Marshal(&rec); err == nil {
		_ = b.db.Update(func(tx *bolt.Tx) error {
			bk := tx.Bucket(bucketName)
			return bk.Put([]byte(key), encoded)
		})
	}
	return rec.Value, nil
}

// Set stores value under key with the default TTL (30d). For an
// explicit TTL use SetWithTTL. A TTL of 0 means "never expire."
func (b *LocalBackend) Set(key, value string) error {
	return b.SetWithTTL(key, value, b.defaultTTL)
}

// SetWithTTL is the explicit-TTL form. Pass 0 for "never expire."
// Negative TTLs are rejected as a user error rather than silently
// treated as "expire immediately."
func (b *LocalBackend) SetWithTTL(key, value string, ttl time.Duration) error {
	if key == "" {
		return fmt.Errorf("store: key is empty")
	}
	if ttl < 0 {
		return fmt.Errorf("store: ttl must be non-negative (got %s); use 0 for never-expire", ttl)
	}
	now := b.nowFn()
	rec := entryRecord{
		Value:     value,
		CreatedAt: now,
		TTL:       int64(ttl),
	}
	encoded, err := json.Marshal(&rec)
	if err != nil {
		return fmt.Errorf("store: encoding entry %q: %w", key, err)
	}
	return b.db.Update(func(tx *bolt.Tx) error {
		bk := tx.Bucket(bucketName)
		return bk.Put([]byte(key), encoded)
	})
}

// Delete removes the entry at key. Missing keys are not an error —
// idempotent delete matches the vault.Backend contract.
func (b *LocalBackend) Delete(key string) error {
	if key == "" {
		return fmt.Errorf("store: key is empty")
	}
	return b.deleteEntry(key)
}

// deleteEntry is the unguarded delete used by both Delete and the
// lazy-expiration cleanup path in Get.
func (b *LocalBackend) deleteEntry(key string) error {
	return b.db.Update(func(tx *bolt.Tx) error {
		bk := tx.Bucket(bucketName)
		return bk.Delete([]byte(key))
	})
}

// List returns every non-expired key in the store, sorted
// lexicographically. The implementation walks the entire bucket
// because bbolt has no secondary index by TTL — but the bucket is
// already ordered and the per-entry expiration check is cheap.
//
// Expired entries are filtered out (matching Get's semantics) but
// NOT deleted by List itself. Cleanup happens lazily on Get.
func (b *LocalBackend) List() ([]string, error) {
	now := b.nowFn()
	var keys []string
	err := b.db.View(func(tx *bolt.Tx) error {
		bk := tx.Bucket(bucketName)
		return bk.ForEach(func(k, v []byte) error {
			var rec entryRecord
			if err := json.Unmarshal(v, &rec); err != nil {
				// Skip corrupt entries rather than failing the whole
				// list. A future `factorly store verify` could surface
				// these; for now they're invisible to the user.
				return nil
			}
			if rec.expired(now) {
				return nil
			}
			keys = append(keys, string(k))
			return nil
		})
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(keys)
	return keys, nil
}

// Search returns every non-expired key containing query as a
// substring (case-insensitive), sorted. The intentional simplicity:
// no regex, no tokenization, no scoring. Tag-style namespacing via
// key prefixes (`research:url:<url>`) gets you most of what fancier
// search would, with none of the surface area.
func (b *LocalBackend) Search(query string) ([]string, error) {
	all, err := b.List()
	if err != nil {
		return nil, err
	}
	if query == "" {
		return all, nil
	}
	q := strings.ToLower(query)
	matches := make([]string, 0, len(all))
	for _, k := range all {
		if strings.Contains(strings.ToLower(k), q) {
			matches = append(matches, k)
		}
	}
	return matches, nil
}

// Close releases the bbolt file lock. Idempotent.
func (b *LocalBackend) Close() error {
	var err error
	b.closeOnce.Do(func() {
		if b.db != nil {
			err = b.db.Close()
		}
	})
	return err
}
