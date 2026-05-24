// Copyright 2026 Jordan Sherer <hi@jordansherer.com>
// SPDX-License-Identifier: gpl

package vault

import (
	"fmt"
	"strings"
	"testing"
)

// mockBackend implements Backend for testing.
type mockBackend struct {
	secrets map[string]string
}

func (m *mockBackend) Get(key string) (string, error) {
	v, ok := m.secrets[key]
	if !ok {
		return "", ErrNotFound
	}
	return v, nil
}
func (m *mockBackend) Set(key, value string) error { m.secrets[key] = value; return nil }
func (m *mockBackend) Delete(key string) error     { delete(m.secrets, key); return nil }
func (m *mockBackend) List() ([]string, error)     { return nil, nil }
func (m *mockBackend) Close() error                { return nil }

func TestResolveVaultRef(t *testing.T) {
	r := NewResolver()
	r.Register("vault", &mockBackend{secrets: map[string]string{"TOKEN": "secret123"}})

	result, err := r.Resolve("Bearer {{vault:TOKEN}}")
	if err != nil {
		t.Fatal(err)
	}
	if result != "Bearer secret123" {
		t.Errorf("expected 'Bearer secret123', got %q", result)
	}
}

func TestResolveMultipleRefs(t *testing.T) {
	r := NewResolver()
	r.Register("vault", &mockBackend{secrets: map[string]string{
		"USER": "admin",
		"PASS": "s3cret",
	}})

	result, err := r.Resolve("{{vault:USER}}:{{vault:PASS}}")
	if err != nil {
		t.Fatal(err)
	}
	if result != "admin:s3cret" {
		t.Errorf("expected 'admin:s3cret', got %q", result)
	}
}

func TestResolveNoRefs(t *testing.T) {
	r := NewResolver()

	result, err := r.Resolve("plain string with no refs")
	if err != nil {
		t.Fatal(err)
	}
	if result != "plain string with no refs" {
		t.Errorf("expected unchanged string, got %q", result)
	}
}

func TestResolveEnvVarPassesThrough(t *testing.T) {
	r := NewResolver()

	// {{ENV_VAR}} (no colon) should not be matched by the resolver
	result, err := r.Resolve("{{ENV_VAR}}")
	if err != nil {
		t.Fatal(err)
	}
	if result != "{{ENV_VAR}}" {
		t.Errorf("expected '{{ENV_VAR}}' unchanged, got %q", result)
	}
}

func TestResolveUnknownBackend(t *testing.T) {
	r := NewResolver()

	result, err := r.Resolve("${unknown:KEY}")
	if err != nil {
		t.Fatal(err)
	}
	if result != "${unknown:KEY}" {
		t.Errorf("expected unresolved ref, got %q", result)
	}
}

func TestResolveMissingKey(t *testing.T) {
	r := NewResolver()
	r.Register("vault", &mockBackend{secrets: map[string]string{}})

	_, err := r.Resolve("{{vault:MISSING}}")
	if err == nil {
		t.Fatal("expected error for missing key")
	}
}

func TestResolveMissingKeyErrorDoesNotLeakKeyName(t *testing.T) {
	r := NewResolver()
	r.Register("vault", &mockBackend{secrets: map[string]string{}})

	_, err := r.Resolve("{{vault:SECRET_API_KEY}}")
	if err == nil {
		t.Fatal("expected error for missing key")
	}
	// Error should NOT contain the key name (security: don't leak key names)
	errMsg := err.Error()
	if strings.Contains(errMsg, "SECRET_API_KEY") {
		t.Errorf("error message should not contain key name, got: %s", errMsg)
	}
	// But should mention the backend
	if !strings.Contains(errMsg, "vault") {
		t.Errorf("error message should mention the backend, got: %s", errMsg)
	}
}

func TestResolveMultipleBackends(t *testing.T) {
	r := NewResolver()
	r.Register("vault", &mockBackend{secrets: map[string]string{"A": "from-vault"}})
	r.Register("other", &mockBackend{secrets: map[string]string{"B": "from-other"}})

	result, err := r.Resolve("{{vault:A}} and {{other:B}}")
	if err != nil {
		t.Fatal(err)
	}
	if result != "from-vault and from-other" {
		t.Errorf("expected 'from-vault and from-other', got %q", result)
	}
}

