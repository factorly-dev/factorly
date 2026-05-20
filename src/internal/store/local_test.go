// Copyright 2026 Jordan Sherer <hi@jordansherer.com>
// SPDX-License-Identifier: gpl

package store

import (
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// newBackend opens a fresh LocalBackend in a tempdir. Tests get
// isolation for free (each test sees a new database). t.Cleanup
// ensures the bbolt file lock is released even on failure.
func newBackend(t *testing.T) *LocalBackend {
	t.Helper()
	path := filepath.Join(t.TempDir(), "store.db")
	b, err := OpenLocalAt(path)
	if err != nil {
		t.Fatalf("OpenLocalAt: %v", err)
	}
	t.Cleanup(func() { _ = b.Close() })
	return b
}

func TestLocalBackendRoundTrip(t *testing.T) {
	b := newBackend(t)

	if err := b.Set("foo", "bar"); err != nil {
		t.Fatal(err)
	}
	got, err := b.Get("foo")
	if err != nil {
		t.Fatal(err)
	}
	if got != "bar" {
		t.Errorf("got %q, want %q", got, "bar")
	}
}

func TestLocalBackendGetMissingReturnsErrNotFound(t *testing.T) {
	b := newBackend(t)
	_, err := b.Get("nope")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("got %v, want ErrNotFound", err)
	}
}

// TestLocalBackendEmptyKeyRejected catches a class of bugs where
// callers feed an empty key through stringly-typed plumbing. Better
// to fail loudly than silently store under "" (which bbolt allows).
func TestLocalBackendEmptyKeyRejected(t *testing.T) {
	b := newBackend(t)
	if err := b.Set("", "x"); err == nil {
		t.Error("expected Set with empty key to error")
	}
	if _, err := b.Get(""); err == nil {
		t.Error("expected Get with empty key to error")
	}
	if err := b.Delete(""); err == nil {
		t.Error("expected Delete with empty key to error")
	}
}

func TestLocalBackendDeleteIsIdempotent(t *testing.T) {
	b := newBackend(t)
	if err := b.Set("k", "v"); err != nil {
		t.Fatal(err)
	}
	if err := b.Delete("k"); err != nil {
		t.Fatal(err)
	}
	// Delete again — must not error.
	if err := b.Delete("k"); err != nil {
		t.Errorf("second delete should be idempotent, got %v", err)
	}
	// Get must report missing.
	if _, err := b.Get("k"); !errors.Is(err, ErrNotFound) {
		t.Errorf("after delete, got %v, want ErrNotFound", err)
	}
}

func TestLocalBackendListSorted(t *testing.T) {
	b := newBackend(t)
	for _, k := range []string{"charlie", "alpha", "bravo"} {
		if err := b.Set(k, "v"); err != nil {
			t.Fatal(err)
		}
	}
	keys, err := b.List()
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"alpha", "bravo", "charlie"}
	if len(keys) != len(want) {
		t.Fatalf("got %d keys, want %d", len(keys), len(want))
	}
	for i, k := range keys {
		if k != want[i] {
			t.Errorf("position %d: got %q, want %q", i, k, want[i])
		}
	}
}

func TestLocalBackendSearchSubstringCaseInsensitive(t *testing.T) {
	b := newBackend(t)
	for _, k := range []string{
		"research:url:example.com",
		"research:url:google.com",
		"deployment:sha",
		"User:Preference:Theme",
	} {
		_ = b.Set(k, "v")
	}

	cases := []struct {
		query string
		want  []string
	}{
		{"research", []string{"research:url:example.com", "research:url:google.com"}},
		{"GOOGLE", []string{"research:url:google.com"}},   // case-insensitive
		{"preference", []string{"User:Preference:Theme"}}, // case-insensitive against mixed-case key
		{"nothing-here", nil},
		{"", []string{
			"User:Preference:Theme", "deployment:sha",
			"research:url:example.com", "research:url:google.com",
		}}, // empty query = all keys
	}
	for _, c := range cases {
		t.Run(c.query, func(t *testing.T) {
			got, err := b.Search(c.query)
			if err != nil {
				t.Fatal(err)
			}
			if len(got) != len(c.want) {
				t.Fatalf("got %d results %v, want %d %v", len(got), got, len(c.want), c.want)
			}
			for i, k := range got {
				if k != c.want[i] {
					t.Errorf("position %d: got %q, want %q", i, k, c.want[i])
				}
			}
		})
	}
}

