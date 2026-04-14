package shadow

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/factorly-dev/factorly/internal/agent"
)

// Action describes what the shadow layer decided.
type Action string

const (
	ActionAllowed     Action = "allowed"
	ActionDenied      Action = "denied"
	ActionConfirmed   Action = "confirmed"
	ActionRateLimited Action = "rate_limited"
	ActionLoopWarning Action = "loop_warning"
	ActionLoopBlocked Action = "loop_blocked"
)

// ConfirmFunc prompts the user for confirmation. Returns true if approved.
// CLI: interactive stdin prompt. MCP: elicitation request to the client.
type ConfirmFunc func(ctx context.Context, toolName string, params map[string]string) bool

// Rule holds the parsed shadow config for one tool config entry.
type Rule struct {
	Deny       []string   // sub-tool names to block
	Confirm    []string   // sub-tool names requiring confirmation
	ConfirmAll bool       // true if confirm: true (all calls)
	RateLimit  *RateLimit // parsed rate limit, nil if none
	LogParams  []string   // param keys to highlight in logs
}

// RateLimit is a parsed "100/hour" spec.
type RateLimit struct {
	Count  int
	Window time.Duration
}

// Policy is the central shadow enforcement engine.
type Policy struct {
	rules        map[string]*Rule
	rateStore    *RateStore
	confirmFn    ConfirmFunc
	loopDetector *LoopDetector
}

// New creates a shadow policy.
func New(rules map[string]*Rule, confirmFn ConfirmFunc, rateStorePath string) *Policy {
	return &Policy{
		rules:        rules,
		rateStore:    NewRateStore(rateStorePath),
		confirmFn:    confirmFn,
		loopDetector: NewLoopDetector(0),
	}
}

// Check evaluates shadow rules for a tool call. Returns the action taken
// and an error if the call should be blocked.
func (p *Policy) Check(ctx context.Context, toolName string, params map[string]string, iface string) (Action, error) {
	configName, subTool := splitToolName(toolName, p.rules)

	rule, ok := p.rules[configName]
	if !ok {
		// No rules — still check loop detection
		return p.checkLoop(toolName, params, ActionAllowed)
	}

	// 1. Deny check
	if isDenied(rule, toolName, subTool) {
		return ActionDenied, fmt.Errorf("tool %q is denied by shadow policy", toolName)
	}

	// 2. Confirm check
	action := ActionAllowed
	if needsConfirm(rule, toolName, subTool) {
		if p.confirmFn == nil {
			return ActionDenied, fmt.Errorf("tool %q requires confirmation but no confirm handler available", toolName)
		}
		if !p.confirmFn(ctx, toolName, params) {
			return ActionDenied, fmt.Errorf("tool %q: confirmation declined", toolName)
		}
		action = ActionConfirmed
	}

	// 3. Rate limit check
	agentID := agent.AgentID(ctx)
	if err := p.checkRateLimit(toolName, rule, agentID); err != nil {
		return ActionRateLimited, err
	}

	// 4. Loop detection (always active)
	return p.checkLoop(toolName, params, action)
}

// checkLoop applies loop detection and returns the appropriate action.
func (p *Policy) checkLoop(toolName string, params map[string]string, currentAction Action) (Action, error) {
	if p.loopDetector == nil {
		return currentAction, nil
	}
	switch p.loopDetector.Check(toolName, params) {
	case LoopBlocked:
		return ActionLoopBlocked, fmt.Errorf("tool %q blocked: identical call repeated too many times (possible agent loop)", toolName)
	case LoopWarning:
		return ActionLoopWarning, nil
	default:
		return currentAction, nil
	}
}

// IsDenied returns true if the tool is blocked by a deny rule.
func (p *Policy) IsDenied(toolName string) bool {
	configName, subTool := splitToolName(toolName, p.rules)
	rule, ok := p.rules[configName]
	if !ok {
		return false
	}
	return isDenied(rule, toolName, subTool)
}

// LogParamsFor returns the log_params list for a tool name.
func (p *Policy) LogParamsFor(toolName string) []string {
	configName, _ := splitToolName(toolName, p.rules)
	if rule, ok := p.rules[configName]; ok {
		return rule.LogParams
	}
	return nil
}

func (p *Policy) checkRateLimit(toolName string, rule *Rule, agentID string) error {
	if rule.RateLimit == nil {
		return nil
	}
	key := toolName
	if agentID != "" {
		key = agentID + ":" + toolName
	}
	allowed, remaining, err := p.rateStore.Check(key, rule.RateLimit.Count, rule.RateLimit.Window)
	if err != nil {
		// Rate store error — allow the call but warn
		return nil
	}
	if !allowed {
		return fmt.Errorf("tool %q rate limited: %d/%s exceeded (resets in %s)",
			toolName, rule.RateLimit.Count, rule.RateLimit.Window, remaining.Truncate(time.Second))
	}
	return nil
}

// splitToolName resolves the config entry that owns a tool.
// Exact match first (per-tool shadow), then prefix match (MCP server-level).
func splitToolName(toolName string, rules map[string]*Rule) (configName, subTool string) {
	if _, ok := rules[toolName]; ok {
		return toolName, ""
	}
	if idx := strings.IndexByte(toolName, '.'); idx > 0 {
		prefix := toolName[:idx]
		if _, ok := rules[prefix]; ok {
			return prefix, toolName[idx+1:]
		}
	}
	return toolName, ""
}

func isDenied(rule *Rule, toolName, subTool string) bool {
	target := subTool
	if target == "" {
		target = toolName
	}
	for _, d := range rule.Deny {
		if d == target || d == toolName {
			return true
		}
	}
	return false
}

func needsConfirm(rule *Rule, toolName, subTool string) bool {
	if rule.ConfirmAll {
		return true
	}
	target := subTool
	if target == "" {
		target = toolName
	}
	for _, c := range rule.Confirm {
		if c == target || c == toolName {
			return true
		}
	}
	return false
}

// ParseRateLimit parses strings like "100/hour", "10/min", "5/sec".
func ParseRateLimit(s string) (*RateLimit, error) {
	parts := strings.SplitN(s, "/", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid rate_limit format %q (expected count/unit)", s)
	}
	count, err := strconv.Atoi(parts[0])
	if err != nil {
		return nil, fmt.Errorf("invalid rate_limit count %q: %w", parts[0], err)
	}
	var window time.Duration
	switch parts[1] {
	case "sec", "second":
		window = time.Second
	case "min", "minute":
		window = time.Minute
	case "hour":
		window = time.Hour
	default:
		return nil, fmt.Errorf("unknown rate_limit unit %q", parts[1])
	}
	return &RateLimit{Count: count, Window: window}, nil
}
