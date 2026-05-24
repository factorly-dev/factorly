// Copyright 2026 Jordan Sherer <hi@jordansherer.com>
// SPDX-License-Identifier: gpl

package code

import (
	"context"
	"errors"
	"sort"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// fakeStore is a minimal in-memory implementation of the per-script
// StoreOps surface. Tests inject one via SetStoreOpener and assert
// against its recorded state after a script runs.
type fakeStore struct {
	data map[string]string
	// ttlCalls records the ttl arg of every SetWithTTL invocation, in
	// order, so a test can verify SetWithTTL was used (not Set) and
	// that the duration was passed through unchanged.
	ttlCalls []time.Duration
	// listCalls / deleteCalls track call volume for assertion that
	// the script hit the right methods.
	listCalls   int32
	deleteCalls int32
}

func newFakeStore() *fakeStore {
	return &fakeStore{data: map[string]string{}}
}

func (f *fakeStore) ops() *StoreOps {
	return &StoreOps{
		Get: func(key string) (string, error) {
			v, ok := f.data[key]
			if !ok {
				return "", ErrStoreNotFound
			}
			return v, nil
		},
		Set: func(key, value string) error {
			f.data[key] = value
			return nil
		},
		SetWithTTL: func(key, value string, ttl time.Duration) error {
			f.data[key] = value
			f.ttlCalls = append(f.ttlCalls, ttl)
			return nil
		},
		Delete: func(key string) error {
			atomic.AddInt32(&f.deleteCalls, 1)
			delete(f.data, key)
			return nil // idempotent
		},
		List: func() ([]string, error) {
			atomic.AddInt32(&f.listCalls, 1)
			keys := make([]string, 0, len(f.data))
			for k := range f.data {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			return keys, nil
		},
	}
}

// runScriptWithStore is a small harness that wires a fake store into
// the provider, runs a script, and returns the provider Result so a
// test can assert on Output / Error / ExitCode without going through
// p.Execute (which would require a registered tool name).
func runScriptWithStore(t *testing.T, src string, fs *fakeStore) (output, errStr string, exitCode int) {
	t.Helper()
	p := NewProvider(&fakeExecutor{}, false)
	p.SetStoreOpener(func(ctx context.Context) *StoreOps {
		return fs.ops()
	})
	res, err := p.Run(context.Background(), src, nil, 0)
	if err != nil {
		t.Fatalf("Run returned infrastructure error: %v", err)
	}
	return res.Output, res.Error, res.ExitCode
}

func TestStoreHandle_NilOpenerYieldsNotConfiguredError(t *testing.T) {
	// Without SetStoreOpener, factorly.Store.Get must return a clear
	// runtime error rather than panic. This is the contract for
	// hosts that opt out of giving scripts store access.
	p := NewProvider(&fakeExecutor{}, false)
	src := `package main
import (
    "errors"
    "factorly"
)
func Run(p map[string]string) (any, error) {
    _, err := factorly.Store.Get("k")
    if err == nil { return nil, errors.New("expected an error") }
    return err.Error(), nil
}`
	res, err := p.Run(context.Background(), src, nil, 0)
	if err != nil {
		t.Fatalf("infrastructure error: %v", err)
	}
	if res.ExitCode != 0 {
		t.Fatalf("script failed unexpectedly: %s", res.Error)
	}
	if !strings.Contains(res.Output, "not configured") {
		t.Errorf("expected 'not configured' message, got %q", res.Output)
	}
}

func TestStoreHandle_GetReturnsValue(t *testing.T) {
	fs := newFakeStore()
	fs.data["greeting"] = "hello"

	out, errStr, code := runScriptWithStore(t, `package main
import "factorly"
func Run(p map[string]string) (any, error) {
    return factorly.Store.Get("greeting")
}`, fs)
	if code != 0 {
		t.Fatalf("script failed: %s", errStr)
	}
	if out != "hello" {
		t.Errorf("Get returned %q, want %q", out, "hello")
	}
}

func TestStoreHandle_GetMissingReturnsErrStoreNotFound(t *testing.T) {
	// The sentinel must round-trip through the interpreter so a
	// script can `errors.Is(err, factorly.ErrStoreNotFound)` to
	// branch on absence. Without this, the only way to detect a
	// miss is brittle string matching.
	fs := newFakeStore()
	out, errStr, code := runScriptWithStore(t, `package main
import (
    "errors"
    "factorly"
)
func Run(p map[string]string) (any, error) {
    _, err := factorly.Store.Get("never-set")
    if errors.Is(err, factorly.ErrStoreNotFound) {
        return "missing-detected", nil
    }
    return "wrong-error: " + err.Error(), nil
}`, fs)
	if code != 0 {
		t.Fatalf("script failed: %s", errStr)
	}
	if out != "missing-detected" {
		t.Errorf("expected ErrStoreNotFound branch, got %q", out)
	}
}

func TestStoreHandle_SetWritesValue(t *testing.T) {
	fs := newFakeStore()
	_, errStr, code := runScriptWithStore(t, `package main
import "factorly"
func Run(p map[string]string) (any, error) {
    return nil, factorly.Store.Set("k", "v")
}`, fs)
	if code != 0 {
		t.Fatalf("script failed: %s", errStr)
	}
	if got := fs.data["k"]; got != "v" {
		t.Errorf("Set didn't persist: got %q", got)
	}
}

func TestStoreHandle_SetWithTTLPassesDuration(t *testing.T) {
	// SetWithTTL must take a time.Duration (not a string), and the
	// value must reach the host unchanged. Pin both contracts here
	// so a future refactor to e.g. string TTL fails loudly.
	fs := newFakeStore()
	_, errStr, code := runScriptWithStore(t, `package main
import (
    "factorly"
    "time"
)
func Run(p map[string]string) (any, error) {
    return nil, factorly.Store.SetWithTTL("session.token", "abc", 50*time.Minute)
}`, fs)
	if code != 0 {
		t.Fatalf("script failed: %s", errStr)
	}
	if got := fs.data["session.token"]; got != "abc" {
		t.Errorf("SetWithTTL didn't persist value: got %q", got)
	}
	if len(fs.ttlCalls) != 1 {
		t.Fatalf("expected exactly 1 SetWithTTL call, got %d", len(fs.ttlCalls))
	}
	if fs.ttlCalls[0] != 50*time.Minute {
		t.Errorf("ttl arg mismatch: got %v, want %v", fs.ttlCalls[0], 50*time.Minute)
	}
}

func TestStoreHandle_SetWithTTLZeroMeansNeverExpire(t *testing.T) {
	// 0 must pass through as 0 (not get rewritten to a default).
	// The host-side opener interprets 0 as "never expire."
	fs := newFakeStore()
	_, errStr, code := runScriptWithStore(t, `package main
import "factorly"
func Run(p map[string]string) (any, error) {
    return nil, factorly.Store.SetWithTTL("k", "v", 0)
}`, fs)
	if code != 0 {
		t.Fatalf("script failed: %s", errStr)
	}
	if len(fs.ttlCalls) != 1 || fs.ttlCalls[0] != 0 {
		t.Errorf("expected ttl=0 to round-trip, got %#v", fs.ttlCalls)
	}
}

func TestStoreHandle_DeleteIsIdempotent(t *testing.T) {
	// Deleting a missing key must NOT return ErrStoreNotFound. This
	// matches the CLI's `factorly store delete` and the builtin's
	// behavior.
	fs := newFakeStore()
	_, errStr, code := runScriptWithStore(t, `package main
import "factorly"
func Run(p map[string]string) (any, error) {
    return nil, factorly.Store.Delete("never-existed")
}`, fs)
	if code != 0 {
		t.Fatalf("script failed: %s", errStr)
	}
	if fs.deleteCalls != 1 {
		t.Errorf("expected one Delete call, got %d", fs.deleteCalls)
	}
}

func TestStoreHandle_ListReturnsSortedKeys(t *testing.T) {
	fs := newFakeStore()
	fs.data["b"] = "2"
	fs.data["a"] = "1"
	fs.data["c"] = "3"

	// Script returns the keys joined so we can assert on a single string.
	out, errStr, code := runScriptWithStore(t, `package main
import (
    "factorly"
    "strings"
)
func Run(p map[string]string) (any, error) {
    keys, err := factorly.Store.List()
    if err != nil { return nil, err }
    return strings.Join(keys, ","), nil
}`, fs)
	if code != 0 {
		t.Fatalf("script failed: %s", errStr)
	}
	if out != "a,b,c" {
		t.Errorf("expected sorted keys 'a,b,c', got %q", out)
	}
}

// TestStoreHandle_BackendErrorSurfaces ensures that a host-side error
// (e.g. bbolt open failure) propagates verbatim through the handle so
// the script can react. Without this, real I/O failures would be
// invisible to the script.
func TestStoreHandle_BackendErrorSurfaces(t *testing.T) {
	wantErr := errors.New("store: bbolt locked by another process")
	p := NewProvider(&fakeExecutor{}, false)
	p.SetStoreOpener(func(ctx context.Context) *StoreOps {
		return &StoreOps{
			Get: func(key string) (string, error) { return "", wantErr },
		}
	})
	src := `package main
import "factorly"
func Run(p map[string]string) (any, error) {
    _, err := factorly.Store.Get("k")
    if err == nil { return "no-error", nil }
    return err.Error(), nil
}`
	res, err := p.Run(context.Background(), src, nil, 0)
	if err != nil {
		t.Fatalf("infrastructure error: %v", err)
	}
	if res.ExitCode != 0 {
		t.Fatalf("script failed: %s", res.Error)
	}
	if !strings.Contains(res.Output, wantErr.Error()) {
		t.Errorf("expected backend error to surface, got %q", res.Output)
	}
}

// TestStoreHandle_FreshOpsPerRun verifies that the opener is invoked
// for each Run, not at provider construction time. This is important
// because the host-side closure may snapshot per-call context
// (active workspace, tier targeting) that changes between runs.
func TestStoreHandle_FreshOpsPerRun(t *testing.T) {
	var openerCalls int32
	p := NewProvider(&fakeExecutor{}, false)
	p.SetStoreOpener(func(ctx context.Context) *StoreOps {
		atomic.AddInt32(&openerCalls, 1)
		return newFakeStore().ops()
	})
	src := `package main
import "factorly"
func Run(p map[string]string) (any, error) {
    return factorly.Store.Set("k", "v"), nil
}`
	for i := 0; i < 3; i++ {
		if _, err := p.Run(context.Background(), src, nil, 0); err != nil {
			t.Fatalf("run %d: %v", i, err)
		}
	}
	if openerCalls != 3 {
		t.Errorf("expected opener to fire 3 times (one per Run), got %d", openerCalls)
	}
}

// TestSetStoreOpenerRaceFreeWithConcurrentRuns is a regression guard
// for the RLock/Lock dance in code.go's SetStoreOpener vs Run paths.
// We don't assert specific behavior — just that go test -race doesn't
// flag a data race when SetStoreOpener and Run are interleaved.
func TestSetStoreOpenerRaceFreeWithConcurrentRuns(t *testing.T) {
	p := NewProvider(&fakeExecutor{}, false)
	src := `package main
import "factorly"
func Run(p map[string]string) (any, error) {
    return factorly.Store.Get("k")  // expected to err with "not configured" sometimes
}`
	done := make(chan struct{})
	go func() {
		for i := 0; i < 20; i++ {
			fs := newFakeStore()
			p.SetStoreOpener(func(ctx context.Context) *StoreOps { return fs.ops() })
		}
		close(done)
	}()
	for i := 0; i < 20; i++ {
		_, _ = p.Run(context.Background(), src, nil, 0)
	}
	<-done
}