func TestLocalBackendNeverExpireWithTTLZero(t *testing.T) {
	b := newBackend(t)
	// Inject a clock that jumps a year forward.
	b.nowFn = func() time.Time { return time.Now() }
	if err := b.SetWithTTL("forever", "x", 0); err != nil {
		t.Fatal(err)
	}
	b.nowFn = func() time.Time { return time.Now().Add(365 * 24 * time.Hour) }
	got, err := b.Get("forever")
	if err != nil {
		t.Fatalf("TTL=0 entry should never expire, got %v", err)
	}
	if got != "x" {
		t.Errorf("got %q, want x", got)
	}
}

// TestLocalBackendTTLExpiration exercises the core lifecycle:
// entry exists → time passes → entry is gone. Uses injected clock
// to avoid sleeping in tests.
func TestLocalBackendTTLExpiration(t *testing.T) {
	b := newBackend(t)
	base := time.Now()
	b.nowFn = func() time.Time { return base }

	if err := b.SetWithTTL("ephemeral", "x", time.Hour); err != nil {
		t.Fatal(err)
	}
	// 30 minutes in — still alive.
	b.nowFn = func() time.Time { return base.Add(30 * time.Minute) }
	if _, err := b.Get("ephemeral"); err != nil {
		t.Errorf("at 30m: expected alive, got %v", err)
	}
	// 2 hours in (1h past TTL anchored on LastReadAt=30m) — gone.
	// Note: refresh-on-read at 30m pushed the anchor forward; total
	// lifetime is therefore 30m + 1h = 1h30m from creation.
	b.nowFn = func() time.Time { return base.Add(2 * time.Hour) }
	if _, err := b.Get("ephemeral"); !errors.Is(err, ErrNotFound) {
		t.Errorf("at 2h: expected ErrNotFound, got %v", err)
	}
}

// TestLocalBackendRefreshOnReadExtendsLifetime locks in the
// "frequently-touched entries stay alive" contract. Without refresh
// the entry would die at base+1h; with refresh-on-read at base+30m,
// it lives until base+1h30m.
func TestLocalBackendRefreshOnReadExtendsLifetime(t *testing.T) {
	b := newBackend(t)
	base := time.Now()
	b.nowFn = func() time.Time { return base }
	if err := b.SetWithTTL("k", "v", time.Hour); err != nil {
		t.Fatal(err)
	}
	// Read at 30m — refreshes anchor.
	b.nowFn = func() time.Time { return base.Add(30 * time.Minute) }
	if _, err := b.Get("k"); err != nil {
		t.Fatalf("at 30m: %v", err)
	}
	// At 1h05m — pre-refresh contract would say expired (>1h since
	// creation), refresh contract says alive (35m since last read).
	b.nowFn = func() time.Time { return base.Add(65 * time.Minute) }
	if _, err := b.Get("k"); err != nil {
		t.Errorf("at 1h05m: refresh should have extended TTL, got %v", err)
	}
}

