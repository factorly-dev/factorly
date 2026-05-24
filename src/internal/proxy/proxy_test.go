// Copyright 2026 Jordan Sherer <hi@jordansherer.com>
// SPDX-License-Identifier: gpl

package proxy

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/factorly-dev/factorly/internal/agent"
	"github.com/factorly-dev/factorly/internal/logger"
	"github.com/factorly-dev/factorly/internal/provider"
	"github.com/factorly-dev/factorly/internal/registry"
	"github.com/factorly-dev/factorly/internal/shadow"
	"github.com/factorly-dev/factorly/internal/vault"
)

// stubVaultBackend is a tiny in-memory secret store used by the
// caller-param substitution tests below. Mirrors the mockBackend in
// internal/vault/resolver_test.go but lives here to keep the proxy
// tests free of vault-package internals.
type stubVaultBackend struct {
	secrets map[string]string
}

func (s *stubVaultBackend) Get(key string) (string, error) {
	v, ok := s.secrets[key]
	if !ok {
		return "", vault.ErrNotFound
	}
	return v, nil
}
func (s *stubVaultBackend) Set(k, v string) error { s.secrets[k] = v; return nil }
func (s *stubVaultBackend) Delete(k string) error { delete(s.secrets, k); return nil }
func (s *stubVaultBackend) List() ([]string, error) {
	out := make([]string, 0, len(s.secrets))
	for k := range s.secrets {
		out = append(out, k)
	}
	return out, nil
}
func (s *stubVaultBackend) Close() error { return nil }

type mockProvider struct {
	result *provider.Result
	err    error
	called bool
	params map[string]string
}

func (m *mockProvider) Setup() error    { return nil }
func (m *mockProvider) Teardown() error { return nil }
func (m *mockProvider) Execute(toolName string, params map[string]string) (*provider.Result, error) {
	m.called = true
	m.params = params
	return m.result, m.err
}

type capturingLogger struct {
	entries []*logger.Entry
}

func (l *capturingLogger) Log(entry *logger.Entry) error {
	l.entries = append(l.entries, entry)
	return nil
}
func (l *capturingLogger) Close() error { return nil }

func TestProxyExecuteSuccess(t *testing.T) {
	reg := registry.New()
	reg.Register(&registry.Tool{
		Name:        "test.echo",
		ProviderKey: "mock",
	})

	mock := &mockProvider{
		result: &provider.Result{
			Output:   "hello",
			ExitCode: 0,
			Duration: 50 * time.Millisecond,
		},
	}

	log := &capturingLogger{}
	p := New(reg, map[string]provider.Provider{"mock": mock}, log)

	result, err := p.Execute("test.echo", map[string]string{"msg": "hi"}, "cli")
	if err != nil {
		t.Fatal(err)
	}

	if !mock.called {
		t.Error("expected provider to be called")
	}
	if result.Output != "hello" {
		t.Errorf("expected output 'hello', got %q", result.Output)
	}

	// Check log
	if len(log.entries) != 1 {
		t.Fatalf("expected 1 log entry, got %d", len(log.entries))
	}
	entry := log.entries[0]
	if entry.Tool != "test.echo" {
		t.Errorf("expected log tool test.echo, got %s", entry.Tool)
	}
	if entry.Status != "success" {
		t.Errorf("expected log status success, got %s", entry.Status)
	}
	if entry.Interface != "cli" {
		t.Errorf("expected log interface cli, got %s", entry.Interface)
	}
}

func TestProxyExecuteError(t *testing.T) {
	reg := registry.New()
	reg.Register(&registry.Tool{
		Name:        "test.fail",
		ProviderKey: "mock",
	})

	mock := &mockProvider{
		result: &provider.Result{
			ExitCode: 1,
			Error:    "command failed",
			Duration: 10 * time.Millisecond,
		},
	}

	log := &capturingLogger{}
	p := New(reg, map[string]provider.Provider{"mock": mock}, log)

	result, err := p.Execute("test.fail", map[string]string{}, "cli")
	if err != nil {
		t.Fatal(err)
	}

	if !result.IsError() {
		t.Error("expected error result")
	}

	if len(log.entries) != 1 {
		t.Fatalf("expected 1 log entry, got %d", len(log.entries))
	}
	if log.entries[0].Status != "error" {
		t.Errorf("expected log status error, got %s", log.entries[0].Status)
	}
}

