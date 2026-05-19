// Copyright 2026 Jordan Sherer <hi@jordansherer.com>
// SPDX-License-Identifier: gpl

package vault

import (
	"testing"
)

type fallbackMockBackend struct {
	data map[string]string
}

func newFBMock(data map[string]string) *fallbackMockBackend {
	return &fallbackMockBackend{data: data}
}

func (m *fallbackMockBackend) Get(key string) (string, error) {
	if v, ok := m.data[key]; ok {
		return v, nil
	}
	return "", ErrNotFound
}
func (m *fallbackMockBackend) Set(key, value string) error { m.data[key] = value; return nil }
func (m *fallbackMockBackend) Delete(key string) error     { delete(m.data, key); return nil }
func (m *fallbackMockBackend) List() ([]string, error) {
	keys := make([]string, 0, len(m.data))
	for k := range m.data {
		keys = append(keys, k)
	}
	return keys, nil
}
func (m *fallbackMockBackend) Close() error { return nil }

func TestFallbackGetPrimaryFirst(t *testing.T) {
	primary := newFBMock(map[string]string{"KEY": "project_value"})
	secondary := newFBMock(map[string]string{"KEY": "global_value"})
	fb := &FallbackBackend{Primary: primary, Secondary: secondary}

	val, err := fb.Get("KEY")
	if err != nil {
		t.Fatal(err)
	}
	if val != "project_value" {
		t.Errorf("expected project_value, got %q", val)
	}
}

func TestFallbackGetFallsBackToSecondary(t *testing.T) {
	primary := newFBMock(map[string]string{})
	secondary := newFBMock(map[string]string{"GLOBAL_KEY": "global_value"})
	fb := &FallbackBackend{Primary: primary, Secondary: secondary}

	val, err := fb.Get("GLOBAL_KEY")
	if err != nil {
		t.Fatal(err)
	}
	if val != "global_value" {
		t.Errorf("expected global_value, got %q", val)
	}
}

