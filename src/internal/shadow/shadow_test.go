// Copyright 2026 Jordan Sherer <hi@jordansherer.com>
// SPDX-License-Identifier: gpl

package shadow

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/factorly-dev/factorly/internal/agent"
)

func testPolicy(rules map[string]*Rule, confirmFn ConfirmFunc, t *testing.T) *Policy {
	t.Helper()
	path := filepath.Join(t.TempDir(), "ratelimit.json")
	return New(rules, confirmFn, path)
}

// --- Deny ---

func TestDenyBlocksTool(t *testing.T) {
	p := testPolicy(map[string]*Rule{
		"github": {Deny: []string{"delete_repository"}},
	}, nil, t)

	action, err := p.Check(context.Background(), "github.delete_repository", nil, "cli")
	if err == nil {
		t.Fatal("expected error for denied tool")
	}
	if action != ActionDenied {
		t.Errorf("expected ActionDenied, got %s", action)
	}
	if !strings.Contains(err.Error(), "denied") {
		t.Errorf("expected 'denied' in error, got: %s", err.Error())
	}
}

func TestDenyPassesUnmatchedTool(t *testing.T) {
	p := testPolicy(map[string]*Rule{
		"github": {Deny: []string{"delete_repository"}},
	}, nil, t)

	action, err := p.Check(context.Background(), "github.list_repos", nil, "cli")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if action != ActionAllowed {
		t.Errorf("expected ActionAllowed, got %s", action)
	}
}

func TestDenyExactMatch(t *testing.T) {
	p := testPolicy(map[string]*Rule{
		"stripe.charge": {Deny: []string{"stripe.charge"}},
	}, nil, t)

	action, err := p.Check(context.Background(), "stripe.charge", nil, "cli")
	if err == nil {
		t.Fatal("expected error for denied tool")
	}
	if action != ActionDenied {
		t.Errorf("expected ActionDenied, got %s", action)
	}
}

// --- Confirm ---

func TestConfirmApproved(t *testing.T) {
	p := testPolicy(map[string]*Rule{
		"github": {Confirm: []string{"merge_pull_request"}},
	}, func(ctx context.Context, name string, params map[string]string) bool {
		return true
	}, t)

	action, err := p.Check(context.Background(), "github.merge_pull_request", nil, "cli")
	if err != nil {
		t.Fatal(err)
	}
	if action != ActionConfirmed {
		t.Errorf("expected ActionConfirmed, got %s", action)
	}
}

func TestConfirmDeclined(t *testing.T) {
	p := testPolicy(map[string]*Rule{
		"github": {Confirm: []string{"merge_pull_request"}},
	}, func(ctx context.Context, name string, params map[string]string) bool {
		return false
	}, t)

	action, err := p.Check(context.Background(), "github.merge_pull_request", nil, "cli")
	if err == nil {
		t.Fatal("expected error for declined confirm")
	}
	if action != ActionDenied {
		t.Errorf("expected ActionDenied, got %s", action)
	}
}

func TestConfirmNoHandler(t *testing.T) {
	p := testPolicy(map[string]*Rule{
		"github": {Confirm: []string{"merge_pull_request"}},
	}, nil, t)

	action, err := p.Check(context.Background(), "github.merge_pull_request", nil, "mcp")
	if err == nil {
		t.Fatal("expected error when no confirm handler")
	}
	if action != ActionDenied {
		t.Errorf("expected ActionDenied, got %s", action)
	}
}

func TestConfirmAll(t *testing.T) {
	called := false
	p := testPolicy(map[string]*Rule{
		"stripe.charge": {ConfirmAll: true},
	}, func(ctx context.Context, name string, params map[string]string) bool {
		called = true
		return true
	}, t)

	action, _ := p.Check(context.Background(), "stripe.charge", nil, "cli")
	if !called {
		t.Error("expected confirm function to be called")
	}
	if action != ActionConfirmed {
		t.Errorf("expected ActionConfirmed, got %s", action)
	}
}

// --- Rate Limit ---

func TestRateLimitAllowsWithinWindow(t *testing.T) {
	p := testPolicy(map[string]*Rule{
		"api": {RateLimit: &RateLimit{Count: 3, Window: time.Minute}},
	}, nil, t)

	for i := 0; i < 3; i++ {
		action, err := p.Check(context.Background(), "api.call", nil, "cli")
		if err != nil {
			t.Fatalf("call %d: unexpected error: %v", i+1, err)
		}
		if action != ActionAllowed {
			t.Errorf("call %d: expected ActionAllowed, got %s", i+1, action)
		}
	}
}