func TestProxyExecuteToolNotFound(t *testing.T) {
	reg := registry.New()
	p := New(reg, map[string]provider.Provider{}, logger.NopLogger{})

	_, err := p.Execute("nonexistent", map[string]string{}, "cli")
	if err == nil {
		t.Fatal("expected error for missing tool")
	}
}

func TestProxyExecuteProviderNotFound(t *testing.T) {
	reg := registry.New()
	reg.Register(&registry.Tool{
		Name:        "test.orphan",
		ProviderKey: "missing",
	})

	p := New(reg, map[string]provider.Provider{}, logger.NopLogger{})

	_, err := p.Execute("test.orphan", map[string]string{}, "cli")
	if err == nil {
		t.Fatal("expected error for missing provider")
	}
}

func TestProxyPassesParams(t *testing.T) {
	reg := registry.New()
	reg.Register(&registry.Tool{
		Name:        "test.params",
		ProviderKey: "mock",
	})

	mock := &mockProvider{
		result: &provider.Result{Duration: time.Millisecond},
	}

	p := New(reg, map[string]provider.Provider{"mock": mock}, logger.NopLogger{})

	params := map[string]string{"key": "value", "other": "data"}
	_, err := p.Execute("test.params", params, "mcp")
	if err != nil {
		t.Fatal(err)
	}

	if mock.params["key"] != "value" {
		t.Errorf("expected param key=value, got %v", mock.params)
	}
	if mock.params["other"] != "data" {
		t.Errorf("expected param other=data, got %v", mock.params)
	}
}

// TestProxyStampsWorkflowRunIDFromContext verifies the proxy reads
// provider.WorkflowRunIDKey + WorkflowNameKey from the call context
// and stamps the resulting audit entry's WorkflowRunID and
// WorkflowName fields. This is what makes /history coalescing
// possible — every step call in a workflow run inherits the same
// run ID, so the UI can group entries.
//
// Also covers the negative case: a call without the keys produces
// an entry with empty WorkflowRunID and WorkflowName. (Non-workflow
// calls must not pick up stale identifiers from an outer context.)
func TestProxyStampsWorkflowRunIDFromContext(t *testing.T) {
	reg := registry.New()
	reg.Register(&registry.Tool{Name: "test.tool", ProviderKey: "mock"})
	mock := &mockProvider{result: &provider.Result{Duration: time.Millisecond}}
	log := &capturingLogger{}
	p := New(reg, map[string]provider.Provider{"mock": mock}, log)

	// With both keys set — entry should carry both.
	ctx := context.WithValue(context.Background(), provider.WorkflowRunIDKey, "run-abc123")
	ctx = context.WithValue(ctx, provider.WorkflowNameKey, "daily-prep")
	if _, err := p.ExecuteWithContext(ctx, "test.tool", map[string]string{}, "workflow"); err != nil {
		t.Fatal(err)
	}
	if len(log.entries) != 1 {
		t.Fatalf("expected 1 log entry, got %d", len(log.entries))
	}
	if got := log.entries[0].WorkflowRunID; got != "run-abc123" {
		t.Errorf("WorkflowRunID = %q, want run-abc123", got)
	}
	if got := log.entries[0].WorkflowName; got != "daily-prep" {
		t.Errorf("WorkflowName = %q, want daily-prep", got)
	}

	// With only WorkflowRunIDKey (no name) — name should be empty,
	// run ID should still propagate. Defensive: workflow.go always
	// sets both today, but the proxy should tolerate either alone.
	log.entries = nil
	ctx = context.WithValue(context.Background(), provider.WorkflowRunIDKey, "run-xyz")
	if _, err := p.ExecuteWithContext(ctx, "test.tool", map[string]string{}, "workflow"); err != nil {
		t.Fatal(err)
	}
	if got := log.entries[0].WorkflowRunID; got != "run-xyz" {
		t.Errorf("WorkflowRunID = %q, want run-xyz", got)
	}
	if got := log.entries[0].WorkflowName; got != "" {
		t.Errorf("WorkflowName should be empty when not set, got %q", got)
	}

	// Neither key set — both fields must be empty.
	log.entries = nil
	if _, err := p.Execute("test.tool", map[string]string{}, "cli"); err != nil {
		t.Fatal(err)
	}
	if got := log.entries[0].WorkflowRunID; got != "" {
		t.Errorf("WorkflowRunID should be empty for non-workflow call, got %q", got)
	}
	if got := log.entries[0].WorkflowName; got != "" {
		t.Errorf("WorkflowName should be empty for non-workflow call, got %q", got)
	}
}

