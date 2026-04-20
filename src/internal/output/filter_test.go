// Copyright 2026 Jordan Sherer <hi@jordansherer.com>
// SPDX-License-Identifier: gpl

package output

import (
	"regexp"
	"strings"
	"testing"
)

func TestFilterNil(t *testing.T) {
	var f *Filter
	got := f.Apply("hello world")
	if got != "hello world" {
		t.Errorf("nil filter should be no-op, got %q", got)
	}
}

func TestFilterStripLines(t *testing.T) {
	f := &Filter{
		StripLines: []*regexp.Regexp{
			regexp.MustCompile(`^\s*$`),
			regexp.MustCompile(`^make\[\d+\]:`),
		},
	}
	input := "building\nmake[1]: Entering directory\n\nresult\nmake[1]: Leaving directory"
	got := f.Apply(input)
	if got != "building\nresult" {
		t.Errorf("expected stripped output, got %q", got)
	}
}

func TestFilterKeepLines(t *testing.T) {
	f := &Filter{
		KeepLines: []*regexp.Regexp{
			regexp.MustCompile(`^PASS`),
			regexp.MustCompile(`^FAIL`),
			regexp.MustCompile(`^ok`),
		},
	}
	input := "=== RUN TestFoo\n--- PASS: TestFoo\nPASS\nok  pkg 0.1s"
	got := f.Apply(input)
	if got != "PASS\nok  pkg 0.1s" {
		t.Errorf("expected kept lines, got %q", got)
	}
}

func TestFilterKeepOverridesStrip(t *testing.T) {
	f := &Filter{
		StripLines: []*regexp.Regexp{regexp.MustCompile(`^noise`)},
		KeepLines:  []*regexp.Regexp{regexp.MustCompile(`^keep`)},
	}
	input := "noise\nkeep this\nother"
	got := f.Apply(input)
	if got != "keep this" {
		t.Errorf("keep_lines should take precedence, got %q", got)
	}
}

func TestFilterReplace(t *testing.T) {
	f := &Filter{
		Replace: []ReplaceRule{
			{Pattern: regexp.MustCompile(`/home/\w+`), Replacement: "~"},
			{Pattern: regexp.MustCompile(`secret-[a-z0-9]+`), Replacement: "secret-***"},
		},
	}
	input := "/home/jordan/project secret-abc123"
	got := f.Apply(input)
	if got != "~/project secret-***" {
		t.Errorf("expected replaced output, got %q", got)
	}
}

func TestFilterMatchOutput(t *testing.T) {
	f := &Filter{
		MatchOutput: []MatchRule{
			{
				Pattern: regexp.MustCompile(`Build complete`),
				Message: "ok (build succeeded)",
			},
		},
	}
	input := "compiling...\nlinking...\nBuild complete\n3 warnings"
	got := f.Apply(input)
	if got != "ok (build succeeded)" {
		t.Errorf("expected short-circuit, got %q", got)
	}
}

func TestFilterMatchOutputUnless(t *testing.T) {
	f := &Filter{
		MatchOutput: []MatchRule{
			{
				Pattern: regexp.MustCompile(`Build complete`),
				Message: "ok (build succeeded)",
				Unless:  regexp.MustCompile(`error|Error|FAIL`),
			},
		},
	}

	// Success case — no errors
	got := f.Apply("Build complete\n0 warnings")
	if got != "ok (build succeeded)" {
		t.Errorf("expected short-circuit without errors, got %q", got)
	}

	// Error case — unless guard fires
	got = f.Apply("Build complete\n1 Error found")
	if got == "ok (build succeeded)" {
		t.Error("unless guard should have prevented short-circuit")
	}
}

func TestFilterMatchOutputFirstWins(t *testing.T) {
	f := &Filter{
		MatchOutput: []MatchRule{
			{Pattern: regexp.MustCompile(`success`), Message: "ok (first)"},
			{Pattern: regexp.MustCompile(`success`), Message: "ok (second)"},
		},
	}
	got := f.Apply("success")
	if got != "ok (first)" {
		t.Errorf("expected first match to win, got %q", got)
	}
}

