package proxy

import (
	"testing"
	"time"

	"github.com/factorly-dev/factorly-cli/internal/logger"
	"github.com/factorly-dev/factorly-cli/internal/provider"
	"github.com/factorly-dev/factorly-cli/internal/registry"
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
