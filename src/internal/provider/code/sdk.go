// Copyright 2026 Jordan Sherer <hi@jordansherer.com>
// SPDX-License-Identifier: gpl

package code

import (
	"errors"
	"time"
)

// Result is the value returned by factorly.Call inside a script. It
// intentionally mirrors provider.Result but is declared in this package
// so scripts can import "factorly" and reference factorly.Result without
// pulling the provider package into the interpreter's symbol set.
type Result struct {
	Output   string
	Error    string
	ExitCode int
	Duration time.Duration
}

// ErrStoreNotFound is returned by factorly.Store.Get when the requested
// key isn't present in the active store tier (after the workspace →
// project cascade). Mirrors store.ErrNotFound but lives in this package
// so scripts that import "factorly" don't have to also know about the
// internal/store package.
var ErrStoreNotFound = errors.New("store: key not found")

// StoreHandle is the in-script SDK surface for the workspace store. A
// fresh handle is injected per script execution; methods close over the
// backend opener supplied by the host, so the cascade / tier targeting
// is whatever the outer call's tier was (workspace if active, project
// otherwise). Concurrent calls to handle methods are safe because each
// method opens and closes its own short-lived bbolt handle — no shared
// state inside the handle itself.
//
// The handle is exposed to scripts as factorly.Store (a struct value
// registered via factorlyExports). Scripts call store methods directly:
//
//	val, err := factorly.Store.Get("research:url:" + u)
//	if errors.Is(err, factorly.ErrStoreNotFound) { ... }
//	factorly.Store.SetWithTTL("session.token", t, 50*time.Minute)
//
// All methods log writes through the same audit path the CLI uses so
// `factorly store history <key>` surfaces both CLI and script writes.
type StoreHandle struct {
	// get / set / setWithTTL / del / list are injected at runtime. Nil
	// when no opener was wired (test fixtures, agent-supplied
	// factorly.code calls in HTTP mode that have no store).
	get        func(key string) (string, error)
	set        func(key, value string) error
	setWithTTL func(key, value string, ttl time.Duration) error
	del        func(key string) error
	list       func() ([]string, error)
}

// Get returns the value stored under key. Reads cascade workspace →
// project. Refresh-on-read: a successful Get resets the entry's TTL
// window so frequently-touched entries stay alive. Returns
// (factorly.ErrStoreNotFound, ...) when the key is missing or expired.
func (h *StoreHandle) Get(key string) (string, error) {
	if h == nil || h.get == nil {
		return "", errors.New("store: handle not configured")
	}
	return h.get(key)
}

// Set writes value under key using the backend's default TTL (30 days
// with refresh-on-read). Writes target the active tier (workspace if
// one is active, otherwise project).
func (h *StoreHandle) Set(key, value string) error {
	if h == nil || h.set == nil {
		return errors.New("store: handle not configured")
	}
	return h.set(key, value)
}

// SetWithTTL writes value under key with an explicit TTL. ttl == 0
// means never expire; positive durations expire after that period;
// negative durations are treated as 0. Writes target the active tier
// (same as Set).
func (h *StoreHandle) SetWithTTL(key, value string, ttl time.Duration) error {
	if h == nil || h.setWithTTL == nil {
		return errors.New("store: handle not configured")
	}
	return h.setWithTTL(key, value, ttl)
}

// Delete removes a key. Idempotent — deleting a missing key returns
// nil, matching the CLI's `factorly store delete` behavior.
func (h *StoreHandle) Delete(key string) error {
	if h == nil || h.del == nil {
		return errors.New("store: handle not configured")
	}
	return h.del(key)
}

// List returns every key in the active store tier, sorted. Workspace
// reads cascade through to the project tier just like Get.
func (h *StoreHandle) List() ([]string, error) {
	if h == nil || h.list == nil {
		return nil, errors.New("store: handle not configured")
	}
	return h.list()
}

// IsError reports whether the call failed (nonzero exit or non-empty error).
func (r *Result) IsError() bool {
	if r == nil {
		return true
	}
	return r.ExitCode != 0 || r.Error != ""
}

// ToolInfo describes one registered tool surfaced into the in-script
// SDK via factorly.ListTools(). Hidden tools are excluded.
type ToolInfo struct {
	Name        string
	Description string
	Parameters  []ParamInfo
}

// ParamInfo describes one parameter of a tool. Type is the declared
// type or "string" by default.
type ParamInfo struct {
	Name        string
	Type        string
	Required    bool
	Description string
	Default     string
}
