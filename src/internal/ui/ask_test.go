// Copyright 2026 Jordan Sherer <hi@jordansherer.com>
// SPDX-License-Identifier: gpl

package ui

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestAskBroker_AnswerFlow(t *testing.T) {
	broker := NewAskBroker()

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

	answer, err := broker.Request(ctx, AskRequest{Name: "env", Type: "string"})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if answer != "staging" {
		t.Errorf("answer = %q, want staging", answer)
	}
}

func TestAskBroker_CancelFlow(t *testing.T) {
	broker := NewAskBroker()

	go func() {
		for {
			pending := broker.Pending()
			if len(pending) > 0 {
				broker.Cancel(pending[0].ID)
				return
			}
			time.Sleep(5 * time.Millisecond)
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_, err := broker.Request(ctx, AskRequest{Name: "env"})
	if !errors.Is(err, ErrAskCancelled) {
		t.Errorf("err = %v, want ErrAskCancelled", err)
	}
}

func TestAskBroker_ContextCancelled(t *testing.T) {
	broker := NewAskBroker()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err := broker.Request(ctx, AskRequest{Name: "x"})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("err = %v, want context.DeadlineExceeded", err)
	}
	// After ctx cancel, the request should be cleaned up — the
	// defer in Request removes it. Verify so the pending list
	// doesn't leak.
	if got := len(broker.Pending()); got != 0 {
		t.Errorf("Pending after cancel = %d, want 0", got)
	}
}

func TestAskBroker_ConcurrentRequestsDistinctIDs(t *testing.T) {
	broker := NewAskBroker()

	const n = 5
	answers := make(chan string, n)
	for i := 0; i < n; i++ {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
			defer cancel()
			a, err := broker.Request(ctx, AskRequest{Name: "q"})
			if err != nil {
				answers <- "ERR:" + err.Error()
				return
			}
			answers <- a
		}()
	}

	// Wait until all requests are pending.
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if len(broker.Pending()) >= n {
			break
		}
		time.Sleep(2 * time.Millisecond)
	}
	pending := broker.Pending()
	if len(pending) != n {
		t.Fatalf("pending = %d, want %d", len(pending), n)
	}

	// IDs must all be distinct.
	seen := map[string]bool{}
	for _, p := range pending {
		if seen[p.ID] {
			t.Errorf("duplicate ID: %s", p.ID)
		}
		seen[p.ID] = true
	}

	// Respond — each request gets its own ID back.
	for _, p := range pending {
		broker.Respond(p.ID, p.ID) // echo the ID as the answer
	}
	collected := map[string]bool{}
	for i := 0; i < n; i++ {
		a := <-answers
		collected[a] = true
	}
	if len(collected) != n {
		t.Errorf("got %d distinct answers, want %d (%v)", len(collected), n, collected)
	}
}

func TestAskBroker_RespondInvalidID(t *testing.T) {
	broker := NewAskBroker()
	if ok := broker.Respond("ask-nope", "x"); ok {
		t.Error("Respond on unknown ID should return false")
	}
	if ok := broker.Cancel("ask-nope"); ok {
		t.Error("Cancel on unknown ID should return false")
	}
}

func TestValidateAskAnswer_RequiredEmpty(t *testing.T) {
	req := &AskRequest{Name: "env", Type: "string", Required: true}
	_, errs := validateAskAnswer(req, "  ")
	if len(errs) == 0 {
		t.Error("required + whitespace-only should error")
	}
}

func TestValidateAskAnswer_EnumPasses(t *testing.T) {
	req := &AskRequest{Name: "env", Type: "string", Required: true,
		Enum: []string{"staging", "prod"}}
	got, errs := validateAskAnswer(req, "prod")
	if len(errs) != 0 {
		t.Errorf("unexpected errs: %v", errs)
	}
	if got != "prod" {
		t.Errorf("got %q, want prod", got)
	}
}

func TestValidateAskAnswer_EnumRejects(t *testing.T) {
	req := &AskRequest{Name: "env", Type: "string", Required: true,
		Enum: []string{"staging", "prod"}}
	_, errs := validateAskAnswer(req, "dev")
	if len(errs) == 0 {
		t.Error("expected enum mismatch error")
	}
}

func TestValidateAskAnswer_BooleanCoerces(t *testing.T) {
	req := &AskRequest{Name: "f", Type: "boolean", Required: true}
	got, errs := validateAskAnswer(req, "yes")
	if len(errs) != 0 {
		t.Errorf("unexpected errs: %v", errs)
	}
	if got != "true" {
		t.Errorf("got %q, want 'true' (boolean coercion)", got)
	}
}