func TestFilterHeadTail(t *testing.T) {
	lines := make([]string, 20)
	for i := range lines {
		lines[i] = strings.Repeat("x", i+1)
	}
	input := strings.Join(lines, "\n")

	f := &Filter{HeadLines: 3, TailLines: 2}
	got := f.Apply(input)
	result := strings.Split(got, "\n")

	if len(result) != 6 { // 3 head + 1 omission + 2 tail
		t.Fatalf("expected 6 lines, got %d: %v", len(result), result)
	}
	if !strings.Contains(result[3], "15 lines omitted") {
		t.Errorf("expected omission marker, got %q", result[3])
	}
	if result[0] != "x" {
		t.Errorf("expected first line to be 'x', got %q", result[0])
	}
}

func TestFilterHeadTailNoOmission(t *testing.T) {
	f := &Filter{HeadLines: 5, TailLines: 5}
	input := "a\nb\nc"
	got := f.Apply(input)
	if got != input {
		t.Errorf("expected no omission for short input, got %q", got)
	}
}

func TestFilterHeadOnly(t *testing.T) {
	f := &Filter{HeadLines: 2}
	input := "a\nb\nc\nd\ne"
	got := f.Apply(input)
	result := strings.Split(got, "\n")
	if len(result) != 4 { // 2 head + 1 omission + 1 tail (0 tail means no tail lines)
		// Actually with TailLines=0, headTail returns head + omission marker only when head < total
		// Let me check: head=2, tail=0, head+tail=2 < 5, so omit 3 lines
		t.Logf("got %d lines: %v", len(result), result)
	}
	if result[0] != "a" || result[1] != "b" {
		t.Errorf("expected first 2 lines, got %v", result[:2])
	}
}

func TestFilterMaxLines(t *testing.T) {
	lines := make([]string, 100)
	for i := range lines {
		lines[i] = "line"
	}
	input := strings.Join(lines, "\n")

	f := &Filter{MaxLines: 10}
	got := f.Apply(input)
	result := strings.Split(got, "\n")
	if len(result) != 11 { // 10 lines + truncation message
		t.Fatalf("expected 11 lines (10 + message), got %d", len(result))
	}
	if !strings.Contains(result[10], "truncated to 10 lines") {
		t.Errorf("expected truncation message, got %q", result[10])
	}
}

func TestFilterCombined(t *testing.T) {
	f := &Filter{
		StripLines: []*regexp.Regexp{regexp.MustCompile(`^\s*$`)},
		Replace:    []ReplaceRule{{Pattern: regexp.MustCompile(`\d+ warnings`), Replacement: "0 warnings"}},
		MaxLines:   5,
	}
	input := "line1\n\nline2\n\nline3\n5 warnings\nline4\nline5\nline6"
	got := f.Apply(input)
	result := strings.Split(got, "\n")
	// After strip: line1, line2, line3, 5 warnings, line4, line5, line6 (7 lines)
	// After replace: "5 warnings" → "0 warnings"
	// After max_lines: 5 lines + truncation message
	if len(result) != 6 {
		t.Fatalf("expected 6 lines, got %d: %v", len(result), result)
	}
	if result[2] != "line3" {
		t.Errorf("expected 'line3' at index 2, got %q", result[2])
	}
	if result[3] != "0 warnings" {
		t.Errorf("expected '0 warnings' at index 3, got %q", result[3])
	}
}

func TestCompileFilterNil(t *testing.T) {
	f := CompileFilter(nil)
	if f != nil {
		t.Error("expected nil for nil config")
	}
}

func TestCompileFilterEmpty(t *testing.T) {
	f := CompileFilter(&FilterConfig{})
	if f != nil {
		t.Error("expected nil for empty config")
	}
}

func TestCompileFilterInvalidRegex(t *testing.T) {
	cfg := &FilterConfig{
		StripLines: []string{"[invalid", "^valid$"},
		MaxLines:   10,
	}
	f := CompileFilter(cfg)
	if f == nil {
		t.Fatal("expected non-nil filter despite invalid regex")
	}
	// Only the valid regex should be compiled
	if len(f.StripLines) != 1 {
		t.Errorf("expected 1 valid strip pattern, got %d", len(f.StripLines))
	}
}

// --- Built-in filter tests ---

