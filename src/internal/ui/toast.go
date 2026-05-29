// Copyright 2026 Jordan Sherer <hi@jordansherer.com>
// SPDX-License-Identifier: gpl

package ui

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/url"
)

// toastKind is the visual variant of a UI toast. Mirrors the STYLES
// map in static/toast.js.
type toastKind string

const (
	toastSuccess toastKind = "success"
	toastError   toastKind = "error"
	toastInfo    toastKind = "info"
)

// flashCookie carries a one-shot toast across a navigation. See toast()
// for why we deliver via BOTH a cookie and an HX-Trigger header. It is
// intentionally NOT HttpOnly so static/toast.js can read it.
const flashCookie = "factorly_flash"

// toast shows a one-shot UI toast after the current action. It fires
// regardless of whether the handler redirects or returns an htmx
// fragment swap — so callers no longer have to know which.
//
// We deliver the SAME toast two ways and let the client dedupe:
//   - HX-Trigger header — fires immediately on the current page (the
//     reliable path for fragment-swap responses).
//   - flash cookie — read by static/toast.js on the next page load
//     (the reliable path across redirects, where a header is dropped).
//
// Both payloads carry the same short id. toast.js keeps a small FIFO
// set of seen ids and pops each toast once; whichever delivery arrives
// first wins, the other is ignored. This removes the old fragment-vs-
// redirect decision from every call site.
//
// No-op when msg is empty. Must be called before the response
// status/body is written (it sets a header and a Set-Cookie).
func toast(w http.ResponseWriter, kind toastKind, msg string) {
	if msg == "" {
		return
	}
	id := newToastID()

	// HX-Trigger (immediate, current page).
	payload := map[string]any{
		"toast": map[string]string{
			"id":   id,
			"msg":  msg,
			"kind": string(kind),
		},
	}
	if b, err := json.Marshal(payload); err == nil {
		w.Header().Set("HX-Trigger", string(b))
	}

	// Flash cookie (next page, survives redirects). Format: id|kind|msg.
	http.SetCookie(w, &http.Cookie{
		Name:     flashCookie,
		Value:    url.QueryEscape(id + "|" + string(kind) + "|" + msg),
		Path:     "/",
		MaxAge:   10, // seconds — just long enough to ride one redirect
		SameSite: http.SameSiteLaxMode,
	})
}

// newToastID returns a short random hex id used to dedupe a toast's
// two deliveries (HX-Trigger + cookie). 4 bytes is ample — ids only
// need to be distinct among the handful live in the client's FIFO
// window. Falls back to a fixed string if the RNG fails (worst case:
// two near-simultaneous toasts dedupe into one, which is harmless).
func newToastID() string {
	var b [4]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "toast"
	}
	return hex.EncodeToString(b[:])
}
