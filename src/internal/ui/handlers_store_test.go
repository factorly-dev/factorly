// Copyright 2026 Jordan Sherer <hi@jordansherer.com>
// SPDX-License-Identifier: gpl

package ui

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/factorly-dev/factorly/internal/store"
)

// seedStoreFile creates a bbolt store at path and writes the given
// entries. Returns the path so the test can later open and inspect.
// The file is closed before returning — handlers open it themselves
// via the opener seam, matching production lifecycle.
func seedStoreFile(t *testing.T, entries map[string]string) string {
	t.Helper()
	path := t.TempDir() + "/store.db"
	b, err := store.OpenLocalAt(path)
	if err != nil {
		t.Fatalf("seedStore open: %v", err)
	}
	for k, v := range entries {
		if err := b.Set(k, v); err != nil {
			t.Fatalf("seedStore set %q: %v", k, err)
		}
	}
	if err := b.Close(); err != nil {
		t.Fatalf("seedStore close: %v", err)
	}
	return path
}

// scopedOpener returns a StoreOpener that maps scope strings to
// on-disk paths. Each call opens a fresh backend — matches the
// production opener's contract that the caller owns Close.
func scopedOpener(paths map[string]string) StoreOpener {
	return func(scope string) (store.Backend, error) {
		path, ok := paths[scope]
		if !ok {
			return nil, nil
		}
		return store.OpenLocalAt(path)
	}
}

// mkdirAllPerm wraps os.MkdirAll with the conventional 0o755.
func mkdirAllPerm(path string) error {
	return os.MkdirAll(path, 0o755)
}

// seedAt opens a bbolt store at exactly the given path (rather than
// t.TempDir + suffix), writes entries, and closes. Used when the
// test needs the seed file at a specific location like
// ~/.config/factorly/store.db.
func seedAt(t *testing.T, path string, entries map[string]string) {
	t.Helper()
	b, err := store.OpenLocalAt(path)
	if err != nil {
		t.Fatalf("seedAt open %q: %v", path, err)
	}
	for k, v := range entries {
		if err := b.Set(k, v); err != nil {
			t.Fatalf("seedAt set %q: %v", k, err)
		}
	}
	if err := b.Close(); err != nil {
		t.Fatalf("seedAt close: %v", err)
	}
}