func TestResolvePathStyle(t *testing.T) {
	r := NewResolver()
	r.Register("1password", &mockBackend{secrets: map[string]string{
		"Development/GitHub/token": "ghp_xxx",
	}})

	result, err := r.Resolve("{{1password:Development/GitHub/token}}")
	if err != nil {
		t.Fatal(err)
	}
	if result != "ghp_xxx" {
		t.Errorf("expected 'ghp_xxx', got %q", result)
	}
}

func TestHasVaultRefs(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"{{vault:TOKEN}}", true},
		{"{{1password:vault/item}}", true},
		{"prefix {{vault:KEY}} suffix", true},
		{"{{ENV_VAR}}", false},
		{"no refs here", false},
		{"", false},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := HasVaultRefs(tt.input); got != tt.want {
				t.Errorf("HasVaultRefs(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestResolveWithDefault(t *testing.T) {
	r := NewResolver()
	r.Register("vault", &mapBackend{data: map[string]string{"EXISTS": "real-value"}})

	// Key exists — use real value, ignore default
	got, err := r.Resolve("{{vault:EXISTS|fallback}}")
	if err != nil {
		t.Fatal(err)
	}
	if got != "real-value" {
		t.Errorf("expected real-value, got %q", got)
	}

	// Key missing — use default
	got, err = r.Resolve("{{vault:MISSING|my-default}}")
	if err != nil {
		t.Errorf("expected no error with default, got %v", err)
	}
	if got != "my-default" {
		t.Errorf("expected my-default, got %q", got)
	}

	// Backend missing — use default
	got, err = r.Resolve("{{unknown:KEY|fallback}}")
	if err != nil {
		t.Errorf("expected no error with default, got %v", err)
	}
	if got != "fallback" {
		t.Errorf("expected fallback, got %q", got)
	}

	// No default, key missing — error
	_, err = r.Resolve("{{vault:MISSING}}")
	if err == nil {
		t.Error("expected error for missing key without default")
	}
}

func TestResolveDefaultInContext(t *testing.T) {
	r := NewResolver()
	r.Register("vault", &mapBackend{data: map[string]string{}})

	// Default in middle of string
	got, err := r.Resolve("prefix-{{vault:KEY|default}}-suffix")
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if got != "prefix-default-suffix" {
		t.Errorf("expected prefix-default-suffix, got %q", got)
	}

	// Multiple refs with defaults
	got, err = r.Resolve("{{vault:A|alpha}} and {{vault:B|beta}}")
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if got != "alpha and beta" {
		t.Errorf("expected 'alpha and beta', got %q", got)
	}
}

func TestResolveEscaped(t *testing.T) {
	r := NewResolver()
	r.Register("vault", &mapBackend{data: map[string]string{"TOKEN": "secret"}})

	// Escaped reference passes through as literal {{vault:TOKEN}}
	got, err := r.Resolve(`\{{vault:TOKEN}}`)
	if err != nil {
		t.Fatal(err)
	}
	if got != "{{vault:TOKEN}}" {
		t.Errorf("expected literal {{vault:TOKEN}}, got %q", got)
	}

	// Mixed: one escaped, one resolved
	got, err = r.Resolve(`\{{literal}} and {{vault:TOKEN}}`)
	if err != nil {
		t.Fatal(err)
	}
	if got != "{{literal}} and secret" {
		t.Errorf("expected mixed result, got %q", got)
	}

	// No escape — normal resolution
	got, err = r.Resolve("{{vault:TOKEN}}")
	if err != nil {
		t.Fatal(err)
	}
	if got != "secret" {
		t.Errorf("expected secret, got %q", got)
	}
}

func TestHasVaultRefsWithDefault(t *testing.T) {
	if !HasVaultRefs("{{vault:TOKEN|default}}") {
		t.Error("expected HasVaultRefs to detect ref with default")
	}
}

func TestRedact(t *testing.T) {
	r := NewResolver()
	r.Register("vault", &mockBackend{secrets: map[string]string{
		"TOKEN":  "sk_live_secret_99999",
		"DB_URL": "postgres://admin:pass@db:5432",
	}})

	tests := []struct {
		name     string
		input    string
		refs     []string
		expected string
	}{
		{
			name:     "redacts bearer token in header",
			input:    "Authorization: Bearer sk_live_secret_99999",
			refs:     []string{"{{vault:TOKEN}}"},
			expected: "Authorization: Bearer ****",
		},
		{
			name:     "redacts inline vault ref in URL path",
			input:    "https://api.telegram.org/botsk_live_secret_99999/getMe",
			refs:     []string{"/bot{{vault:TOKEN}}/getMe"},
			expected: "https://api.telegram.org/bot****/getMe",
		},
		{
			name:     "redacts multiple secrets",
			input:    "token=sk_live_secret_99999 db=postgres://admin:pass@db:5432",
			refs:     []string{"{{vault:TOKEN}}", "{{vault:DB_URL}}"},
			expected: "token=**** db=****",
		},
		{
			name:     "no refs, no redaction",
			input:    "GET /public/endpoint",
			refs:     nil,
			expected: "GET /public/endpoint",
		},
		{
			name:     "unresolvable ref leaves string unchanged",
			input:    "key=plaintext",
			refs:     []string{"{{vault:NONEXISTENT}}"},
			expected: "key=plaintext",
		},
		{
			name:     "ref that resolves to same value (no vault match) unchanged",
			input:    "value={{vault:MISSING}}",
			refs:     []string{"{{vault:MISSING}}"},
			expected: "value={{vault:MISSING}}",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := r.Redact(tt.input, tt.refs)
			if result != tt.expected {
				t.Errorf("Redact(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestRedact_NilResolver(t *testing.T) {
	var r *Resolver
	result := r.Redact("secret data", []string{"{{vault:KEY}}"})
	if result != "secret data" {
		t.Errorf("nil resolver should return input unchanged, got %q", result)
	}
}

// mapBackend is a simple in-memory backend for testing.
type mapBackend struct {
	data map[string]string
}

func (m *mapBackend) Get(key string) (string, error) {
	v, ok := m.data[key]
	if !ok {
		return "", ErrNotFound
	}
	return v, nil
}

func (m *mapBackend) Set(key, value string) error { m.data[key] = value; return nil }
func (m *mapBackend) Delete(key string) error     { delete(m.data, key); return nil }

func (m *mapBackend) List() ([]string, error) {
	keys := make([]string, 0, len(m.data))
	for k := range m.data {
		keys = append(keys, k)
	}
	return keys, nil
}

func (m *mapBackend) Close() error { return nil }

// --- ResolveFunc tests ---

func TestResolveFuncExpr(t *testing.T) {
	r := NewResolver()
	r.RegisterFunc("expr", func(content string) (string, error) {
		// Simulate a simple expression evaluator
		if content == "now()" {
			return "2026-05-09T00:00:00Z", nil
		}
		return content, nil
	})

	result, err := r.Resolve("time is {{expr:now()}}")
	if err != nil {
		t.Fatal(err)
	}
	if result != "time is 2026-05-09T00:00:00Z" {
		t.Errorf("expected resolved expr, got %q", result)
	}
}

func TestResolveFuncWithVaultBackend(t *testing.T) {
	r := NewResolver()
	r.Register("vault", &mockBackend{secrets: map[string]string{"TOKEN": "secret123"}})
	r.RegisterFunc("expr", func(content string) (string, error) {
		return "evaluated:" + content, nil
	})

	// Both should resolve in one pass
	result, err := r.Resolve("key={{vault:TOKEN}} expr={{expr:hello}}")
	if err != nil {
		t.Fatal(err)
	}
	if result != "key=secret123 expr=evaluated:hello" {
		t.Errorf("expected both resolved, got %q", result)
	}
}

func TestResolveFuncOverridesBackend(t *testing.T) {
	r := NewResolver()
	r.Register("test", &mockBackend{secrets: map[string]string{"key": "from_backend"}})
	r.RegisterFunc("test", func(content string) (string, error) {
		return "from_func", nil
	})

	// Func takes priority over backend
	result, err := r.Resolve("{{test:key}}")
	if err != nil {
		t.Fatal(err)
	}
	if result != "from_func" {
		t.Errorf("expected func to take priority, got %q", result)
	}
}

func TestResolveFuncNotTracked(t *testing.T) {
	r := NewResolver()
	r.RegisterFunc("expr", func(content string) (string, error) {
		return "resolved", nil
	})

	_, accessed, err := r.ResolveTracked("{{expr:something}}")
	if err != nil {
		t.Fatal(err)
	}
	if len(accessed) != 0 {
		t.Errorf("expr funcs should not be tracked, got %v", accessed)
	}
}

func TestResolveFuncError(t *testing.T) {
	r := NewResolver()
	r.RegisterFunc("expr", func(content string) (string, error) {
		return "", fmt.Errorf("eval error")
	})

	// With no default, should leave pattern unchanged
	result, _ := r.Resolve("{{expr:bad}}")
	if result != "{{expr:bad}}" {
		t.Errorf("expected unchanged on error, got %q", result)
	}
}

func TestResolveFuncWithDefault(t *testing.T) {
	r := NewResolver()
	r.RegisterFunc("expr", func(content string) (string, error) {
		return "", fmt.Errorf("eval error")
	})

	result, _ := r.Resolve("{{expr:bad|fallback}}")
	if result != "fallback" {
		t.Errorf("expected fallback on error, got %q", result)
	}
}

func TestResolveFuncComplexExpr(t *testing.T) {
	r := NewResolver()
	r.RegisterFunc("expr", func(content string) (string, error) {
		// Verify the full expression content is passed through
		return "got:" + content, nil
	})

	result, _ := r.Resolve("{{expr:jsonpath(data, '$.items[0].name')}}")
	if result != "got:jsonpath(data, '$.items[0].name')" {
		t.Errorf("expected full expression passed, got %q", result)
	}
}

// --- IsSafeBackendName / IsSecretBackendName ---

func TestIsSafeBackendName(t *testing.T) {
	cases := []struct {
		name string
		safe bool
	}{
		{"env", true},
		{"store", true},
		{"expr", true},
		{"vault", false},
		{"op", false},
		{"aws-sm", false},
		{"1password", false},
		{"future-backend-we-havent-imagined", false}, // default-deny is the safe behavior
		{"", false},
	}
	for _, c := range cases {
		if got := IsSafeBackendName(c.name); got != c.safe {
			t.Errorf("IsSafeBackendName(%q) = %v, want %v", c.name, got, c.safe)
		}
		if got := IsSecretBackendName(c.name); got == c.safe {
			t.Errorf("IsSecretBackendName(%q) should be inverse of IsSafeBackendName; got %v vs %v", c.name, got, c.safe)
		}
	}
}

// --- ResolveCallerParam ---

func TestResolveCallerParam_SkipsSecretBackendByDefault(t *testing.T) {
	// Without opt-in, a caller-supplied {{vault:K}} must NOT resolve.
	// This is the security guarantee that closes the original leak.
	r := NewResolver()
	r.Register("vault", &mockBackend{secrets: map[string]string{"TOKEN": "real-secret-value"}})

	got, refs, err := r.ResolveCallerParam("hello {{vault:TOKEN}} world", false)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got != "hello {{vault:TOKEN}} world" {
		t.Errorf("expected vault template to flow through unchanged, got %q", got)
	}
	if len(refs) != 0 {
		t.Errorf("expected no secret refs recorded when gated, got %v", refs)
	}
}

func TestResolveCallerParam_AllowsSecretBackendWhenOptedIn(t *testing.T) {
	// With opt-in, {{vault:K}} resolves AND is reported in secretRefs
	// so the caller can redact from the audit log.
	r := NewResolver()
	r.Register("vault", &mockBackend{secrets: map[string]string{"TOKEN": "real-secret-value"}})

	got, refs, err := r.ResolveCallerParam("hello {{vault:TOKEN}} world", true)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got != "hello real-secret-value world" {
		t.Errorf("expected vault value to be substituted when opted in, got %q", got)
	}
	if len(refs) != 1 || refs[0] != "{{vault:TOKEN}}" {
		t.Errorf("expected secretRefs to record the original template, got %v", refs)
	}
}

func TestResolveCallerParam_AlwaysResolvesSafeBackends(t *testing.T) {
	// env / store / expr resolve regardless of allowSecretBackends.
	// They don't expose secrets the caller didn't already have.
	r := NewResolver()
	r.Register("env", &mockBackend{secrets: map[string]string{"USER": "alice"}})
	r.Register("store", &mockBackend{secrets: map[string]string{"CACHE": "cached-id"}})
	r.RegisterFunc("expr", func(c string) (string, error) { return "computed:" + c, nil })

	// allowSecretBackends=false should still resolve all three safe backends.
	got, refs, err := r.ResolveCallerParam("u={{env:USER}} c={{store:CACHE}} x={{expr:now}}", false)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	want := "u=alice c=cached-id x=computed:now"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	if len(refs) != 0 {
		t.Errorf("safe-backend refs must not be reported as secret, got %v", refs)
	}
}

func TestResolveCallerParam_MixedBackends(t *testing.T) {
	// One string with both safe and secret refs; safe ones resolve,
	// secret ones flow through unchanged, only the latter are
	// recorded. This is the realistic shape for an agent-supplied
	// param that includes both legitimate substitutions and an
	// accidental/malicious vault reference.
	r := NewResolver()
	r.Register("env", &mockBackend{secrets: map[string]string{"USER": "alice"}})
	r.Register("vault", &mockBackend{secrets: map[string]string{"TOKEN": "real-secret-value"}})

	got, refs, err := r.ResolveCallerParam("user={{env:USER}} key={{vault:TOKEN}}", false)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got != "user=alice key={{vault:TOKEN}}" {
		t.Errorf("got %q, want partial substitution", got)
	}
	if len(refs) != 0 {
		t.Errorf("expected no secret refs (vault not opted in), got %v", refs)
	}
}

func TestResolveCallerParam_RecordsMultipleSecretRefs(t *testing.T) {
	// Audit redaction needs to know about every secret-backend resolution
	// so the logger can replace each in turn.
	r := NewResolver()
	r.Register("vault", &mockBackend{secrets: map[string]string{
		"A": "value-a",
		"B": "value-b",
	}})

	got, refs, err := r.ResolveCallerParam("{{vault:A}}-{{vault:B}}-{{vault:A}}", true)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got != "value-a-value-b-value-a" {
		t.Errorf("got %q, want resolved", got)
	}
	if len(refs) != 3 {
		t.Errorf("expected 3 secret refs (one per occurrence), got %d (%v)", len(refs), refs)
	}
}

func TestResolveCallerParam_EscapedReferencePassesThrough(t *testing.T) {
	// The existing \{{ escape syntax must still work.
	r := NewResolver()
	r.Register("vault", &mockBackend{secrets: map[string]string{"TOKEN": "secret"}})

	got, _, err := r.ResolveCallerParam(`\{{vault:TOKEN}}`, true)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got != "{{vault:TOKEN}}" {
		t.Errorf("expected escape to pass literal {{...}} through, got %q", got)
	}
}

func TestResolveCallerParam_NilResolverIsNoop(t *testing.T) {
	var r *Resolver
	got, refs, err := r.ResolveCallerParam("{{vault:K}}", true)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got != "{{vault:K}}" || len(refs) != 0 {
		t.Errorf("nil resolver should be a no-op, got %q %v", got, refs)
	}
}

// --- RedactToTemplate ---

func TestRedactToTemplate_ReplacesSecretValueWithTemplate(t *testing.T) {
	r := NewResolver()
	r.Register("vault", &mockBackend{secrets: map[string]string{"TOKEN": "real-secret"}})

	// Simulate the proxy flow: caller param was "Bearer {{vault:TOKEN}}",
	// got resolved to "Bearer real-secret", we need to redact the
	// audit log back to "Bearer {{vault:TOKEN}}".
	resolved := "Bearer real-secret in some longer string"
	got := r.RedactToTemplate(resolved, []string{"{{vault:TOKEN}}"})
	if got != "Bearer {{vault:TOKEN}} in some longer string" {
		t.Errorf("redaction wrong: %q", got)
	}
}

func TestRedactToTemplate_NoOpWhenBackendMissing(t *testing.T) {
	// If we can't re-fetch the value (backend gone), leave the string
	// alone rather than corrupting it. The resolver was registered
	// when the resolution happened; this is a defensive guard for
	// the unlock-state-lost scenario.
	r := NewResolver() // no backend registered
	got := r.RedactToTemplate("the secret value", []string{"{{vault:K}}"})
	if got != "the secret value" {
		t.Errorf("expected no-op when backend missing, got %q", got)
	}
}
