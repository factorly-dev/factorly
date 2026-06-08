// Copyright 2026 Jordan Sherer <hi@jordansherer.com>
// SPDX-License-Identifier: gpl

package main

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/factorly-dev/factorly/internal/ui"
)

// TestFactorlyAsk_NilBroker covers the headless code path: when no UI
// is running, the handler must return a clean operator-readable error
// instead of blocking forever.
func TestFactorlyAsk_NilBroker(t *testing.T) {
	h := makeFactorlyAskHandler(func() *ui.AskBroker { return nil })
	res, err := h(context.Background(), map[string]string{
		"name":        "env",
		"description": "x",
	})
	if err != nil {
		t.Fatalf("unexpected go-error: %v", err)
	}
	if res == nil || res.ExitCode == 0 {
		t.Fatalf("expected ExitCode != 0, got %+v", res)
	}
	if !strings.Contains(res.Error, "UI") {
		t.Errorf("error %q should mention the UI requirement", res.Error)
	}
}

// TestFactorlyAsk_HappyPath drives a real AskBroker through the
// builtin handler with a background goroutine that responds.
func TestFactorlyAsk_HappyPath(t *testing.T) {
	broker := ui.NewAskBroker()
	h := makeFactorlyAskHandler(func() *ui.AskBroker { return broker })

	go func() {
		for {
			pending := broker.Pending()
			if len(pending) > 0 {
				broker.Respond(pending[0].ID, "staging")
				return
			}
			time.Sleep(5 * time.Millisecond)
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	res, err := h(ctx, map[string]string{
		"name":        "env",
		"description": "Which environment?",
		"choices":     "staging,prod",
	})
	if err != nil {
		t.Fatalf("handler returned go-error: %v", err)
	}
	if res.Output != "staging" {
		t.Errorf("output = %q, want staging", res.Output)
	}
}

// TestFactorlyAsk_Cancelled: cancel button → ExitCode 1 with a
// "cancelled" message so the workflow operator sees why the step
// failed.
func TestFactorlyAsk_Cancelled(t *testing.T) {
	broker := ui.NewAskBroker()
	h := makeFactorlyAskHandler(func() *ui.AskBroker { return broker })

	go func() {
		for {
			p := broker.Pending()
			if len(p) > 0 {
				broker.Cancel(p[0].ID)
				return
			}
			time.Sleep(5 * time.Millisecond)
		}
	}()

	res, err := h(context.Background(), map[string]string{"name": "q"})
	if err != nil {
		t.Fatalf("unexpected go-error: %v", err)
	}
	if res.ExitCode == 0 || !strings.Contains(res.Error, "cancelled") {
		t.Errorf("expected cancelled error, got %+v", res)
	}
}

// TestFactorlyAsk_Timeout: no responder + a short timeout param —
// expect the deadline error path to fire with a message that names
// the configured timeout.
func TestFactorlyAsk_Timeout(t *testing.T) {
	broker := ui.NewAskBroker()
	h := makeFactorlyAskHandler(func() *ui.AskBroker { return broker })

	start := time.Now()
	res, err := h(context.Background(), map[string]string{
		"name":    "q",
		"timeout": "100ms",
	})
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("unexpected go-error: %v", err)
	}
	if res.ExitCode == 0 || !strings.Contains(res.Error, "timed out") {
		t.Errorf("expected timeout error, got %+v", res)
	}
	if !strings.Contains(res.Error, "100ms") {
		t.Errorf("error %q should name the configured timeout", res.Error)
	}
	// Sanity: the run should actually have lasted ~100ms, not the
	// 5m default.
	if elapsed > time.Second {
		t.Errorf("handler ran too long: %v", elapsed)
	}
}

// TestBuildAskRequest_ChoicesImpliesEnum: when a workflow author uses
// `choices: a,b,c` without setting `type`, the resulting AskRequest
// should be type=enum and have Enum populated, so the browser renders
// a <select>.
func TestBuildAskRequest_ChoicesImpliesEnum(t *testing.T) {
	req, perr := buildAskRequest(map[string]string{
		"name":    "env",
		"choices": "staging, prod, dev",
	})
	if perr != "" {
		t.Fatalf("buildAskRequest: %s", perr)
	}
	if req.Type != "enum" {
		t.Errorf("Type = %q, want enum", req.Type)
	}
	if len(req.Enum) != 3 || req.Enum[0] != "staging" || req.Enum[2] != "dev" {
		t.Errorf("Enum = %v, want [staging prod dev]", req.Enum)
	}
}

// TestBuildAskRequest_MissingName: `name` is required — when the
// param is missing the handler should reject before touching the
// broker.
func TestBuildAskRequest_MissingName(t *testing.T) {
	_, perr := buildAskRequest(map[string]string{
		"description": "x",
	})
	if perr == "" {
		t.Fatal("expected missing-name error")
	}
}