// inspectStore opens path read-only-ish (regular open; closes after
// inspection) so the test can verify what the handler wrote without
// holding the lock across assertions.
func inspectStore(t *testing.T, path string, key string) (string, error) {
	t.Helper()
	b, err := store.OpenLocalAt(path)
	if err != nil {
		t.Fatalf("inspect open: %v", err)
	}
	defer b.Close()
	return b.Get(key)
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
// data" path. Seeds a file at $HOME/.config/factorly/store.db (the
// path storeSections stat-probes before listing the global tier),
// then points the opener at it.
func TestStorePageRendersGlobalKeys(t *testing.T) {
	srv, _ := testServerWithProxy(t, nil)
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	cfgDir := tmpHome + "/.config/factorly"
	if err := mkdirAllPerm(cfgDir); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	path := cfgDir + "/store.db"
	seedAt(t, path, map[string]string{
		"deployment:sha": "abc123",
		"research:url:a": "hello",
	})
	srv.storeOpener = scopedOpener(map[string]string{"global": path})

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

// TestStoreSetWritesThroughOpener pins the POST /store handler to
// its happy path. The opener returns a fresh LocalBackend at a temp
// path; after the handler runs, we re-open the file to verify the
// write landed.
func TestStoreSetWritesThroughOpener(t *testing.T) {
	srv, _ := testServerWithProxy(t, nil)
	path := seedStoreFile(t, nil)
	srv.storeOpener = scopedOpener(map[string]string{"global": path})

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

	got, err := inspectStore(t, path, "my-key")
	if err != nil {
		t.Fatalf("inspect Get: %v", err)
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

// TestStoreEntryPageRendersValue exercises the new GET
// /store/entry?scope=...&key=... detail page. Should render the
// stored value, TTL badge, and the key itself.
func TestStoreEntryPageRendersValue(t *testing.T) {
	srv, _ := testServerWithProxy(t, nil)
	path := seedStoreFile(t, map[string]string{
		"deployment:sha": "abc123-the-full-value-here",
	})
	srv.storeOpener = scopedOpener(map[string]string{"global": path})

	req := httptest.NewRequest(http.MethodGet, "/store/entry?scope=global&key=deployment:sha", nil)
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{"deployment:sha", "abc123-the-full-value-here", "Global store"} {
		if !strings.Contains(body, want) {
			t.Errorf("page missing %q", want)
		}
	}
}

// TestStoreEntryUpdateRedirects confirms POST /store/entry persists
// the value and 303-redirects back to the detail page. The form is
// a plain <form method="POST">; body-level hx-boost makes the
// browser-style POST+303+GET feel like an in-page swap with no
// special handler logic.
func TestStoreEntryUpdateRedirects(t *testing.T) {
	srv, _ := testServerWithProxy(t, nil)
	path := seedStoreFile(t, map[string]string{"k": "old-value"})
	srv.storeOpener = scopedOpener(map[string]string{"global": path})

	form := url.Values{}
	form.Set("scope", "global")
	form.Set("key", "k")
	form.Set("value", "new-value")
	req := httptest.NewRequest(http.MethodPost, "/store/entry", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("expected 303 redirect, got %d body=%s", rec.Code, rec.Body.String())
	}
	loc := rec.Header().Get("Location")
	if !strings.Contains(loc, "/store/entry") || !strings.Contains(loc, "key=k") || !strings.Contains(loc, "scope=global") {
		t.Errorf("redirect Location = %q, want /store/entry?scope=global&key=k", loc)
	}

	// Confirm the file actually changed.
	got, err := inspectStore(t, path, "k")
	if err != nil {
		t.Fatalf("inspect: %v", err)
	}
	if got != "new-value" {
		t.Errorf("file value = %q, want new-value", got)
	}
}

// TestStoreEntryDelete exercises POST /store/entry/delete — the
// detail-page Delete form's submit target. After the redirect, the
// key should be gone from the file.
func TestStoreEntryDelete(t *testing.T) {
	srv, _ := testServerWithProxy(t, nil)
	path := seedStoreFile(t, map[string]string{"doomed": "x"})
	srv.storeOpener = scopedOpener(map[string]string{"global": path})

	form := url.Values{}
	form.Set("scope", "global")
	form.Set("key", "doomed")
	req := httptest.NewRequest(http.MethodPost, "/store/entry/delete", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("expected 303 redirect, got %d body=%s", rec.Code, rec.Body.String())
	}
	if loc := rec.Header().Get("Location"); loc != "/store" {
		t.Errorf("redirect Location = %q, want /store", loc)
	}

	if _, err := inspectStore(t, path, "doomed"); err == nil {
		t.Error("key still present after delete")
	}
}

// TestStoreEntryDeleteRejectsBadScope guards against arbitrary scope
// strings sneaking deletes into unexpected places.
func TestStoreEntryDeleteRejectsBadScope(t *testing.T) {
	srv, _ := testServerWithProxy(t, nil)
	form := url.Values{}
	form.Set("scope", "workspace:../escape")
	form.Set("key", "doomed")
	req := httptest.NewRequest(http.MethodPost, "/store/entry/delete", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d body=%s", rec.Code, rec.Body.String())
	}
}

// TestStoreEntryMissingKey404 confirms the detail page returns 404
// rather than rendering an empty value section for a key that
// doesn't exist.
func TestStoreEntryMissingKey404(t *testing.T) {
	srv, _ := testServerWithProxy(t, nil)
	path := seedStoreFile(t, nil) // empty store
	srv.storeOpener = scopedOpener(map[string]string{"global": path})

	req := httptest.NewRequest(http.MethodGet, "/store/entry?scope=global&key=nope", nil)
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rec.Code)
	}
}

// TestStoreDeleteRemovesKey covers the delete handler. Pre-seeds a
// key, hits DELETE, confirms the key is gone via the backend.
func TestStoreDeleteRemovesKey(t *testing.T) {
	srv, _ := testServerWithProxy(t, nil)
	path := seedStoreFile(t, map[string]string{"doomed": "x"})
	srv.storeOpener = scopedOpener(map[string]string{"global": path})

	req := httptest.NewRequest(http.MethodDelete, "/store/doomed?scope=global", nil)
	req.SetPathValue("key", "doomed")
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("DELETE status = %d, body = %s", rec.Code, rec.Body.String())
	}

	if _, err := inspectStore(t, path, "doomed"); err == nil {
		t.Error("key still present after delete")
	}
}
