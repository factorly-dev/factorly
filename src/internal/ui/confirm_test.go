// Copyright 2026 Jordan Sherer <hi@jordansherer.com>
// SPDX-License-Identifier: gpl

package ui

import (
	"context"
	"testing"
	"time"
)

func TestConfirmBroker_ApproveFlow(t *testing.T) {
	broker := NewConfirmBroker()

	// Simulate browser approving in background
	go func() {
		// Wait for request to appear
		for {
			pending := broker.Pending()
			if len(pending) > 0 {
				broker.Respond(pending[0].ID, true)
				return
			}
			time.Sleep(5 * time.Millisecond)
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	result := broker.Request(ctx, "dangerous.tool", map[string]string{"path": "/etc/hosts"})
	if !result {
		t.Error("expected approval, got denial")
	}
}

func TestConfirmBroker_DenyFlow(t *testing.T) {
	broker := NewConfirmBroker()

	go func() {
		for {
			pending := broker.Pending()
			if len(pending) > 0 {
				broker.Respond(pending[0].ID, false)
				return
			}
			time.Sleep(5 * time.Millisecond)
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	result := broker.Request(ctx, "dangerous.tool", nil)
	if result {
		t.Error("expected denial, got approval")
	}
}

func TestConfirmBroker_ContextCancelled(t *testing.T) {
	broker := NewConfirmBroker()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	// No one responds — should timeout and return false
	result := broker.Request(ctx, "slow.tool", nil)
	if result {
		t.Error("expected false on context cancellation")
	}
}

func TestConfirmBroker_RespondInvalidID(t *testing.T) {
	broker := NewConfirmBroker()

	ok := broker.Respond("nonexistent", true)
	if ok {
		t.Error("responding to nonexistent ID should return false")
	}
}

func TestConfirmBroker_PendingList(t *testing.T) {
	broker := NewConfirmBroker()

	// Start a request in background (will block)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		defer cancel()
		broker.Request(ctx, "tool.a", map[string]string{"x": "1"})
	}()

	// Wait for it to appear
	time.Sleep(20 * time.Millisecond)

	pending := broker.Pending()
	if len(pending) != 1 {
		t.Fatalf("expected 1 pending, got %d", len(pending))
	}
	if pending[0].Tool != "tool.a" {
		t.Errorf("expected tool.a, got %s", pending[0].Tool)
	}
	if pending[0].Params["x"] != "1" {
		t.Errorf("expected param x=1, got %v", pending[0].Params)
	}

	// Clean up
	broker.Respond(pending[0].ID, true)
}