func TestRateLimitBlocksAtThreshold(t *testing.T) {
	p := testPolicy(map[string]*Rule{
		"api": {RateLimit: &RateLimit{Count: 2, Window: time.Minute}},
	}, nil, t)

	// First two calls succeed
	_, _ = p.Check(context.Background(), "api.call", nil, "cli")
	_, _ = p.Check(context.Background(), "api.call", nil, "cli")

	// Third call should be rate limited
	action, err := p.Check(context.Background(), "api.call", nil, "cli")
	if err == nil {
		t.Fatal("expected rate limit error")
	}
	if action != ActionRateLimited {
		t.Errorf("expected ActionRateLimited, got %s", action)
	}
	if !strings.Contains(err.Error(), "rate limited") {
		t.Errorf("expected 'rate limited' in error, got: %s", err.Error())
	}
}

func TestRateLimitTokenBucketSmoothing(t *testing.T) {
	// With a 2/minute limit, token bucket should allow:
	// - First 2 calls immediately (initial tokens)
	// - Then ~30 seconds between each subsequent call
	p := testPolicy(map[string]*Rule{
		"api": {RateLimit: &RateLimit{Count: 2, Window: time.Minute}},
	}, nil, t)

	// First 2 calls should succeed
	action1, err1 := p.Check(context.Background(), "api.call", nil, "cli")
	if err1 != nil {
		t.Fatalf("call 1: %v", err1)
	}
	if action1 != ActionAllowed {
		t.Errorf("call 1: expected allowed, got %s", action1)
	}

	action2, err2 := p.Check(context.Background(), "api.call", nil, "cli")
	if err2 != nil {
		t.Fatalf("call 2: %v", err2)
	}
	if action2 != ActionAllowed {
		t.Errorf("call 2: expected allowed, got %s", action2)
	}

	// Third call should be rate limited (no time elapsed)
	action3, err3 := p.Check(context.Background(), "api.call", nil, "cli")
	if err3 == nil {
		t.Fatal("call 3: expected rate limit error")
	}
	if action3 != ActionRateLimited {
		t.Errorf("call 3: expected rate limited, got %s", action3)
	}
}

func TestRateLimitTokenBucketRefill(t *testing.T) {
	// Use a very short window so we can test refill without long sleeps.
	// Keep total calls under 4 to avoid triggering loop detection (warns at 4).
	// With Count=2 and Window=200ms, refill rate is 10 tokens/sec.
	p := testPolicy(map[string]*Rule{
		"api": {RateLimit: &RateLimit{Count: 2, Window: 200 * time.Millisecond}},
	}, nil, t)

	// Consume both tokens
	for i := 0; i < 2; i++ {
		_, err := p.Check(context.Background(), "api.refill_call", nil, "cli")
		if err != nil {
			t.Fatalf("call %d: %v", i+1, err)
		}
	}

	// Should be rate limited now
	action3, err := p.Check(context.Background(), "api.refill_call", nil, "cli")
	if err == nil {
		t.Fatal("expected rate limit after 2 calls")
	}
	if action3 != ActionRateLimited {
		t.Errorf("expected rate limited, got %s", action3)
	}

	// Wait for partial refill (100ms = 1 token at 10/sec rate)
	time.Sleep(150 * time.Millisecond)

	// Should be allowed again (1 token refilled)
	action, err := p.Check(context.Background(), "api.refill_call", nil, "cli")
	if err != nil {
		t.Fatalf("after refill: %v", err)
	}
	if action != ActionAllowed {
		t.Errorf("after refill: expected allowed, got %s", action)
	}
}

// --- No Rules ---

func TestNoRulesAllows(t *testing.T) {
	p := testPolicy(map[string]*Rule{}, nil, t)

	action, err := p.Check(context.Background(), "any.tool", nil, "cli")
	if err != nil {
		t.Fatal(err)
	}
	if action != ActionAllowed {
		t.Errorf("expected ActionAllowed, got %s", action)
	}
}

// --- ParseRateLimit ---

func TestParseRateLimitValid(t *testing.T) {
	tests := []struct {
		input  string
		count  int
		window time.Duration
	}{
		{"100/hour", 100, time.Hour},
		{"10/min", 10, time.Minute},
		{"5/sec", 5, time.Second},
		{"1/minute", 1, time.Minute},
		{"50/second", 50, time.Second},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			rl, err := ParseRateLimit(tt.input)
			if err != nil {
				t.Fatal(err)
			}
			if rl.Count != tt.count {
				t.Errorf("expected count %d, got %d", tt.count, rl.Count)
			}
			if rl.Window != tt.window {
				t.Errorf("expected window %s, got %s", tt.window, rl.Window)
			}
		})
	}
}

func TestParseRateLimitInvalid(t *testing.T) {
	invalid := []string{"abc/hour", "10", "10/day", "/min", ""}
	for _, s := range invalid {
		t.Run(s, func(t *testing.T) {
			_, err := ParseRateLimit(s)
			if err == nil {
				t.Errorf("expected error for %q", s)
			}
		})
	}
}

