// Copyright 2026 Jordan Sherer <hi@jordansherer.com>
// SPDX-License-Identifier: gpl

package main

import (
	"bufio"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/factorly-dev/factorly/internal/vault"
)

// TestCreateVaultInteractiveWritesUnlockableVault is the happy path:
// matching passphrases create a vault file on disk that can be reopened
// with the same passphrase. This pins that Initialize() actually
// persists the empty vault (the salt + key), not just builds it in
// memory.
func TestCreateVaultInteractiveWritesUnlockableVault(t *testing.T) {
	path := filepath.Join(t.TempDir(), "vault.enc")

	if err := createVaultInteractive("project", path, seqPass(t, "hunter2", "hunter2")); err != nil {
		t.Fatalf("createVaultInteractive: %v", err)
	}

	if _, err := os.Stat(path); err != nil {
		t.Fatalf("vault file not written: %v", err)
	}

	// Reopen with the right passphrase — must succeed and round-trip a value.
	pw := vault.NewSecret([]byte("hunter2"))
	b, err := vault.OpenLocalAt(path, pw)
	if err != nil {
		t.Fatalf("reopen with correct passphrase: %v", err)
	}
	defer b.Close()
	if err := b.Set("K", "v"); err != nil {
		t.Fatalf("set on reopened vault: %v", err)
	}
	got, err := b.Get("K")
	if err != nil || got != "v" {
		t.Fatalf("get = %q, %v; want \"v\", nil", got, err)
	}
}

// TestCreateVaultInteractiveWrongPassphraseRejected confirms the
// passphrase set during init is the one that actually guards the vault.
// Because Initialize() persists a real (empty) encrypted index, opening
// with the wrong passphrase fails immediately on the outer GCM decrypt —
// no Set needed.
func TestCreateVaultInteractiveWrongPassphraseRejected(t *testing.T) {
	path := filepath.Join(t.TempDir(), "vault.enc")
	if err := createVaultInteractive("global", path, seqPass(t, "correct-horse", "correct-horse")); err != nil {
		t.Fatalf("createVaultInteractive: %v", err)
	}

	wrong := vault.NewSecret([]byte("battery-staple"))
	_, err := vault.OpenLocalAt(path, wrong)
	if err == nil {
		t.Fatal("expected open with wrong passphrase to fail, got nil error")
	}
	if !errors.Is(err, vault.ErrWrongPassword) {
		t.Errorf("err = %v, want errors.Is(..., ErrWrongPassword)", err)
	}
}

// seqPass returns a readPass func that hands back the given passphrases
// in strict sequence, one per call, failing the test if called more
// times than values supplied. Used to drive the retry loop precisely.
func seqPass(t *testing.T, values ...string) func(string) (vault.Secret, error) {
	t.Helper()
	i := 0
	return func(string) (vault.Secret, error) {
		if i >= len(values) {
			t.Fatalf("readPass called %d times, only %d values provided", i+1, len(values))
		}
		v := values[i]
		i++
		return vault.NewSecret([]byte(v)), nil
	}
}

// TestCreateVaultInteractiveRetriesThenSucceeds confirms a failure on
// the first attempts re-prompts, and a matching pair on the final
// allowed attempt still creates the vault. An empty entry consumes only
// one read (no confirm prompt), a mismatch consumes two:
//
//	attempt 1: "a","b"  → mismatch (2 reads)
//	attempt 2: ""        → empty    (1 read)
//	attempt 3: "good","good" → match (2 reads)
func TestCreateVaultInteractiveRetriesThenSucceeds(t *testing.T) {
	path := filepath.Join(t.TempDir(), "vault.enc")

	readPass := seqPass(t, "a", "b", "", "good", "good")
	if err := createVaultInteractive("project", path, readPass); err != nil {
		t.Fatalf("expected success on 3rd attempt, got: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("vault not written after successful retry: %v", err)
	}
	// And it's unlockable with the passphrase from the successful attempt.
	b, err := vault.OpenLocalAt(path, vault.NewSecret([]byte("good")))
	if err != nil {
		t.Fatalf("reopen with retried passphrase: %v", err)
	}
	b.Close()
}

// TestCreateVaultInteractiveFailsAfterMaxAttempts confirms that
// exhausting all attempts (here: 3 mismatches) returns an error and
// leaves NO vault file behind.
func TestCreateVaultInteractiveFailsAfterMaxAttempts(t *testing.T) {
	path := filepath.Join(t.TempDir(), "vault.enc")

	// 3 attempts × (prompt, confirm), all mismatching.
	readPass := seqPass(t, "a", "b", "c", "d", "e", "f")
	err := createVaultInteractive("global", path, readPass)
	if err == nil {
		t.Fatal("expected error after exhausting attempts, got nil")
	}
	if !strings.Contains(err.Error(), "after 3 attempts") {
		t.Errorf("error = %q, want it to mention the attempt count", err)
	}
	if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
		t.Errorf("vault file should not exist after failed attempts; stat err = %v", statErr)
	}
}

// TestCreateVaultInteractiveEmptyRetries confirms an empty passphrase
// counts as a failed attempt (re-prompted), not an immediate skip.
// First attempt: empty. Second attempt: valid match.
func TestCreateVaultInteractiveEmptyRetries(t *testing.T) {
	path := filepath.Join(t.TempDir(), "vault.enc")

	// Attempt 1: empty passphrase (no confirm prompt reached).
	// Attempt 2: matching non-empty pair.
	readPass := seqPass(t, "", "s3cret", "s3cret")
	if err := createVaultInteractive("project", path, readPass); err != nil {
		t.Fatalf("expected success after empty-then-valid, got: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("vault not written: %v", err)
	}
}

// TestMaybeInitVaultSkipsExisting confirms the existence guard: when a
// vault file already exists, maybeInitVault leaves it byte-for-byte
// untouched and never prompts (so it's safe to run on a project that
// already has a vault). We assert by content equality since the
// function prints a notice but must not rewrite the file.
func TestMaybeInitVaultSkipsExisting(t *testing.T) {
	path := filepath.Join(t.TempDir(), "vault.enc")
	const sentinel = "pre-existing vault bytes — must not be overwritten"
	if err := os.WriteFile(path, []byte(sentinel), 0o600); err != nil {
		t.Fatalf("seed vault file: %v", err)
	}

	// Empty stdin scanner: if the guard failed and we fell through to a
	// prompt, the scan would return "" and we'd take the default "y" —
	// so reaching the prompt at all risks mutating the file. The
	// existence check returns first, so no prompt happens.
	scanner := bufio.NewScanner(strings.NewReader(""))
	if err := maybeInitVault(scanner, "project", path); err != nil {
		t.Fatalf("maybeInitVault on existing vault should not error: %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read back vault file: %v", err)
	}
	if string(got) != sentinel {
		t.Errorf("existing vault was modified:\n got = %q\nwant = %q", got, sentinel)
	}
}