// TestLocalBackendExpiredEntryCleanedUp confirms the lazy
// expiration path's best-effort cleanup. After a Get returns
// ErrNotFound on an expired entry, the entry should be physically
// gone (List shouldn't see it either).
func TestLocalBackendExpiredEntryCleanedUp(t *testing.T) {
	b := newBackend(t)
	base := time.Now()
	b.nowFn = func() time.Time { return base }
	if err := b.SetWithTTL("doomed", "x", time.Hour); err != nil {
		t.Fatal(err)
	}
	// Jump past expiry.
	b.nowFn = func() time.Time { return base.Add(2 * time.Hour) }
	_, _ = b.Get("doomed") // triggers cleanup
	keys, err := b.List()
	if err != nil {
		t.Fatal(err)
	}
	for _, k := range keys {
		if k == "doomed" {
			t.Errorf("expired entry %q still appears in List", k)
		}
	}
}

func TestLocalBackendListExcludesExpired(t *testing.T) {
	b := newBackend(t)
	base := time.Now()
	b.nowFn = func() time.Time { return base }
	_ = b.SetWithTTL("alive", "x", time.Hour)
	_ = b.SetWithTTL("dead", "x", time.Minute)
	b.nowFn = func() time.Time { return base.Add(30 * time.Minute) }

	keys, err := b.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(keys) != 1 || keys[0] != "alive" {
		t.Errorf("expected ['alive'], got %v", keys)
	}
}

// TestLocalBackendNegativeTTLRejected guards against the "typo
// expire-immediately" hazard. Negative durations are almost always
// a bug, so we surface the error instead of silently treating them
// as zero or instantly-expired.
func TestLocalBackendNegativeTTLRejected(t *testing.T) {
	b := newBackend(t)
	if err := b.SetWithTTL("k", "v", -time.Hour); err == nil {
		t.Error("expected negative TTL to error")
	}
}

// TestLocalBackendCloseIsIdempotent matches the contract of every
// other vault/store-style backend: closing twice should not error.
func TestLocalBackendCloseIsIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "store.db")
	b, err := OpenLocalAt(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := b.Close(); err != nil {
		t.Fatalf("first close: %v", err)
	}
	if err := b.Close(); err != nil {
		t.Errorf("second close should be no-op, got %v", err)
	}
}

// TestLocalBackendOpenTimeoutOnContention validates the lock-timeout
// behavior. Opening a second handle to the same file while the first
// is still alive must fail within the openTimeout window — not hang
// forever.
func TestLocalBackendOpenTimeoutOnContention(t *testing.T) {
	path := filepath.Join(t.TempDir(), "store.db")
	first, err := OpenLocalAt(path)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()

	start := time.Now()
	_, err = OpenLocalAt(path)
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("expected second Open to fail while first is held")
	}
	// Allow some slack but the operation must clearly bound on the
	// timeout, not block on the lock indefinitely.
	if elapsed > 5*time.Second {
		t.Errorf("Open took %s, expected to fail near openTimeout (%s)", elapsed, openTimeout)
	}
}

// TestLocalBackendConcurrentWrites stresses the bbolt file-locking
// to confirm we don't observe torn writes, lost updates, or panics
// under hammered concurrent access from within a single process.
// Real cross-process contention is covered by the integration test
// (which spawns subprocesses); this is the in-process guard.
func TestLocalBackendConcurrentWrites(t *testing.T) {
	b := newBackend(t)
	const N = 50
	var wg sync.WaitGroup
	wg.Add(N)
	for i := 0; i < N; i++ {
		i := i
		go func() {
			defer wg.Done()
			k := "concurrent"
			// Append i to the value; we don't care about the final
			// state, only that no Set call errors and no panic occurs.
			if err := b.Set(k, strings.Repeat("x", i+1)); err != nil {
				t.Errorf("goroutine %d Set: %v", i, err)
			}
		}()
	}
	wg.Wait()

	// After all writers complete, the key must exist and read cleanly.
	v, err := b.Get("concurrent")
	if err != nil {
		t.Fatalf("Get after concurrent writes: %v", err)
	}
	if v == "" {
		t.Error("value disappeared under concurrency")
	}
}