// --- LogParamsFor ---

func TestLogParamsFor(t *testing.T) {
	p := testPolicy(map[string]*Rule{
		"github": {LogParams: []string{"repo", "branch"}},
	}, nil, t)

	params := p.LogParamsFor("github.merge_pull_request")
	if len(params) != 2 {
		t.Fatalf("expected 2 log params, got %d", len(params))
	}
	if params[0] != "repo" || params[1] != "branch" {
		t.Errorf("expected [repo, branch], got %v", params)
	}
}

func TestLogParamsForNoRule(t *testing.T) {
	p := testPolicy(map[string]*Rule{}, nil, t)

	params := p.LogParamsFor("any.tool")
	if len(params) != 0 {
		t.Errorf("expected no log params, got %v", params)
	}
}

// --- splitToolName ---

func TestSplitToolNameExact(t *testing.T) {
	rules := map[string]*Rule{"stripe.charge": {}}
	cfg, sub := splitToolName("stripe.charge", rules)
	if cfg != "stripe.charge" || sub != "" {
		t.Errorf("expected exact match, got cfg=%q sub=%q", cfg, sub)
	}
}

func TestSplitToolNamePrefix(t *testing.T) {
	rules := map[string]*Rule{"github": {}}
	cfg, sub := splitToolName("github.delete_repository", rules)
	if cfg != "github" || sub != "delete_repository" {
		t.Errorf("expected prefix match, got cfg=%q sub=%q", cfg, sub)
	}
}

func TestSplitToolNameNoMatch(t *testing.T) {
	rules := map[string]*Rule{"github": {}}
	cfg, sub := splitToolName("slack.post_message", rules)
	if cfg != "slack.post_message" || sub != "" {
		t.Errorf("expected no match passthrough, got cfg=%q sub=%q", cfg, sub)
	}
}

// --- Agent-Scoped Rate Limits ---

func TestRateLimitAgentScoped(t *testing.T) {
	p := testPolicy(map[string]*Rule{
		"api": {RateLimit: &RateLimit{Count: 2, Window: time.Minute}},
	}, nil, t)

	// Agent A makes 2 calls — should succeed
	ctxA := agent.WithAgentID(context.Background(), "agent-a")
	for i := 0; i < 2; i++ {
		action, err := p.Check(ctxA, "api.call", nil, "mcp")
		if err != nil {
			t.Fatalf("agent-a call %d: %v", i+1, err)
		}
		if action != ActionAllowed {
			t.Errorf("agent-a call %d: expected allowed, got %s", i+1, action)
		}
	}

	// Agent A's 3rd call — should be rate limited
	_, err := p.Check(ctxA, "api.call", nil, "mcp")
	if err == nil {
		t.Fatal("expected agent-a to be rate limited")
	}

	// Agent B should still be able to call (independent limit)
	ctxB := agent.WithAgentID(context.Background(), "agent-b")
	action, err := p.Check(ctxB, "api.call", nil, "mcp")
	if err != nil {
		t.Fatalf("agent-b: %v", err)
	}
	if action != ActionAllowed {
		t.Errorf("agent-b: expected allowed, got %s", action)
	}
}

// --- Loop Detection Integration ---

func TestLoopDetectionIntegrationWithPolicy(t *testing.T) {
	// Even with no rules, loop detection should still work for tools that match
	p := testPolicy(map[string]*Rule{
		"test": {}, // empty rule, no deny/confirm/rate
	}, nil, t)

	params := map[string]string{"x": "1"}

	// First 3 calls: normal
	for i := 0; i < 3; i++ {
		action, err := p.Check(context.Background(), "test.echo", params, "cli")
		if err != nil {
			t.Fatalf("call %d: %v", i+1, err)
		}
		if action != ActionAllowed {
			t.Errorf("call %d: expected allowed, got %s", i+1, action)
		}
	}

	// Calls 4-8: warning (still no error)
	for i := 3; i < 8; i++ {
		action, err := p.Check(context.Background(), "test.echo", params, "cli")
		if err != nil {
			t.Fatalf("call %d: unexpected error: %v", i+1, err)
		}
		if action != ActionLoopWarning {
			t.Errorf("call %d: expected loop_warning, got %s", i+1, action)
		}
	}

	// Calls 9-11: still warning
	for i := 8; i < 11; i++ {
		_, err := p.Check(context.Background(), "test.echo", params, "cli")
		if err != nil {
			t.Fatalf("call %d: unexpected error (should still be warning): %v", i+1, err)
		}
	}

	// Call 12+: blocked
	action, err := p.Check(context.Background(), "test.echo", params, "cli")
	if err == nil {
		t.Fatal("expected loop blocked error at call 12")
	}
	if action != ActionLoopBlocked {
		t.Errorf("expected loop_blocked, got %s", action)
	}
}
