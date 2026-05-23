// Copyright 2026 Jordan Sherer <hi@jordansherer.com>
// SPDX-License-Identifier: gpl

package ui

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/factorly-dev/factorly/internal/config"
	"github.com/factorly-dev/factorly/internal/vault"
)

// TestUnlockErrorMessage pins the classifier: ErrWrongPassword gets
// the friendly retry nudge; nil → ""; everything else surfaces as
// "Failed to open vault: <err>". A regression here would make the
// UI lose the user-facing distinction between "you mistyped" and
// "your vault file is broken."
func TestUnlockErrorMessage(t *testing.T) {
	cases := []struct {
		name string
		in   error
		want string
	}{
		{"nil → empty", nil, ""},
		{"wrong password", vault.ErrWrongPassword, "Incorrect password — try again."},
		{"wrapped wrong password",
			fmt.Errorf("decrypting vault: %w (underlying: bad MAC)", vault.ErrWrongPassword),
			"Incorrect password — try again."},
		{"other error surfaces as-is",
			errors.New("permission denied"),
			"Failed to open vault: permission denied"},
		{"no-opener config error",
			errors.New("vault manager: no password opener configured"),
			"Failed to open vault: vault manager: no password opener configured"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := unlockErrorMessage(tc.in); got != tc.want {
				t.Errorf("unlockErrorMessage(%v) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// TestHandleVaultUnlockWrongPasswordShowsRetry exercises the full
// /vault/unlock handler against a real LocalBackend so the wiring
// from OpenWithPassword → ErrWrongPassword → unlockErrorMessage →
// rendered form is end-to-end covered.
func TestHandleVaultUnlockWrongPasswordShowsRetry(t *testing.T) {
	dir := t.TempDir()
	vaultPath := filepath.Join(dir, "vault.enc")

	// Seed a real vault with a known password.
	b, err := vault.OpenLocalAt(vaultPath, vault.NewSecret([]byte("correctpass")))
	if err != nil {
		t.Fatalf("seed vault: %v", err)
	}
	_ = b.Set("KEY", "value")
	b.Close()

	// Manager with a passwordOpener that delegates to the real
	// vault. Anything other than "correctpass" will fail with
	// ErrWrongPassword via the wrapping we added in local.go.
	mgr := vault.NewManager(
		nil,
		func(scope string, pw vault.Secret) (vault.Backend, error) {
			return vault.OpenLocalAt(vaultPath, pw)
		},
	)

	cfgPath := filepath.Join(dir, "factorly.yaml")
	_ = os.WriteFile(cfgPath, []byte("tools: {}\n"), 0o644)
	srv, err := New(Options{
		Config:       &config.Config{Tools: make(map[string]config.ToolConfig)},
		CfgPath:      cfgPath,
		VaultManager: mgr,
	})
	if err != nil {
		t.Fatal(err)
	}

	form := url.Values{"scope": {"project"}, "password": {"DEFINITELY-WRONG"}}
	req := httptest.NewRequest(http.MethodPost, "/vault/unlock", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 (re-rendered unlock form), got %d: %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Incorrect password") {
		t.Errorf("expected 'Incorrect password' retry nudge, got: %s", body)
	}
	// Generic fallback copy must NOT appear — that's reserved for
	// non-ErrWrongPassword failures.
	if strings.Contains(body, "Failed to open vault") {
		t.Error("wrong-password case should not surface as 'Failed to open vault'")
	}
}

// TestHandleVaultUnlockCorrectPasswordSucceeds pins the happy path
// from the user's POV: a correct password no longer re-renders the
// unlock form. We don't try to assert the rendered keys list here —
// the post-unlock view is built from a separate vaultSections code
// path that needs more wiring than this test sets up. The important
// guarantee is that 'Incorrect password' does NOT appear when the
// password was right.
func TestHandleVaultUnlockCorrectPasswordSucceeds(t *testing.T) {
	dir := t.TempDir()
	vaultPath := filepath.Join(dir, "vault.enc")

	b, err := vault.OpenLocalAt(vaultPath, vault.NewSecret([]byte("rightpass")))
	if err != nil {
		t.Fatalf("seed vault: %v", err)
	}
	_ = b.Set("MY_KEY", "v")
	b.Close()

	mgr := vault.NewManager(
		nil,
		func(scope string, pw vault.Secret) (vault.Backend, error) {
			return vault.OpenLocalAt(vaultPath, pw)
		},
	)
	cfgPath := filepath.Join(dir, "factorly.yaml")
	_ = os.WriteFile(cfgPath, []byte("tools: {}\n"), 0o644)
	srv, err := New(Options{
		Config:       &config.Config{Tools: make(map[string]config.ToolConfig)},
		CfgPath:      cfgPath,
		VaultManager: mgr,
	})
	if err != nil {
		t.Fatal(err)
	}

	form := url.Values{"scope": {"project"}, "password": {"rightpass"}}
	req := httptest.NewRequest(http.MethodPost, "/vault/unlock", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if strings.Contains(body, "Incorrect password") {
		t.Errorf("did not expect 'Incorrect password' on successful unlock; body=%s", body)
	}
	if strings.Contains(body, "Failed to open vault") {
		t.Errorf("did not expect 'Failed to open vault' on successful unlock; body=%s", body)
	}
}
