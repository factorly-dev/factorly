// Copyright 2026 Jordan Sherer <hi@jordansherer.com>
// SPDX-License-Identifier: gpl

package vault

import "fmt"

// FallbackBackend wraps two vault backends. Get checks the primary first,
// falls back to secondary (opened lazily on first fallback). Set/Delete/List
// operate on primary only.
//
// SecondaryOpen failures are memoized: the opener fires once, the
// resulting (backend, error) pair is cached, and the error is surfaced
// from subsequent calls. Before this memoization existed, a failed
// open would silently fall through to "secret not found" — masking
// the real cause (wrong password, missing keyfile, no stdin).
type FallbackBackend struct {
	Primary       Backend
	Secondary     Backend
	SecondaryOpen func() (Backend, error) // lazy opener for secondary vault

	// Sticky results from the first openSecondary attempt. Once
	// secondaryTried is true, openSecondary never re-invokes the opener.
	secondaryTried bool
	secondaryErr   error
}

// openSecondary returns the secondary backend, lazily opening it on
// first call. Both success and failure are memoized so subsequent
// callers see the same result.
//
// Returns (nil, nil) when there is no secondary configured at all
// (neither Secondary nor SecondaryOpen).
func (f *FallbackBackend) openSecondary() (Backend, error) {
	if f.Secondary != nil {
		return f.Secondary, nil
	}
	if f.secondaryTried {
		return nil, f.secondaryErr
	}
	if f.SecondaryOpen == nil {
		return nil, nil
	}
	f.secondaryTried = true
	b, err := f.SecondaryOpen()
	if err != nil {
		f.secondaryErr = err
		return nil, err
	}
	f.Secondary = b
	return b, nil
}

// EnsureSecondary forces the lazy SecondaryOpen to fire and returns
// the resulting backend (or nil if it failed / was never set). Used
// by the UI to eagerly warm the project/global tiers at startup so
// the user doesn't see them as "locked" when in fact the CLI prompt
// already had the password to open them.
//
// The error (if any) is memoized on the FallbackBackend and surfaces
// on the next Get; this method discards it to preserve the existing
// signature.
func (f *FallbackBackend) EnsureSecondary() Backend {
	b, _ := f.openSecondary()
	return b
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
