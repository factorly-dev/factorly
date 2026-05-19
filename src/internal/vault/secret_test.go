// Copyright 2026 Jordan Sherer <hi@jordansherer.com>
// SPDX-License-Identifier: gpl

package vault

import (
	"bytes"
	"testing"
)

func TestSecretZeroValueIsUsable(t *testing.T) {
	var s Secret
	if !s.Empty() {
		t.Error("zero value should be empty")
	}
	if s.Len() != 0 {
		t.Errorf("zero value Len(): got %d, want 0", s.Len())
	}
	if s.Bytes() != nil {
		t.Errorf("zero value Bytes(): got %v, want nil", s.Bytes())
	}
	s.Zero() // must not panic
}

func TestNewSecretWrapsSlice(t *testing.T) {
	pw := []byte("hunter2")
	s := NewSecret(pw)
	if !bytes.Equal(s.Bytes(), pw) {
		t.Errorf("Bytes() = %q, want %q", s.Bytes(), pw)
	}
	if s.Len() != 7 {
		t.Errorf("Len(): got %d, want 7", s.Len())
	}
}

func TestSecretFromStringCopies(t *testing.T) {
	s := SecretFromString("abc")
	if string(s.Bytes()) != "abc" {
		t.Errorf("Bytes() = %q, want abc", s.Bytes())
	}
	// Mutate the Secret's bytes; the original string can't be observed
	// to change, but we confirm the underlying slice is mutable.
	s.Bytes()[0] = 'X'
	if s.Bytes()[0] != 'X' {
		t.Error("Secret bytes must be mutable")
	}
}

func TestSecretZeroScrubsBytes(t *testing.T) {
	s := NewSecret([]byte("supersecret"))
	originalLen := s.Len()
	s.Zero()
	for i, b := range s.Bytes() {
		if b != 0 {
			t.Errorf("byte %d not zeroed: got %#x", i, b)
		}
	}
	if s.Len() != originalLen {
		t.Errorf("Len() after Zero: got %d, want %d (length preserved)", s.Len(), originalLen)
	}
}

func TestSecretZeroIsIdempotent(t *testing.T) {
	s := NewSecret([]byte("abc"))
	s.Zero()
	s.Zero() // must not panic, must not change anything
	for _, b := range s.Bytes() {
		if b != 0 {
			t.Error("repeat Zero corrupted state")
		}
	}
}

func TestSecretCloneIndependent(t *testing.T) {
	orig := NewSecret([]byte("master-pw"))
	dup := orig.Clone()

	if !bytes.Equal(orig.Bytes(), dup.Bytes()) {
		t.Errorf("Clone bytes diverge: orig=%q dup=%q", orig.Bytes(), dup.Bytes())
	}

	// Zeroing the clone must not affect the original.
	dup.Zero()
	if !bytes.Equal(orig.Bytes(), []byte("master-pw")) {
		t.Errorf("zeroing clone affected original: orig=%q", orig.Bytes())
	}

	// And vice versa.
	orig2 := NewSecret([]byte("master-pw"))
	dup2 := orig2.Clone()
	orig2.Zero()
	if !bytes.Equal(dup2.Bytes(), []byte("master-pw")) {
		t.Errorf("zeroing original affected clone: dup=%q", dup2.Bytes())
	}
}

func TestSecretCloneOfZeroValue(t *testing.T) {
	var s Secret
	dup := s.Clone()
	if !dup.Empty() {
		t.Error("Clone of zero value should be empty")
	}
	if dup.Bytes() != nil {
		t.Errorf("Clone of zero value: got %v, want nil bytes", dup.Bytes())
	}
}

func TestSecretEmpty(t *testing.T) {
	cases := []struct {
		name string
		s    Secret
		want bool
	}{
		{"zero value", Secret{}, true},
		{"empty slice", NewSecret([]byte{}), true},
		{"nil from string", SecretFromString(""), true},
		{"non-empty", NewSecret([]byte("x")), false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.s.Empty(); got != c.want {
				t.Errorf("Empty() = %v, want %v", got, c.want)
			}
		})
	}
}