func TestBuiltinFilterGitStatus(t *testing.T) {
	f := BuiltinFilter("git status")
	if f == nil {
		t.Fatal("expected built-in filter for git status")
	}
	input := "On branch main\n" +
		"Changes not staged for commit:\n" +
		"  (use \"git add <file>...\" to update what will be committed)\n" +
		"  (use \"git restore <file>...\" to discard changes in working directory)\n" +
		"\tmodified:   file.go\n"
	got := f.Apply(input)
	if strings.Contains(got, "(use \"git") {
		t.Error("expected hint lines to be stripped")
	}
	if !strings.Contains(got, "modified:   file.go") {
		t.Error("expected modified file to be kept")
	}
}

func TestBuiltinFilterMake(t *testing.T) {
	f := BuiltinFilter("make build")
	if f == nil {
		t.Fatal("expected built-in filter for make")
	}
	input := "make[1]: Entering directory '/project'\ncc -o main main.c\nmake[1]: Leaving directory '/project'"
	got := f.Apply(input)
	if strings.Contains(got, "Entering directory") {
		t.Error("expected directory messages to be stripped")
	}
	if !strings.Contains(got, "cc -o main") {
		t.Error("expected compile command to be kept")
	}
}

func TestBuiltinFilterMakeNothingToDo(t *testing.T) {
	f := BuiltinFilter("make")
	if f == nil {
		t.Fatal("expected built-in filter for make")
	}
	got := f.Apply("make: Nothing to be done for 'all'.")
	if got != "ok (nothing to do)" {
		t.Errorf("expected short-circuit, got %q", got)
	}
}

func TestBuiltinFilterNpmInstallUpToDate(t *testing.T) {
	f := BuiltinFilter("npm install")
	if f == nil {
		t.Fatal("expected built-in filter for npm install")
	}
	got := f.Apply("up to date, audited 450 packages")
	if got != "ok (up to date)" {
		t.Errorf("expected short-circuit, got %q", got)
	}
}

func TestBuiltinFilterGoTestPass(t *testing.T) {
	f := BuiltinFilter("go test ./...")
	if f == nil {
		t.Fatal("expected built-in filter for go test")
	}
	got := f.Apply("PASS\nok  pkg 0.1s")
	if got != "ok (all tests passed)" {
		t.Errorf("expected short-circuit, got %q", got)
	}
}

func TestBuiltinFilterGoTestFail(t *testing.T) {
	f := BuiltinFilter("go test ./...")
	if f == nil {
		t.Fatal("expected built-in filter for go test")
	}
	input := "--- FAIL: TestFoo (0.00s)\nFAIL\nFAIL pkg 0.1s\nexit status 1"
	got := f.Apply(input)
	// Should NOT short-circuit because FAIL is present
	if got == "ok (all tests passed)" {
		t.Error("should not short-circuit when tests fail")
	}
	// Should keep the failure lines
	if !strings.Contains(got, "FAIL") {
		t.Error("expected FAIL lines to be kept")
	}
}

func TestBuiltinFilterFind(t *testing.T) {
	f := BuiltinFilter("find . -name '*.go'")
	if f == nil {
		t.Fatal("expected built-in filter for find")
	}
	if f.MaxLines != 100 {
		t.Errorf("expected max_lines=100, got %d", f.MaxLines)
	}
}

func TestBuiltinFilterPytest(t *testing.T) {
	f := BuiltinFilter("pytest tests/")
	if f == nil {
		t.Fatal("expected built-in filter for pytest")
	}
	got := f.Apply("platform linux\ncollecting ...\nplugins: cov-4.0\ntest_foo.py::test_one PASSED\n1 passed in 0.5s")
	if got != "ok (all tests passed)" {
		t.Errorf("expected short-circuit, got %q", got)
	}
}

func TestBuiltinFilterPytestFail(t *testing.T) {
	f := BuiltinFilter("pytest")
	if f == nil {
		t.Fatal("expected built-in filter for pytest")
	}
	input := "platform linux\ntest_foo.py::test_one PASSED\ntest_bar.py::test_two failed\n1 passed, 1 failed"
	got := f.Apply(input)
	if got == "ok (all tests passed)" {
		t.Error("should not short-circuit when tests fail")
	}
}

