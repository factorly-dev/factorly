package shadow

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"
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
