// Copyright 2026 Jordan Sherer <hi@jordansherer.com>
// SPDX-License-Identifier: gpl

package vault

import (
	"testing"
)

func TestExternalBackendSubstituteKey(t *testing.T) {
	args := []string{"read", "op://Development/{{key}}", "--format=json"}
	result := substituteKey(args, "GITHUB_TOKEN")

	if result[0] != "read" {
		t.Errorf("expected 'read', got %q", result[0])
	}
	if result[1] != "op://Development/GITHUB_TOKEN" {
		t.Errorf("expected substituted key, got %q", result[1])
	}
	if result[2] != "--format=json" {
		t.Errorf("expected unchanged arg, got %q", result[2])
	}
}

func TestExternalBackendSubstituteKeyWithSlashes(t *testing.T) {
	args := []string{"read", "{{key}}"}
	result := substituteKey(args, "MyVault/MyItem/password")

	if result[1] != "MyVault/MyItem/password" {
		t.Errorf("expected key with slashes, got %q", result[1])
	}
}

func TestExternalBackendGetWithEcho(t *testing.T) {
	b := NewExternalBackend("test", ExternalBackendConfig{
		Type: "cli",
		Get: BackendOpConfig{
			Command: "echo",
			Args:    []string{"secret_value_{{key}}"},
		},
	})

	val, err := b.Get("MY_KEY")
	if err != nil {
		t.Fatal(err)
	}
	if val != "secret_value_MY_KEY" {
		t.Errorf("expected 'secret_value_MY_KEY', got %q", val)
	}
}

func TestExternalBackendListWithEcho(t *testing.T) {
	b := NewExternalBackend("test", ExternalBackendConfig{
		Type: "cli",
		Get:  BackendOpConfig{Command: "echo"},
		List: &BackendOpConfig{
			Command: "printf",
			Args:    []string{"KEY1\nKEY2\nKEY3"},
		},
	})

	keys, err := b.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(keys) != 3 {
		t.Fatalf("expected 3 keys, got %d", len(keys))
	}
	if keys[0] != "KEY1" || keys[1] != "KEY2" || keys[2] != "KEY3" {
		t.Errorf("unexpected keys: %v", keys)
	}
}

func TestExternalBackendSetReadOnly(t *testing.T) {
	b := NewExternalBackend("op", ExternalBackendConfig{
		Type: "cli",
		Get:  BackendOpConfig{Command: "echo"},
	})

	err := b.Set("key", "value")
	if err == nil {
		t.Fatal("expected read-only error")
	}
	if err.Error() != `backend "op" is read-only — manage secrets in op directly` {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestExternalBackendDeleteReadOnly(t *testing.T) {
	b := NewExternalBackend("aws", ExternalBackendConfig{
		Type: "cli",
		Get:  BackendOpConfig{Command: "echo"},
	})

	err := b.Delete("key")
	if err == nil {
		t.Fatal("expected read-only error")
	}
}

func TestExternalBackendListNotConfigured(t *testing.T) {
	b := NewExternalBackend("test", ExternalBackendConfig{
		Type: "cli",
		Get:  BackendOpConfig{Command: "echo"},
	})

	_, err := b.List()
	if err == nil {
		t.Fatal("expected error for unconfigured list")
	}
}

func TestExternalBackendGetFailure(t *testing.T) {
	b := NewExternalBackend("test", ExternalBackendConfig{
		Type: "cli",
		Get: BackendOpConfig{
			Command: "false",
		},
	})

	_, err := b.Get("key")
	if err == nil {
		t.Fatal("expected error for failed get")
	}
}
