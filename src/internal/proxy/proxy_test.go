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
)

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