func TestProxyLoopDetectionBlocks(t *testing.T) {
	reg := registry.New()
	reg.Register(&registry.Tool{
		Name:        "test.echo",
		ProviderKey: "mock",
	})

	mock := &mockProvider{
		result: &provider.Result{Output: "ok", Duration: time.Millisecond},
	}

	rules := map[string]*shadow.Rule{
		"test": {},
	}
	path := filepath.Join(t.TempDir(), "rate.json")
	policy := shadow.New(rules, nil, path)

	log := &capturingLogger{}
	p := New(reg, map[string]provider.Provider{"mock": mock}, log, WithShadow(policy))

	params := map[string]string{"msg": "hello"}

	// First 3 calls should succeed (normal phase, LoopNormal <= 3)
	for i := 0; i < 3; i++ {
		_, err := p.Execute("test.echo", params, "cli")
		if err != nil {
			t.Fatalf("call %d: unexpected error: %v", i+1, err)
		}
	}

	// Calls 4-11 should still succeed (warning phase, LoopWarning 4-11)
	for i := 3; i < 11; i++ {
		_, err := p.Execute("test.echo", params, "cli")
		if err != nil {
			t.Fatalf("call %d: unexpected error: %v", i+1, err)
		}
	}

	// Call 12+ should be blocked (LoopBlocked >= 12)
	_, err := p.Execute("test.echo", params, "cli")
	if err == nil {
		t.Fatal("expected loop detection to block call 12")
	}
	if !strings.Contains(err.Error(), "repeated") && !strings.Contains(err.Error(), "loop") {
		t.Errorf("expected loop-related error, got: %v", err)
	}
}

func TestProxyOutputTruncation(t *testing.T) {
	reg := registry.New()
	reg.Register(&registry.Tool{
		Name:        "test.big",
		ProviderKey: "mock",
		MaxOutput:   100,
	})

	bigOutput := strings.Repeat("x", 500)
	mock := &mockProvider{
		result: &provider.Result{Output: bigOutput, Duration: time.Millisecond},
	}

	p := New(reg, map[string]provider.Provider{"mock": mock}, logger.NopLogger{})

	result, err := p.Execute("test.big", nil, "cli")
	if err != nil {
		t.Fatal(err)
	}

	if len(result.Output) > 150 { // some slack for the truncation marker
		t.Errorf("expected truncated output, got %d bytes", len(result.Output))
	}
	if !strings.Contains(result.Output, "truncated") {
		t.Error("expected truncation marker in output")
	}
}

func TestProxyOutputCompression(t *testing.T) {
	reg := registry.New()
	reg.Register(&registry.Tool{
		Name:        "test.api",
		ProviderKey: "mock",
		Compress:    []string{"json"},
	})

	prettyJSON := "{\n  \"name\": \"test\",\n  \"value\": 42\n}"
	mock := &mockProvider{
		result: &provider.Result{Output: prettyJSON, Duration: time.Millisecond},
	}

	p := New(reg, map[string]provider.Provider{"mock": mock}, logger.NopLogger{})

	result, err := p.Execute("test.api", nil, "cli")
	if err != nil {
		t.Fatal(err)
	}

	if strings.Contains(result.Output, "\n  ") {
		t.Error("expected JSON to be compacted, still has indentation")
	}
	if !strings.Contains(result.Output, `"name":"test"`) {
		t.Error("expected compacted JSON content")
	}
}

func TestProxyAgentIDInLog(t *testing.T) {
	reg := registry.New()
	reg.Register(&registry.Tool{
		Name:        "test.echo",
		ProviderKey: "mock",
	})

	mock := &mockProvider{
		result: &provider.Result{Output: "ok", Duration: time.Millisecond},
	}

	log := &capturingLogger{}
	p := New(reg, map[string]provider.Provider{"mock": mock}, log)

	ctx := agent.WithAgentID(context.Background(), "session-abc-123")
	_, err := p.ExecuteWithContext(ctx, "test.echo", map[string]string{"msg": "hi"}, "mcp")
	if err != nil {
		t.Fatal(err)
	}

	if len(log.entries) != 1 {
		t.Fatalf("expected 1 log entry, got %d", len(log.entries))
	}
	if log.entries[0].AgentID != "session-abc-123" {
		t.Errorf("expected agent ID 'session-abc-123', got %q", log.entries[0].AgentID)
	}
}