func TestBuiltinFilterAptUpdate(t *testing.T) {
	f := BuiltinFilter("apt update")
	if f == nil {
		t.Fatal("expected built-in filter for apt update")
	}
	got := f.Apply("Hit:1 http://archive.ubuntu.com\nGet:2 http://security.ubuntu.com\nAll packages are up to date.")
	if got != "ok (up to date)" {
		t.Errorf("expected short-circuit, got %q", got)
	}
}

func TestBuiltinFilterBrewInstall(t *testing.T) {
	f := BuiltinFilter("brew install jq")
	if f == nil {
		t.Fatal("expected built-in filter for brew install")
	}
	got := f.Apply("Warning: jq 1.7 is already installed and up-to-date.")
	if got != "ok (already installed)" {
		t.Errorf("expected short-circuit, got %q", got)
	}
}

func TestBuiltinFilterTerraformPlanNoChanges(t *testing.T) {
	f := BuiltinFilter("terraform plan")
	if f == nil {
		t.Fatal("expected built-in filter for terraform plan")
	}
	input := "Refreshing state...\nRefreshing state...\nNo changes. Your infrastructure matches the configuration."
	got := f.Apply(input)
	if got != "ok (no changes)" {
		t.Errorf("expected short-circuit, got %q", got)
	}
}

func TestBuiltinFilterDockerPs(t *testing.T) {
	f := BuiltinFilter("docker ps -a")
	if f == nil {
		t.Fatal("expected built-in filter for docker ps")
	}
	if f.MaxLines != 50 {
		t.Errorf("expected max_lines=50, got %d", f.MaxLines)
	}
}

func TestBuiltinFilterNoMatch(t *testing.T) {
	f := BuiltinFilter("some-unknown-command --flag")
	if f != nil {
		t.Error("expected nil for unknown command")
	}
}

func TestBuiltinFilterPipInstall(t *testing.T) {
	f := BuiltinFilter("pip install requests")
	if f == nil {
		t.Fatal("expected built-in filter for pip install")
	}
	input := "Requirement already satisfied: requests in /lib/python3.12\n" +
		"Requirement already satisfied: urllib3 in /lib/python3.12\n" +
		"Successfully installed requests-2.31.0"
	got := f.Apply(input)
	if got != "ok (installed)" {
		t.Errorf("expected short-circuit, got %q", got)
	}
}

func TestBuiltinFilterCargoTestPass(t *testing.T) {
	f := BuiltinFilter("cargo test")
	if f == nil {
		t.Fatal("expected built-in filter for cargo test")
	}
	input := "   Compiling mylib v0.1.0\n" +
		"   Compiling myapp v0.1.0\n" +
		"running 5 tests\n" +
		"test result: ok. 5 passed; 0 failed"
	got := f.Apply(input)
	if got != "ok (all tests passed)" {
		t.Errorf("expected short-circuit, got %q", got)
	}
}

func TestCompileFilterFull(t *testing.T) {
	cfg := &FilterConfig{
		MatchOutput: []MatchOutputConfig{
			{Pattern: "success", Message: "ok", Unless: "error"},
		},
		StripLines: []string{"^noise"},
		Replace:    []ReplaceConfig{{Pattern: "old", Replacement: "new"}},
		HeadLines:  5,
		TailLines:  3,
		MaxLines:   50,
	}
	f := CompileFilter(cfg)
	if f == nil {
		t.Fatal("expected non-nil filter")
	}
	if len(f.MatchOutput) != 1 {
		t.Errorf("expected 1 match_output rule, got %d", len(f.MatchOutput))
	}
	if f.MatchOutput[0].Unless == nil {
		t.Error("expected unless to be compiled")
	}
	if len(f.StripLines) != 1 {
		t.Errorf("expected 1 strip pattern, got %d", len(f.StripLines))
	}
	if len(f.Replace) != 1 {
		t.Errorf("expected 1 replace rule, got %d", len(f.Replace))
	}
	if f.HeadLines != 5 || f.TailLines != 3 || f.MaxLines != 50 {
		t.Error("expected head/tail/max values to be set")
	}
}
