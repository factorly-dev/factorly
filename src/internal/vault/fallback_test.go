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
