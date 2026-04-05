package provider

import (
	"testing"
	"time"
)

func TestResultIsError(t *testing.T) {
	tests := []struct {
		name     string
		result   Result
		expected bool
	}{
		{"success", Result{Output: "ok", ExitCode: 0}, false},
		{"nonzero exit", Result{ExitCode: 1}, true},
		{"error string", Result{Error: "something failed"}, true},
		{"both", Result{ExitCode: 2, Error: "fail"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.result.IsError() != tt.expected {
				t.Errorf("expected IsError=%v, got %v", tt.expected, tt.result.IsError())
			}
		})
	}
}

func TestCLIExecuteEcho(t *testing.T) {
	p := NewCLI(map[string]CLIToolDef{
		"test.echo": {
			Command: "echo",
			Args:    []string{"{message}"},
		},
	})

	result, err := p.Execute("test.echo", map[string]string{"message": "hello world"})
	if err != nil {
		t.Fatal(err)
	}
	if result.ExitCode != 0 {
		t.Errorf("expected exit code 0, got %d", result.ExitCode)
	}
	if result.Output != "hello world\n" {
		t.Errorf("expected 'hello world\\n', got %q", result.Output)
	}
	if result.Duration == 0 {
		t.Error("expected non-zero duration")
	}
}

func TestCLIExecuteMultipleParams(t *testing.T) {
	p := NewCLI(map[string]CLIToolDef{
		"test.multi": {
			Command: "echo",
			Args:    []string{"{first}", "{second}"},
		},
	})

	result, err := p.Execute("test.multi", map[string]string{
		"first":  "hello",
		"second": "world",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Output != "hello world\n" {
		t.Errorf("expected 'hello world\\n', got %q", result.Output)
	}
}

func TestCLIExecuteNoParams(t *testing.T) {
	p := NewCLI(map[string]CLIToolDef{
		"test.simple": {
			Command: "echo",
			Args:    []string{"static", "output"},
		},
	})

	result, err := p.Execute("test.simple", map[string]string{})
	if err != nil {
		t.Fatal(err)
	}
	if result.Output != "static output\n" {
		t.Errorf("expected 'static output\\n', got %q", result.Output)
	}
}

func TestCLIExecuteUnresolvedPlaceholder(t *testing.T) {
	p := NewCLI(map[string]CLIToolDef{
		"test.missing": {
			Command: "echo",
			Args:    []string{"{url}"},
		},
	})

	_, err := p.Execute("test.missing", map[string]string{})
	if err == nil {
		t.Fatal("expected error for unresolved placeholder")
	}
}

func TestCLIExecuteToolNotFound(t *testing.T) {
	p := NewCLI(map[string]CLIToolDef{})

	_, err := p.Execute("nonexistent", map[string]string{})
	if err == nil {
		t.Fatal("expected error for missing tool")
	}
}

func TestCLIExecuteFailingCommand(t *testing.T) {
	p := NewCLI(map[string]CLIToolDef{
		"test.fail": {
			Command: "false",
			Args:    []string{},
		},
	})

	result, err := p.Execute("test.fail", map[string]string{})
	if err != nil {
		t.Fatal(err)
	}
	if result.ExitCode == 0 {
		t.Error("expected non-zero exit code")
	}
	if !result.IsError() {
		t.Error("expected IsError=true for failing command")
	}
}

func TestCLIExecuteBadCommand(t *testing.T) {
	p := NewCLI(map[string]CLIToolDef{
		"test.bad": {
			Command: "nonexistent-binary-12345",
			Args:    []string{},
		},
	})

	result, err := p.Execute("test.bad", map[string]string{})
	if err != nil {
		t.Fatal(err)
	}
	if result.ExitCode == 0 {
		t.Error("expected non-zero exit code for bad command")
	}
}

func TestCLIExecuteWithEnv(t *testing.T) {
	p := NewCLI(map[string]CLIToolDef{
		"test.env": {
			Command: "sh",
			Args:    []string{"-c", "echo $FACTORLY_TEST_VAR"},
			Env:     map[string]string{"FACTORLY_TEST_VAR": "from_env"},
		},
	})

	result, err := p.Execute("test.env", map[string]string{})
	if err != nil {
		t.Fatal(err)
	}
	if result.Output != "from_env\n" {
		t.Errorf("expected 'from_env\\n', got %q", result.Output)
	}
}

func TestCLIExecuteTimeout(t *testing.T) {
	p := NewCLI(map[string]CLIToolDef{
		"test.slow": {
			Command: "sleep",
			Args:    []string{"10"},
			Timeout: 100 * time.Millisecond,
		},
	})

	result, err := p.Execute("test.slow", map[string]string{})
	if err != nil {
		t.Fatal(err)
	}
	if result.ExitCode == 0 {
		t.Error("expected non-zero exit code for timed out command")
	}
}

func TestCLIExecuteStdin(t *testing.T) {
	p := NewCLI(map[string]CLIToolDef{
		"test.stdin": {
			Command: "cat",
			Stdin:   "{input}",
		},
	})

	result, err := p.Execute("test.stdin", map[string]string{"input": "hello from stdin"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Output != "hello from stdin" {
		t.Errorf("expected 'hello from stdin', got %q", result.Output)
	}
}

func TestCLIExecuteStdinWithArgs(t *testing.T) {
	p := NewCLI(map[string]CLIToolDef{
		"test.grep": {
			Command: "grep",
			Args:    []string{"{pattern}"},
			Stdin:   "{input}",
		},
	})

	result, err := p.Execute("test.grep", map[string]string{
		"pattern": "hello",
		"input":   "hello world\ngoodbye world\nhello again",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.ExitCode != 0 {
		t.Errorf("expected exit code 0, got %d", result.ExitCode)
	}
	if result.Output != "hello world\nhello again\n" {
		t.Errorf("expected grep output, got %q", result.Output)
	}
}

func TestCLIExecuteStdinNoPlaceholder(t *testing.T) {
	p := NewCLI(map[string]CLIToolDef{
		"test.static": {
			Command: "cat",
			Stdin:   "static content",
		},
	})

	result, err := p.Execute("test.static", map[string]string{})
	if err != nil {
		t.Fatal(err)
	}
	if result.Output != "static content" {
		t.Errorf("expected 'static content', got %q", result.Output)
	}
}

func TestCLIExecuteNoStdin(t *testing.T) {
	// Verify existing behavior: no stdin field means no stdin piped
	p := NewCLI(map[string]CLIToolDef{
		"test.echo": {
			Command: "echo",
			Args:    []string{"{msg}"},
		},
	})

	result, err := p.Execute("test.echo", map[string]string{"msg": "works"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Output != "works\n" {
		t.Errorf("expected 'works\\n', got %q", result.Output)
	}
}

func TestSubstituteString(t *testing.T) {
	tests := []struct {
		name   string
		tmpl   string
		params map[string]string
		want   string
	}{
		{"single param", "{input}", map[string]string{"input": "hello"}, "hello"},
		{"embedded param", "prefix {val} suffix", map[string]string{"val": "X"}, "prefix X suffix"},
		{"multiple params", "{a} and {b}", map[string]string{"a": "1", "b": "2"}, "1 and 2"},
		{"no params", "static", map[string]string{}, "static"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := substituteString(tt.tmpl, tt.params)
			if got != tt.want {
				t.Errorf("substituteString(%q) = %q, want %q", tt.tmpl, got, tt.want)
			}
		})
	}
}

func TestSubstituteArgs(t *testing.T) {
	tests := []struct {
		name      string
		templates []string
		params    map[string]string
		expected  []string
	}{
		{
			"single param",
			[]string{"-s", "{url}"},
			map[string]string{"url": "https://example.com"},
			[]string{"-s", "https://example.com"},
		},
		{
			"multiple params",
			[]string{"-o", "{output}", "{url}"},
			map[string]string{"url": "https://example.com", "output": "file.html"},
			[]string{"-o", "file.html", "https://example.com"},
		},
		{
			"param in middle of string",
			[]string{"Authorization: Bearer {token}"},
			map[string]string{"token": "abc123"},
			[]string{"Authorization: Bearer abc123"},
		},
		{
			"no params",
			[]string{"-s", "--silent"},
			map[string]string{},
			[]string{"-s", "--silent"},
		},
		{
			"repeated param",
			[]string{"{val}", "{val}"},
			map[string]string{"val": "x"},
			[]string{"x", "x"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := substituteArgs(tt.templates, tt.params)
			if len(result) != len(tt.expected) {
				t.Fatalf("expected %d args, got %d", len(tt.expected), len(result))
			}
			for i, v := range result {
				if v != tt.expected[i] {
					t.Errorf("arg[%d]: expected %q, got %q", i, tt.expected[i], v)
				}
			}
		})
	}
}
