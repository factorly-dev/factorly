// Copyright 2026 Jordan Sherer <hi@jordansherer.com>
// SPDX-License-Identifier: gpl

package vault

import (
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

func TestHasVaultRefsWithDefault(t *testing.T) {
	if !HasVaultRefs("{{vault:TOKEN|default}}") {
		t.Error("expected HasVaultRefs to detect ref with default")
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
