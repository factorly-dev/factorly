// Copyright 2026 Jordan Sherer <hi@jordansherer.com>
// SPDX-License-Identifier: gpl

package vault

import (
	"fmt"
	"sync"
)

// FallbackBackend wraps two vault backends. Get checks the primary first,
// falls back to secondary (opened lazily on first fallback). Set/Delete/List
// operate on primary only.
//
// SecondaryOpen failures are memoized: the opener fires exactly once,
// the resulting (backend, error) pair is cached, and the error is
// surfaced from subsequent calls. Before this memoization existed, a
// failed open would silently fall through to "secret not found" —
// masking the real cause (wrong password, missing keyfile, no stdin).
//
// The struct is safe for concurrent use. sync.Once serializes the
// first-time invocation of SecondaryOpen so concurrent callers can
// never trigger duplicate password prompts. Other goroutines block
// inside openSecondary until the in-flight opener returns.
type FallbackBackend struct {
	Primary       Backend
	Secondary     Backend
	SecondaryOpen func() (Backend, error) // lazy opener for secondary vault

	openOnce     sync.Once
	secondaryErr error // set inside openOnce.Do; safe to read after the Once fires
}

// openSecondary returns the secondary backend, lazily opening it on
// first call. Both success and failure are memoized so subsequent
// callers see the same result.
//
// Returns (nil, nil) when there is no secondary configured at all
// (neither Secondary nor SecondaryOpen).
//
// Concurrency: sync.Once both serializes the opener invocation and
// establishes the happens-before edge for reads of Secondary /
// secondaryErr. All access goes through Do() — even the
// "preconfigured Secondary" fast path — so the race detector is
// satisfied regardless of whether the caller is pre-warming a
// secondary or letting the lazy opener fire.
func (f *FallbackBackend) openSecondary() (Backend, error) {
	if f.SecondaryOpen == nil && f.Secondary == nil {
		// No lazy work to do and no preconfigured secondary; safe to
		// short-circuit (these fields are caller-controlled and not
		// mutated by us). This branch is the only one allowed to skip
		// the Once because it returns no shared state.
		return nil, nil
	}
	f.openOnce.Do(func() {
		// Preconfigured secondary: nothing to open, just memoize the
		// "done" state so concurrent readers see the synchronized fields.
		if f.Secondary != nil {
			return
		}
		// SecondaryOpen != nil here. Fire it; the result establishes
		// happens-before with all subsequent post-Do reads.
		b, err := f.SecondaryOpen()
		if err != nil {
			f.secondaryErr = err
			return
		}
		f.Secondary = b
	})
	if f.secondaryErr != nil {
		return nil, f.secondaryErr
	}
	return f.Secondary, nil
}

// EnsureSecondary forces the lazy SecondaryOpen to fire and returns
// both the resulting backend and any opener error. Used by the UI to
// eagerly warm the project/global tiers at startup so the user
// doesn't see them as "locked" when in fact the CLI prompt already
// had the password to open them.
//
// Returns (nil, nil) when no secondary is configured at all.
// Returns (nil, err) when the lazy open failed; the same error is
// memoized and will resurface from the next Get/Set/Delete/List.
// Returns (backend, nil) on successful (or already-cached) open.
func (f *FallbackBackend) EnsureSecondary() (Backend, error) {
	return f.openSecondary()
}

func (f *FallbackBackend) Get(key string) (string, error) {
	if f.Primary != nil {
		val, err := f.Primary.Get(key)
		if err == nil {
			return val, nil
		}
	}
	sec, err := f.openSecondary()
	if err != nil {
		return "", fmt.Errorf("vault chain: %w", err)
	}
	if sec == nil {
		return "", ErrNotFound
	}
	return sec.Get(key)
}

func (f *FallbackBackend) Set(key, value string) error {
	if f.Primary != nil {
		return f.Primary.Set(key, value)
	}
	sec, err := f.openSecondary()
	if err != nil {
		return fmt.Errorf("vault chain: %w", err)
	}
	if sec == nil {
		return ErrNotFound
	}
	return sec.Set(key, value)
}

func (f *FallbackBackend) Delete(key string) error {
	if f.Primary != nil {
		return f.Primary.Delete(key)
	}
	sec, err := f.openSecondary()
	if err != nil {
		return fmt.Errorf("vault chain: %w", err)
	}
	if sec == nil {
		return ErrNotFound
	}
	return sec.Delete(key)
}

func (f *FallbackBackend) List() ([]string, error) {
	if f.Primary != nil {
		return f.Primary.List()
	}
	sec, err := f.openSecondary()
	if err != nil {
		return nil, fmt.Errorf("vault chain: %w", err)
	}
	if sec == nil {
		return nil, nil
	}
	return sec.List()
}

// Has returns true if the key exists in either vault. Has is
// non-error-bearing by design, so a lazy-open failure is swallowed
// here (the next Get/Set call will surface the real error).
func (f *FallbackBackend) Has(key string) bool {
	if f.Primary != nil {
		if lb, ok := f.Primary.(*LocalBackend); ok && lb.Has(key) {
			return true
		}
	}
	sec, _ := f.openSecondary()
	if sec != nil {
		if lb, ok := sec.(*LocalBackend); ok && lb.Has(key) {
			return true
		}
	}
	return false
}

func (f *FallbackBackend) Close() error {
	if f.Primary != nil {
		f.Primary.Close()
	}
	if f.Secondary != nil {
		f.Secondary.Close()
	}
	return nil
}
