// Copyright 2026 Jordan Sherer <hi@jordansherer.com>
// SPDX-License-Identifier: gpl

package code

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/factorly-dev/factorly/internal/provider"
)

// fakeExecutor is a ToolExecutor that records calls and returns a
// scripted Result. Used to assert how scripts interact with the proxy
// without needing the real proxy stack.
type fakeExecutor struct {
	calls      int32
	lastName   string
	lastParams map[string]string
	lastIface  string
	respond    func(name string, params map[string]string) (*provider.Result, error)
}

func (f *fakeExecutor) ExecuteWithContext(_ context.Context, name string, params map[string]string, iface string) (*provider.Result, error) {
	atomic.AddInt32(&f.calls, 1)
	f.lastName = name
	f.lastParams = params
	f.lastIface = iface
	if f.respond != nil {
		return f.respond(name, params)
	}
	return &provider.Result{Output: "ok"}, nil
}

func TestRegisterCode_RejectsBadScripts(t *testing.T) {
	cases := []struct {
		name    string
		src     string
		mustErr string
	}{
		{
			name:    "missing package decl",
			src:     `func Run(params map[string]string) (any, error) { return nil, nil }`,
			mustErr: "package",
		},
		{
			name: "syntax error",
			src: `package s
func Run(params map[string]string) (any, error {
}`,
			mustErr: "compile",
		},
		{
			name: "missing Run function",
			src: `package s
func Other() {}`,
			mustErr: "Run",
		},
		{
			name: "wrong Run signature",
			src: `package s
func Run() string { return "no params" }`,
			mustErr: "signature",
		},
		{
			name: "denied import",
			src: `package s
import "os"
func Run(params map[string]string) (any, error) {
    return os.Getenv("HOME"), nil
}`,
			mustErr: "compile",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := NewProvider(&fakeExecutor{}, false)
			err := p.RegisterCode(tc.name, tc.src, 100)
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tc.mustErr)
			}
			if !strings.Contains(strings.ToLower(err.Error()), strings.ToLower(tc.mustErr)) {
				t.Errorf("error %q does not contain %q", err.Error(), tc.mustErr)
			}
		})
	}
}