func TestProxyNoAgentIDWhenNotSet(t *testing.T) {
	reg := registry.New()
	reg.Register(&registry.Tool{
		Name:        "test.echo",
		ProviderKey: "mock",
	})

	mock := &mockProvider{
		result: &provider.Result{Output: "ok", Duration: time.Millisecond},
	}

	log := &capturingLogger{}
	p := New(reg, map[string]provider.Provider{"mock": mock}, log)

	_, err := p.Execute("test.echo", map[string]string{"msg": "hi"}, "cli")
	if err != nil {
		t.Fatal(err)
	}

	if len(log.entries) != 1 {
		t.Fatalf("expected 1 log entry, got %d", len(log.entries))
	}
	if log.entries[0].AgentID != "" {
		t.Errorf("expected empty agent ID, got %q", log.entries[0].AgentID)
	}
}

func TestProxySavingsTracking(t *testing.T) {
	reg := registry.New()
	reg.Register(&registry.Tool{
		Name:        "test.big",
		ProviderKey: "mock",
		MaxOutput:   100,
	})

	bigOutput := strings.Repeat("x", 500)
	mock := &mockProvider{
		result: &provider.Result{Output: bigOutput, Duration: time.Millisecond},
	}

	log := &capturingLogger{}
	p := New(reg, map[string]provider.Provider{"mock": mock}, log)

	_, err := p.Execute("test.big", nil, "cli")
	if err != nil {
		t.Fatal(err)
	}

	if len(log.entries) != 1 {
		t.Fatalf("expected 1 log entry, got %d", len(log.entries))
	}
	entry := log.entries[0]
	if entry.OriginalBytes != 500 {
		t.Errorf("expected original_bytes=500, got %d", entry.OriginalBytes)
	}
	if entry.ProcessedBytes >= entry.OriginalBytes {
		t.Errorf("expected processed_bytes < original_bytes, got %d >= %d", entry.ProcessedBytes, entry.OriginalBytes)
	}
	if entry.ProcessedBytes == 0 {
		t.Error("expected non-zero processed_bytes")
	}
}

func TestProxyNoSavingsWhenNoProcessing(t *testing.T) {
	reg := registry.New()
	reg.Register(&registry.Tool{
		Name:        "test.plain",
		ProviderKey: "mock",
	})

	mock := &mockProvider{
		result: &provider.Result{Output: "hello", Duration: time.Millisecond},
	}

	log := &capturingLogger{}
	p := New(reg, map[string]provider.Provider{"mock": mock}, log)

	_, err := p.Execute("test.plain", nil, "cli")
	if err != nil {
		t.Fatal(err)
	}

	if len(log.entries) != 1 {
		t.Fatalf("expected 1 log entry, got %d", len(log.entries))
	}
	entry := log.entries[0]
	if entry.OriginalBytes != 0 {
		t.Errorf("expected original_bytes=0 (omitted), got %d", entry.OriginalBytes)
	}
	if entry.ProcessedBytes != 0 {
		t.Errorf("expected processed_bytes=0 (omitted), got %d", entry.ProcessedBytes)
	}
}

// --- Caller-param vault hydration safety ---
//
// These tests pin the security guarantee that caller-supplied param
// values can't accidentally trigger vault substitution unless the
// tool's ParamConfig explicitly opts in via HydrateVaultRefs: true.
// See internal/vault/resolver.go IsSafeBackendName for the allowlist.

func TestProxyResolverSkipsVaultBackendForCallerParamsByDefault(t *testing.T) {
	// Without HydrateVaultRefs, a caller-supplied "{{vault:K}}" must
	// flow through to the provider unchanged AND appear in the audit
	// log unchanged. This is the security guarantee that closes the
	// original leak (Trello card body received resolved secret).
	reg := registry.New()
	reg.Register(&registry.Tool{
		Name:        "test.echo",
		ProviderKey: "mock",
		Parameters: []registry.Parameter{
			{Name: "text"}, // HydrateVaultRefs: false (default)
		},
	})
	mock := &mockProvider{result: &provider.Result{Output: "ok", Duration: time.Millisecond}}
	log := &capturingLogger{}
	resolver := vault.NewResolver()
	resolver.Register("vault", &stubVaultBackend{secrets: map[string]string{"SECRET": "real-secret-value"}})
	p := New(reg, map[string]provider.Provider{"mock": mock}, log, WithResolver(resolver))

	_, err := p.Execute("test.echo", map[string]string{"text": "hello {{vault:SECRET}} world"}, "cli")
	if err != nil {
		t.Fatal(err)
	}

	// Provider must have received the LITERAL template, not the resolved secret.
	if mock.params["text"] != "hello {{vault:SECRET}} world" {
		t.Errorf("provider received resolved secret %q; expected literal template", mock.params["text"])
	}
	// Audit log must also show the literal template.
	if len(log.entries) != 1 {
		t.Fatalf("expected 1 log entry, got %d", len(log.entries))
	}
	if got := log.entries[0].Params["text"]; got != "hello {{vault:SECRET}} world" {
		t.Errorf("audit log leaked resolved secret: %q", got)
	}
	if strings.Contains(log.entries[0].Params["text"], "real-secret-value") {
		t.Error("audit log contains the real secret value — this is the leak we're fixing")
	}
}