func TestFallbackGetNotFound(t *testing.T) {
	primary := newFBMock(map[string]string{})
	secondary := newFBMock(map[string]string{})
	fb := &FallbackBackend{Primary: primary, Secondary: secondary}

	_, err := fb.Get("MISSING")
	if err != ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestFallbackSetTargetsPrimary(t *testing.T) {
	primary := newFBMock(map[string]string{})
	secondary := newFBMock(map[string]string{})
	fb := &FallbackBackend{Primary: primary, Secondary: secondary}

	if err := fb.Set("NEW_KEY", "new_value"); err != nil {
		t.Fatal(err)
	}

	// Should be in primary, not secondary
	if primary.data["NEW_KEY"] != "new_value" {
		t.Error("expected key in primary")
	}
	if _, ok := secondary.data["NEW_KEY"]; ok {
		t.Error("expected key NOT in secondary")
	}
}

func TestFallbackDeleteTargetsPrimary(t *testing.T) {
	primary := newFBMock(map[string]string{"KEY": "val"})
	secondary := newFBMock(map[string]string{"KEY": "val"})
	fb := &FallbackBackend{Primary: primary, Secondary: secondary}

	if err := fb.Delete("KEY"); err != nil {
		t.Fatal(err)
	}

	if _, ok := primary.data["KEY"]; ok {
		t.Error("expected key deleted from primary")
	}
	if secondary.data["KEY"] != "val" {
		t.Error("expected key preserved in secondary")
	}
}

func TestFallbackListFromPrimary(t *testing.T) {
	primary := newFBMock(map[string]string{"A": "1", "B": "2"})
	secondary := newFBMock(map[string]string{"C": "3"})
	fb := &FallbackBackend{Primary: primary, Secondary: secondary}

	keys, err := fb.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(keys) != 2 {
		t.Errorf("expected 2 keys from primary, got %d", len(keys))
	}
}

func TestFallbackNilPrimary(t *testing.T) {
	secondary := newFBMock(map[string]string{"KEY": "val"})
	fb := &FallbackBackend{Primary: nil, Secondary: secondary}

	val, err := fb.Get("KEY")
	if err != nil {
		t.Fatal(err)
	}
	if val != "val" {
		t.Errorf("expected val, got %q", val)
	}
}

func TestFallbackNilSecondary(t *testing.T) {
	primary := newFBMock(map[string]string{"KEY": "val"})
	fb := &FallbackBackend{Primary: primary, Secondary: nil}

	val, err := fb.Get("KEY")
	if err != nil {
		t.Fatal(err)
	}
	if val != "val" {
		t.Errorf("expected val, got %q", val)
	}

	// Missing key with nil secondary
	_, err = fb.Get("MISSING")
	if err != ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

// TestFallbackNests verifies a three-tier chain: a FallbackBackend
// nested as the Primary of another FallbackBackend. Used by workspace
// vaults to chain workspace → project → global.
func TestFallbackNests(t *testing.T) {
	workspace := newFBMock(map[string]string{"WS_ONLY": "ws"})
	project := newFBMock(map[string]string{"SHARED": "proj", "PROJ_ONLY": "p"})
	global := newFBMock(map[string]string{"GLOBAL_ONLY": "g"})

	// project → global
	inner := &FallbackBackend{Primary: project, Secondary: global}
	// workspace → (project → global)
	outer := &FallbackBackend{Primary: workspace, Secondary: inner}

	cases := []struct {
		key, want string
	}{
		{"WS_ONLY", "ws"},
		{"SHARED", "proj"},
		{"PROJ_ONLY", "p"},
		{"GLOBAL_ONLY", "g"},
	}
	for _, c := range cases {
		got, err := outer.Get(c.key)
		if err != nil {
			t.Errorf("Get(%q): unexpected error %v", c.key, err)
			continue
		}
		if got != c.want {
			t.Errorf("Get(%q) = %q, want %q", c.key, got, c.want)
		}
	}

	// Set targets the outermost Primary (workspace), matching the
	// "writes go to the explicit tier" contract.
	if err := outer.Set("NEW", "value"); err != nil {
		t.Fatal(err)
	}
	if workspace.data["NEW"] != "value" {
		t.Errorf("Set should land in workspace; workspace=%+v project=%+v", workspace.data, project.data)
	}
	if _, ok := project.data["NEW"]; ok {
		t.Errorf("Set should not touch project vault")
	}
}

func TestEnsureSecondaryFiresLazyOpen(t *testing.T) {
	primary := newFBMock(map[string]string{"PRI": "p"})
	openCalls := 0
	fb := &FallbackBackend{
		Primary: primary,
		SecondaryOpen: func() (Backend, error) {
			openCalls++
			return newFBMock(map[string]string{"SEC": "s"}), nil
		},
	}
	if openCalls != 0 {
		t.Errorf("SecondaryOpen called %d times before EnsureSecondary", openCalls)
	}
	got := fb.EnsureSecondary()
	if got == nil {
		t.Fatal("EnsureSecondary returned nil")
	}
	if openCalls != 1 {
		t.Errorf("expected exactly 1 open, got %d", openCalls)
	}
	if val, _ := got.Get("SEC"); val != "s" {
		t.Errorf("EnsureSecondary returned wrong backend: Get(SEC)=%q", val)
	}
	// Calling again should not re-open — Secondary is now cached.
	again := fb.EnsureSecondary()
	if openCalls != 1 {
		t.Errorf("EnsureSecondary re-opened: openCalls=%d", openCalls)
	}
	if again != got {
		t.Error("EnsureSecondary returned a different backend on second call")
	}
}

func TestEnsureSecondaryReturnsNilOnOpenFailure(t *testing.T) {
	primary := newFBMock(map[string]string{})
	fb := &FallbackBackend{
		Primary: primary,
		SecondaryOpen: func() (Backend, error) {
			return nil, ErrNotFound
		},
	}
	if got := fb.EnsureSecondary(); got != nil {
		t.Errorf("expected nil on open failure, got %T", got)
	}
}

func TestEnsureSecondaryNoOpWhenAlreadySet(t *testing.T) {
	primary := newFBMock(nil)
	secondary := newFBMock(map[string]string{"K": "v"})
	fb := &FallbackBackend{Primary: primary, Secondary: secondary}
	if got := fb.EnsureSecondary(); got != secondary {
		t.Error("EnsureSecondary should return existing Secondary unchanged")
	}
}
