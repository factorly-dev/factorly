// Copyright 2026 Jordan Sherer <hi@jordansherer.com>
// SPDX-License-Identifier: gpl

package vault

// Secret holds a sensitive byte slice (typically a vault password)
// with explicit lifecycle. It exists to replace the bare []byte
// pattern that required callers to manually call zeroBytes() on every
// return path — a forgettable, error-prone discipline.
//
// Ownership: a Secret owns its underlying bytes. Pass by value; the
// underlying slice is shared (Secrets are not deep-copied on
// assignment). Use Clone() for fan-out to multiple consumers that
// each need their own zero-able copy.
//
// Lifecycle: `defer s.Zero()` is the canonical idiom. Zero() is
// idempotent and safe on the zero value (a freshly-declared Secret
// with no bytes).
//
// Comparison with bare []byte: the previous code had ~15 manual
// zeroBytes() calls split between clean `defer` patterns and
// error-path scattered ones. Secret turns the lifecycle into a
// one-line discipline at construction; the rest is type-checked.
type Secret struct {
	b []byte
}

// NewSecret wraps b as a Secret. The caller transfers ownership: do
// not retain or mutate b after the call. To take a defensive copy
// instead, use SecretFromString or call .Clone() on the result.
func NewSecret(b []byte) Secret {
	return Secret{b: b}
}

// SecretFromString copies s into a new Secret. Use for env-var or
// other string-sourced passwords where the source string is already
// resident in memory (zeroing the original via Go's string type is
// impossible — strings are immutable). Going through Secret at least
// makes the *additional* copy zeroable.
func SecretFromString(s string) Secret {
	return Secret{b: []byte(s)}
}

// Bytes returns the underlying byte slice. The caller must not retain
// the slice past the Secret's Zero() call — doing so would expose
// scrubbed bytes through the alias.
//
// Returns nil for the zero value (an uninitialized Secret) and an
// empty (but non-nil) slice for an explicitly-zero-length Secret.
func (s Secret) Bytes() []byte {
	return s.b
}

// Len returns the password length in bytes.
func (s Secret) Len() int {
	return len(s.b)
}

// Empty reports whether the Secret holds no bytes (zero value or
// zero-length input).
func (s Secret) Empty() bool {
	return len(s.b) == 0
}

// Clone returns an independent Secret holding a copy of the bytes.
// Use when the same password needs to be passed to multiple consumers
// each with their own lifecycle (e.g. a workspace password forked to
// the workspace tier *and* a fallback chain that may try the same
// password against project/global vaults).
//
// Zeroing one Clone does not affect the other.
func (s Secret) Clone() Secret {
	if s.b == nil {
		return Secret{}
	}
	dup := make([]byte, len(s.b))
	copy(dup, s.b)
	return Secret{b: dup}
}

// Zero scrubs the underlying bytes. Idempotent; safe on the zero
// value. After Zero, Bytes() returns the same slice (now zeroed) and
// Len() still reports the original length — the slice header isn't
// reset because callers may have aliased the slice and would crash
// on a sudden nil. Don't keep using a Secret after Zero; the
// invariant is "Zero is the last call on this Secret."
func (s *Secret) Zero() {
	for i := range s.b {
		s.b[i] = 0
	}
}
