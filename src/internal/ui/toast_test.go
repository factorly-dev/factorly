// Copyright 2026 Jordan Sherer <hi@jordansherer.com>
// SPDX-License-Identifier: gpl

package ui

import (
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// TestToast_EmitsBothDeliveries confirms a single toast() call sets
// BOTH an HX-Trigger header (immediate, current page) and a flash
// cookie (next page), carrying the SAME id so the client dedupes.
func TestToast_EmitsBothDeliveries(t *testing.T) {
	w := httptest.NewRecorder()
	toast(w, toastSuccess, "Saved secret API_KEY")

	// HX-Trigger header present with id/msg/kind.
	trig := w.Header().Get("HX-Trigger")
	if !strings.Contains(trig, `"toast"`) ||
		!strings.Contains(trig, `"msg":"Saved secret API_KEY"`) ||
		!strings.Contains(trig, `"kind":"success"`) ||
		!strings.Contains(trig, `"id":`) {
		t.Fatalf("HX-Trigger missing fields: %q", trig)
	}

	// Flash cookie present, format id|kind|msg.
	var cookieVal string
	for _, c := range w.Result().Cookies() {
		if c.Name == flashCookie {
			cookieVal = c.Value
		}
	}
	if cookieVal == "" {
		t.Fatal("expected flash cookie")
	}
	decoded, err := url.QueryUnescape(cookieVal)
	if err != nil {
		t.Fatalf("cookie not decodable: %v", err)
	}
	parts := strings.SplitN(decoded, "|", 3)
	if len(parts) != 3 {
		t.Fatalf("cookie value = %q, want id|kind|msg", decoded)
	}
	if parts[1] != "success" || parts[2] != "Saved secret API_KEY" {
		t.Errorf("cookie kind/msg = %q/%q, want success/'Saved secret API_KEY'", parts[1], parts[2])
	}

	// The id in the header must equal the id in the cookie — that's
	// what lets the client dedupe the two deliveries.
	cookieID := parts[0]
	if cookieID == "" {
		t.Fatal("cookie id is empty")
	}
	if !strings.Contains(trig, `"id":"`+cookieID+`"`) {
		t.Errorf("header id and cookie id differ; header=%q cookieID=%q", trig, cookieID)
	}
}

// TestToast_UniqueIDsPerCall confirms two toast() calls produce
// different ids (so two distinct actions don't dedupe into one).
func TestToast_UniqueIDsPerCall(t *testing.T) {
	id := func() string {
		w := httptest.NewRecorder()
		toast(w, toastInfo, "x")
		for _, c := range w.Result().Cookies() {
			if c.Name == flashCookie {
				dec, _ := url.QueryUnescape(c.Value)
				return strings.SplitN(dec, "|", 3)[0]
			}
		}
		return ""
	}
	if a, b := id(), id(); a == "" || a == b {
		t.Errorf("expected distinct non-empty ids, got %q and %q", a, b)
	}
}

// TestToast_EmptyMessageNoOp confirms an empty message sets neither
// a cookie nor a header.
func TestToast_EmptyMessageNoOp(t *testing.T) {
	w := httptest.NewRecorder()
	toast(w, toastInfo, "")
	if w.Header().Get("HX-Trigger") != "" {
		t.Error("empty message should not set HX-Trigger")
	}
	for _, c := range w.Result().Cookies() {
		if c.Name == flashCookie {
			t.Error("empty message should not set a flash cookie")
		}
	}
}

// TestToast_EncodesSpecialChars confirms a message containing the
// cookie delimiters round-trips. The cookie value is URL-encoded as a
// whole, and the client splits on the first two '|' only, so a '|' in
// the message body survives.
func TestToast_EncodesSpecialChars(t *testing.T) {
	w := httptest.NewRecorder()
	toast(w, toastError, "blocked: a|b; c=d")
	var v string
	for _, c := range w.Result().Cookies() {
		if c.Name == flashCookie {
			v = c.Value
		}
	}
	if strings.ContainsAny(v, "; ") {
		t.Errorf("raw cookie value must be encoded (no literal ; or space): %q", v)
	}
	decoded, _ := url.QueryUnescape(v)
	// id|error|blocked: a|b; c=d  → splitN(3) → [id, "error", "blocked: a|b; c=d"]
	parts := strings.SplitN(decoded, "|", 3)
	if len(parts) != 3 || parts[1] != "error" || parts[2] != "blocked: a|b; c=d" {
		t.Errorf("decoded parts = %v, want [id error 'blocked: a|b; c=d']", parts)
	}
}