func TestProxyResolverHydratesVaultBackendWhenParamOptedIn(t *testing.T) {
	// With HydrateVaultRefs: true on the param, the substitution
	// happens AND the audit log is redacted back to the template.
	reg := registry.New()
	reg.Register(&registry.Tool{
		Name:        "test.signed",
		ProviderKey: "mock",
		Parameters: []registry.Parameter{
			{Name: "signing_key", HydrateVaultRefs: true},
		},
	})
	mock := &mockProvider{result: &provider.Result{Output: "ok", Duration: time.Millisecond}}
	log := &capturingLogger{}
	resolver := vault.NewResolver()
	resolver.Register("vault", &stubVaultBackend{secrets: map[string]string{"HMAC_KEY": "abc123secret"}})
	p := New(reg, map[string]provider.Provider{"mock": mock}, log, WithResolver(resolver))

	_, err := p.Execute("test.signed", map[string]string{"signing_key": "Bearer {{vault:HMAC_KEY}}"}, "cli")
	if err != nil {
		t.Fatal(err)
	}

	// Provider DID get the resolved value.
	if mock.params["signing_key"] != "Bearer abc123secret" {
		t.Errorf("provider should have received resolved value, got %q", mock.params["signing_key"])
	}
	// Audit log shows the original template, NOT the resolved value.
	if len(log.entries) != 1 {
		t.Fatalf("expected 1 log entry, got %d", len(log.entries))
	}
	logged := log.entries[0].Params["signing_key"]
	if logged != "Bearer {{vault:HMAC_KEY}}" {
		t.Errorf("audit log should show template; got %q", logged)
	}
	if strings.Contains(logged, "abc123secret") {
		t.Error("audit log still contains plaintext secret after redaction")
	}
}

func TestProxyResolverHydratesSafeBackendsOnCallerParams(t *testing.T) {
	// env / store / expr resolve on caller params regardless of
	// HydrateVaultRefs — they can't leak anything the caller didn't
	// already have. Resolved values appear plaintext in the audit
	// log (no redaction); these aren't secrets.
	reg := registry.New()
	reg.Register(&registry.Tool{
		Name:        "test.cached",
		ProviderKey: "mock",
		Parameters:  []registry.Parameter{{Name: "msg"}},
	})
	mock := &mockProvider{result: &provider.Result{Output: "ok", Duration: time.Millisecond}}
	log := &capturingLogger{}
	resolver := vault.NewResolver()
	resolver.Register("store", &stubVaultBackend{secrets: map[string]string{"LAST_ID": "id-42"}})
	resolver.Register("env", &stubVaultBackend{secrets: map[string]string{"USER": "alice"}})
	p := New(reg, map[string]provider.Provider{"mock": mock}, log, WithResolver(resolver))

	_, err := p.Execute("test.cached", map[string]string{"msg": "u={{env:USER}} id={{store:LAST_ID}}"}, "cli")
	if err != nil {
		t.Fatal(err)
	}

	// Both safe-backend refs resolved.
	want := "u=alice id=id-42"
	if mock.params["msg"] != want {
		t.Errorf("provider got %q, want %q", mock.params["msg"], want)
	}
	// Audit log shows the resolved value (no redaction for safe backends).
	if log.entries[0].Params["msg"] != want {
		t.Errorf("audit log got %q, want %q", log.entries[0].Params["msg"], want)
	}
}