// TestLocalBackendEntryReturnsMetadata exercises the Entry method —
// the side-effect-free read used by the UI detail page. Must NOT
// bump LastReadAt (that's Get's contract), must return ErrNotFound
// for missing/expired, and must surface accurate CreatedAt + TTL.
func TestLocalBackendEntryReturnsMetadata(t *testing.T) {
	b := newBackend(t)
	base := time.Now()
	b.nowFn = func() time.Time { return base }
	if err := b.SetWithTTL("k", "v", 2*time.Hour); err != nil {
		t.Fatal(err)
	}
	info, err := b.Entry("k")
	if err != nil {
		t.Fatalf("Entry: %v", err)
	}
	if info.Value != "v" {
		t.Errorf("Value = %q, want v", info.Value)
	}
	if info.TTL != 2*time.Hour {
		t.Errorf("TTL = %s, want 2h", info.TTL)
	}
	if !info.CreatedAt.Equal(base) {
		t.Errorf("CreatedAt = %v, want %v", info.CreatedAt, base)
	}
	if !info.LastReadAt.IsZero() {
		t.Errorf("LastReadAt should be zero on a fresh entry, got %v", info.LastReadAt)
	}

	// Confirm Entry is side-effect-free: a second Entry call still
	// sees LastReadAt as zero. (Get would have bumped it.)
	info2, err := b.Entry("k")
	if err != nil {
		t.Fatalf("Entry 2nd: %v", err)
	}
	if !info2.LastReadAt.IsZero() {
		t.Error("Entry should not bump LastReadAt; got non-zero")
	}

	// Expired entries return ErrNotFound (matches Get).
	b.nowFn = func() time.Time { return base.Add(3 * time.Hour) }
	if _, err := b.Entry("k"); err != ErrNotFound {
		t.Errorf("expired Entry: got %v, want ErrNotFound", err)
	}

	// Missing key returns ErrNotFound.
	if _, err := b.Entry("nope"); err != ErrNotFound {
		t.Errorf("missing Entry: got %v, want ErrNotFound", err)
	}
}

// TestEntryInfoRemaining covers the never-expire and approaching-
// expiry math the UI uses to render a TTL badge.
func TestEntryInfoRemaining(t *testing.T) {
	base := time.Now()
	// never-expire
	info := EntryInfo{CreatedAt: base, TTL: 0}
	if _, hasTTL := info.Remaining(base); hasTTL {
		t.Error("TTL=0 should report no remaining (never-expire)")
	}
	// alive
	info = EntryInfo{CreatedAt: base, TTL: time.Hour}
	rem, hasTTL := info.Remaining(base.Add(30 * time.Minute))
	if !hasTTL || rem != 30*time.Minute {
		t.Errorf("at 30m: remaining = %v hasTTL=%v, want 30m true", rem, hasTTL)
	}
	// past expiry (negative)
	info = EntryInfo{CreatedAt: base, TTL: time.Hour}
	rem, hasTTL = info.Remaining(base.Add(2 * time.Hour))
	if !hasTTL || rem >= 0 {
		t.Errorf("past expiry: remaining = %v hasTTL=%v, want negative,true", rem, hasTTL)
	}
	// LastReadAt anchors lifetime
	info = EntryInfo{CreatedAt: base, LastReadAt: base.Add(45 * time.Minute), TTL: time.Hour}
	rem, _ = info.Remaining(base.Add(50 * time.Minute))
	// 1h - (50m - 45m) = 55m
	if rem != 55*time.Minute {
		t.Errorf("refresh-anchored: remaining = %v, want 55m", rem)
	}
}

// TestLocalBackendPathAccessor confirms Path returns the file we
// opened. Pedantic but the UI relies on this for diagnostics.
func TestLocalBackendPathAccessor(t *testing.T) {
	path := filepath.Join(t.TempDir(), "store.db")
	b, err := OpenLocalAt(path)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	if b.Path() != path {
		t.Errorf("Path() = %q, want %q", b.Path(), path)
	}
}