func TestExecute_StringReturn(t *testing.T) {
	p := NewProvider(&fakeExecutor{}, false)
	src := `package s
func Run(params map[string]string) (any, error) {
    return "hello " + params["name"], nil
}`
	if err := p.RegisterCode("greet", src, 100); err != nil {
		t.Fatal(err)
	}
	res, err := p.ExecuteWithContext(context.Background(), "greet", map[string]string{"name": "world"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Output != "hello world" {
		t.Errorf("Output = %q, want %q", res.Output, "hello world")
	}
	if res.IsError() {
		t.Errorf("unexpected error result: %+v", res)
	}
}

func TestExecute_StructReturn_BecomesJSON(t *testing.T) {
	p := NewProvider(&fakeExecutor{}, false)
	src := `package s
func Run(params map[string]string) (any, error) {
    return map[string]int{"a": 1, "b": 2}, nil
}`
	if err := p.RegisterCode("data", src, 100); err != nil {
		t.Fatal(err)
	}
	res, err := p.ExecuteWithContext(context.Background(), "data", nil)
	if err != nil {
		t.Fatal(err)
	}
	// Map iteration order is nondeterministic in marshal; assert both
	// possible orderings.
	if res.Output != `{"a":1,"b":2}` && res.Output != `{"b":2,"a":1}` {
		t.Errorf("Output = %q, want JSON map", res.Output)
	}
}

func TestExecute_NilReturn(t *testing.T) {
	p := NewProvider(&fakeExecutor{}, false)
	src := `package s
func Run(params map[string]string) (any, error) {
    return nil, nil
}`
	if err := p.RegisterCode("noop", src, 100); err != nil {
		t.Fatal(err)
	}
	res, err := p.ExecuteWithContext(context.Background(), "noop", nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.Output != "" {
		t.Errorf("Output = %q, want empty string", res.Output)
	}
	if res.IsError() {
		t.Errorf("unexpected error: %+v", res)
	}
}

func TestExecute_ErrorReturn(t *testing.T) {
	p := NewProvider(&fakeExecutor{}, false)
	src := `package s
import "errors"
func Run(params map[string]string) (any, error) {
    return nil, errors.New("boom")
}`
	if err := p.RegisterCode("err", src, 100); err != nil {
		t.Fatal(err)
	}
	res, err := p.ExecuteWithContext(context.Background(), "err", nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.ExitCode != 1 {
		t.Errorf("ExitCode = %d, want 1", res.ExitCode)
	}
	if !strings.Contains(res.Error, "boom") {
		t.Errorf("Error = %q, want to contain 'boom'", res.Error)
	}
}

func TestExecute_Panic(t *testing.T) {
	p := NewProvider(&fakeExecutor{}, false)
	src := `package s
func Run(params map[string]string) (any, error) {
    panic("script blew up")
}`
	if err := p.RegisterCode("boom", src, 100); err != nil {
		t.Fatal(err)
	}
	res, err := p.ExecuteWithContext(context.Background(), "boom", nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.ExitCode != 2 {
		t.Errorf("ExitCode = %d, want 2 (panic)", res.ExitCode)
	}
	if !strings.Contains(res.Error, "panic") {
		t.Errorf("Error = %q, want to contain 'panic'", res.Error)
	}
}

func TestExecute_FactorlyCall_RoundTrip(t *testing.T) {
	fake := &fakeExecutor{
		respond: func(name string, params map[string]string) (*provider.Result, error) {
			return &provider.Result{Output: "fetched: " + params["url"]}, nil
		},
	}
	p := NewProvider(fake, false)
	src := `package s
import (
    "errors"
    "factorly"
)
func Run(params map[string]string) (any, error) {
    res, err := factorly.Call("web.fetch", map[string]string{"url": params["target"]})
    if err != nil { return nil, err }
    if res.IsError() { return nil, errors.New(res.Error) }
    return res.Output, nil
}`
	if err := p.RegisterCode("fetch", src, 100); err != nil {
		t.Fatal(err)
	}
	res, err := p.ExecuteWithContext(context.Background(), "fetch", map[string]string{"target": "https://example.com"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Output != "fetched: https://example.com" {
		t.Errorf("Output = %q", res.Output)
	}
	if fake.lastName != "web.fetch" {
		t.Errorf("inner call name = %q, want web.fetch", fake.lastName)
	}
	if fake.lastIface != "code" {
		t.Errorf("inner iface = %q, want code", fake.lastIface)
	}
	if fake.lastParams["url"] != "https://example.com" {
		t.Errorf("inner params url = %q", fake.lastParams["url"])
	}
}

func TestExecute_FactorlyCall_PropagatesToolError(t *testing.T) {
	fake := &fakeExecutor{
		respond: func(name string, params map[string]string) (*provider.Result, error) {
			// Tool ran but errored — returns (*Result, nil) with IsError().
			return &provider.Result{Error: "HTTP 500", ExitCode: 1}, nil
		},
	}
	p := NewProvider(fake, false)
	src := `package s
import (
    "errors"
    "factorly"
)
func Run(params map[string]string) (any, error) {
    res, err := factorly.Call("x", nil)
    if err != nil { return nil, err }
    if res.IsError() { return nil, errors.New("tool failed: " + res.Error) }
    return res.Output, nil
}`
	if err := p.RegisterCode("t", src, 100); err != nil {
		t.Fatal(err)
	}
	res, err := p.ExecuteWithContext(context.Background(), "t", nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Error, "tool failed: HTTP 500") {
		t.Errorf("Error = %q, want propagated message", res.Error)
	}
}

func TestExecute_FactorlyCall_PropagatesInfraError(t *testing.T) {
	wantErr := errors.New("denied by shadow policy")
	fake := &fakeExecutor{
		respond: func(name string, params map[string]string) (*provider.Result, error) {
			return nil, wantErr
		},
	}
	p := NewProvider(fake, false)
	src := `package s
import "factorly"
func Run(params map[string]string) (any, error) {
    _, err := factorly.Call("x", nil)
    return nil, err
}`
	if err := p.RegisterCode("t", src, 100); err != nil {
		t.Fatal(err)
	}
	res, err := p.ExecuteWithContext(context.Background(), "t", nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Error, "denied by shadow policy") {
		t.Errorf("Error = %q", res.Error)
	}
}

func TestExecute_MaxCallsBudget(t *testing.T) {
	fake := &fakeExecutor{
		respond: func(name string, params map[string]string) (*provider.Result, error) {
			return &provider.Result{Output: "ok"}, nil
		},
	}
	p := NewProvider(fake, false)
	src := `package s
import (
    "fmt"
    "factorly"
)
func Run(params map[string]string) (any, error) {
    for i := 0; i < 200; i++ {
        _, err := factorly.Call("x", nil)
        if err != nil {
            return nil, fmt.Errorf("call %d: %w", i, err)
        }
    }
    return "completed", nil
}`
	if err := p.RegisterCode("loop", src, 5); err != nil {
		t.Fatal(err)
	}
	res, err := p.ExecuteWithContext(context.Background(), "loop", nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Error, "max_calls") {
		t.Errorf("Error = %q, want to contain 'max_calls'", res.Error)
	}
	// 5 successful + 1 over-budget call = 6 total invocations attempted
	// of fake.ExecuteWithContext, but only the first 5 reach it (the
	// 6th is rejected before delegating). So fake.calls should be 5.
	if got := atomic.LoadInt32(&fake.calls); got != 5 {
		t.Errorf("fake.calls = %d, want 5", got)
	}
}

func TestRemoveCode(t *testing.T) {
	p := NewProvider(&fakeExecutor{}, false)
	src := `package s
func Run(params map[string]string) (any, error) { return "x", nil }`
	if err := p.RegisterCode("t", src, 100); err != nil {
		t.Fatal(err)
	}
	p.RemoveCode("t")
	_, err := p.ExecuteWithContext(context.Background(), "t", nil)
	if err == nil || !strings.Contains(err.Error(), "not registered") {
		t.Errorf("expected not-registered error, got %v", err)
	}
}