func TestProxyResolverMixedSafeAndSecretRefsInOneParam(t *testing.T) {
	// One param contains both a safe ref and a secret ref. The safe
	// one resolves; the secret one flows through. Audit log shows
	// the safe resolution AND the literal secret template.
	reg := registry.New()
	reg.Register(&registry.Tool{
		Name:        "test.mixed",
		ProviderKey: "mock",
		Parameters:  []registry.Parameter{{Name: "msg"}},
	})
	mock := &mockProvider{result: &provider.Result{Output: "ok", Duration: time.Millisecond}}
	log := &capturingLogger{}
	resolver := vault.NewResolver()
	resolver.Register("env", &stubVaultBackend{secrets: map[string]string{"USER": "alice"}})
	resolver.Register("vault", &stubVaultBackend{secrets: map[string]string{"PASS": "hunter2"}})
	p := New(reg, map[string]provider.Provider{"mock": mock}, log, WithResolver(resolver))

	_, err := p.Execute("test.mixed", map[string]string{"msg": "user={{env:USER}} pass={{vault:PASS}}"}, "cli")
	if err != nil {
		t.Fatal(err)
	}

	want := "user=alice pass={{vault:PASS}}"
	if mock.params["msg"] != want {
		t.Errorf("provider got %q, want %q (env resolves, vault doesn't)", mock.params["msg"], want)
	}
	if strings.Contains(mock.params["msg"], "hunter2") {
		t.Error("provider received the vault secret despite no opt-in")
	}
	if log.entries[0].Params["msg"] != want {
		t.Errorf("audit log got %q, want %q", log.entries[0].Params["msg"], want)
	}
}

func TestProxyResolverUndeclaredParamDefaultsToDeny(t *testing.T) {
	// If a caller passes a param that's not declared in the tool's
	// Parameters list (some tool types accept free-form params), the
	// per-param opt-in lookup returns false. Secret backends stay
	// blocked. This matters specifically for MCP-discovered tools
	// where every discovered param defaults to no opt-in.
	reg := registry.New()
	reg.Register(&registry.Tool{
		Name:        "test.freeform",
		ProviderKey: "mock",
		// No Parameters declared.
	})
	mock := &mockProvider{result: &provider.Result{Output: "ok", Duration: time.Millisecond}}
	log := &capturingLogger{}
	resolver := vault.NewResolver()
	resolver.Register("vault", &stubVaultBackend{secrets: map[string]string{"K": "v"}})
	p := New(reg, map[string]provider.Provider{"mock": mock}, log, WithResolver(resolver))

	_, err := p.Execute("test.freeform", map[string]string{"undeclared": "x={{vault:K}}"}, "cli")
	if err != nil {
		t.Fatal(err)
	}

	if mock.params["undeclared"] != "x={{vault:K}}" {
		t.Errorf("undeclared params must default-deny secret backends; got %q", mock.params["undeclared"])
	}
}

func TestProxyResolverRedactsConfigSideDefaultsThatHitVault(t *testing.T) {
	// Tool config has a param with Default = "{{vault:K}}" AND
	// HydrateVaultRefs: true (so the resolver will substitute even
	// when the caller doesn't supply the param). The substitution
	// happens, the provider gets the secret, but the audit log
	// shows the template.
	reg := registry.New()
	reg.Register(&registry.Tool{
		Name:        "test.defaulted",
		ProviderKey: "mock",
		Parameters: []registry.Parameter{
			{Name: "auth_token", Default: "Bearer {{vault:API_KEY}}", HydrateVaultRefs: true},
		},
	})
	mock := &mockProvider{result: &provider.Result{Output: "ok", Duration: time.Millisecond}}
	log := &capturingLogger{}
	resolver := vault.NewResolver()
	resolver.Register("vault", &stubVaultBackend{secrets: map[string]string{"API_KEY": "sk-live-1234"}})
	p := New(reg, map[string]provider.Provider{"mock": mock}, log, WithResolver(resolver))

	// Caller doesn't supply auth_token; the default fires AND resolves.
	_, err := p.Execute("test.defaulted", map[string]string{}, "cli")
	if err != nil {
		t.Fatal(err)
	}

	if mock.params["auth_token"] != "Bearer sk-live-1234" {
		t.Errorf("provider should have received resolved default, got %q", mock.params["auth_token"])
	}
	if log.entries[0].Params["auth_token"] != "Bearer {{vault:API_KEY}}" {
		t.Errorf("audit log should show template after redaction, got %q", log.entries[0].Params["auth_token"])
	}
}
