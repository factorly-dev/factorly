// Copyright 2026 Jordan Sherer <hi@jordansherer.com>
// SPDX-License-Identifier: gpl

package ui

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/factorly-dev/factorly/internal/store"
)

// seedStore opens a fresh LocalBackend at the given path and stuffs
// it with the given key/value pairs. Used to verify the /store page
// reflects what's on disk.
func seedStore(t *testing.T, path string, entries map[string]string) *store.LocalBackend {
	t.Helper()
	b, err := store.OpenLocalAt(path)
	if err != nil {
		t.Fatalf("seedStore open: %v", err)
	}
	for k, v := range entries {
		if err := b.Set(k, v); err != nil {
			t.Fatalf("seedStore set %q: %v", k, err)
		}
	}
	t.Cleanup(func() { _ = b.Close() })
	return b
}

// TestStorePageRendersEmptyWhenNoStores confirms the /store page
// doesn't crash on a fresh project that's never written anything.
// The vault page has the equivalent "no sections, show CLI hint"
// branch; store always shows global so we get at least one section.
func TestStorePageRendersEmptyWhenNoStores(t *testing.T) {
	srv, _ := testServerWithProxy(t, nil)
	req := httptest.NewRequest(http.MethodGet, "/store", nil)
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{"Store", "Global store"} {
		if !strings.Contains(body, want) {
			t.Errorf("page missing %q", want)
		}
	}
}

// TestStorePageRendersGlobalKeys exercises the "global store has
// data" path. Pre-seeded via the Manager.Put hook so the test
// doesn't depend on HOME or .factorly/ directory state.
func TestStorePageRendersGlobalKeys(t *testing.T) {
	srv, _ := testServerWithProxy(t, nil)
	path := t.TempDir() + "/store.db"
	seed := seedStore(t, path, map[string]string{
		"deployment:sha": "abc123",
		"research:url:a": "hello",
	})
	srv.storeMgr.Put("global", seed)

	req := httptest.NewRequest(http.MethodGet, "/store", nil)
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	body := rec.Body.String()
	for _, key := range []string{"deployment:sha", "research:url:a"} {
		if !strings.Contains(body, key) {
			t.Errorf("page missing key %q", key)
		}
	}
}

// TestStoreSetWritesThroughManager pins the POST /store handler
// to its happy path. Uses a pre-seeded manager so the handler
// doesn't try to open .factorly/store.db on disk.
func TestStoreSetWritesThroughManager(t *testing.T) {
	srv, _ := testServerWithProxy(t, nil)
	path := t.TempDir() + "/store.db"
	seed := seedStore(t, path, nil)
	srv.storeMgr.Put("global", seed)

	form := url.Values{}
	form.Set("key", "my-key")
	form.Set("value", "my-value")
	form.Set("scope", "global")
	req := httptest.NewRequest(http.MethodPost, "/store", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("POST status = %d, body = %s", rec.Code, rec.Body.String())
	}

	got, err := seed.Get("my-key")
	if err != nil {
		t.Fatalf("backend Get: %v", err)
	}
	if got != "my-value" {
		t.Errorf("got %q, want my-value", got)
	}
}

// TestStoreSetRejectsBadScope guards against arbitrary scope
// strings sneaking writes into unexpected places (e.g. someone
// crafting POST /store with scope=workspace:../escape).
func TestStoreSetRejectsBadScope(t *testing.T) {
	srv, _ := testServerWithProxy(t, nil)
	form := url.Values{}
	form.Set("key", "k")
	form.Set("value", "v")
	form.Set("scope", "workspace:../escape")
	req := httptest.NewRequest(http.MethodPost, "/store", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d body=%s", rec.Code, rec.Body.String())
	}
}

// TestStoreDeleteRemovesKey covers the delete handler. Pre-seeds a
// key, hits DELETE, confirms the key is gone via the backend.
func TestStoreDeleteRemovesKey(t *testing.T) {
	srv, _ := testServerWithProxy(t, nil)
	path := t.TempDir() + "/store.db"
	seed := seedStore(t, path, map[string]string{"doomed": "x"})
	srv.storeMgr.Put("global", seed)

	req := httptest.NewRequest(http.MethodDelete, "/store/doomed?scope=global", nil)
	req.SetPathValue("key", "doomed")
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("DELETE status = %d, body = %s", rec.Code, rec.Body.String())
	}

	if _, err := seed.Get("doomed"); err == nil {
		t.Error("key still present after delete")
	}
}
