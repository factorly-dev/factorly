// Copyright 2026 Jordan Sherer <hi@jordansherer.com>
// SPDX-License-Identifier: gpl

package promote

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/factorly-dev/factorly/internal/logger"
)

// writeLog seeds a JSONL audit log file with the given entries and
// returns the path. Helper to keep table-driven tests compact.
func writeLog(t *testing.T, entries []*logger.Entry) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "audit.jsonl")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	for _, e := range entries {
		if err := enc.Encode(e); err != nil {
			t.Fatal(err)
		}
	}
	return path
}

func TestFromLogRecoversSource(t *testing.T) {
	src := "package main\nfunc Run(p map[string]string) (any, error) { return \"hi\", nil }"
	logPath := writeLog(t, []*logger.Entry{
		{
			Timestamp: time.Now(),
			Tool:      "factorly.code",
			Params: map[string]string{
				"code":   src,
				"params": `{"who":"world"}`,
			},
			SourceSHA: "abc123def4567890abc123def4567890abc123def4567890abc123def4567890",
		},
	})

	res, err := FromLog(logPath, "abc1")
	if err != nil {
		t.Fatal(err)
	}
	if res.Source != src {
		t.Errorf("Source mismatch:\n got %q\nwant %q", res.Source, src)
	}
	if !strings.HasPrefix(res.SHA, "abc1") {
		t.Errorf("SHA = %q, want prefix abc1", res.SHA)
	}
	if len(res.Parameters) != 1 || res.Parameters[0].Name != "who" {
		t.Errorf("expected one inferred param 'who', got %+v", res.Parameters)
	}
	if res.Parameters[0].Default != "world" {
		t.Errorf("param default = %q, want 'world'", res.Parameters[0].Default)
	}
	if res.Parameters[0].Type != "string" {
		t.Errorf("param type = %q, want 'string'", res.Parameters[0].Type)
	}
}

func TestFromLogPrefersMostRecentEntry(t *testing.T) {
	src1 := "package main\nfunc Run(p map[string]string) (any, error) { return \"v1\", nil }"
	src2 := "package main\nfunc Run(p map[string]string) (any, error) { return \"v2\", nil }"
	// Same SHA prefix can't have different bodies, but the operator
	// might have re-run the same script with different params — we
	// want the MOST RECENT run so the inferred defaults reflect
	// current usage. Build two entries with the same SHA but
	// different param values.
	sha := "deadbeef" + strings.Repeat("0", 56)
	logPath := writeLog(t, []*logger.Entry{
		{Timestamp: time.Now().Add(-time.Hour), Tool: "factorly.code",
			Params: map[string]string{"code": src1, "params": `{"n":"1"}`}, SourceSHA: sha},
		{Timestamp: time.Now(), Tool: "factorly.code",
			Params: map[string]string{"code": src2, "params": `{"n":"2"}`}, SourceSHA: sha},
	})

	res, err := FromLog(logPath, "deadbeef")
	if err != nil {
		t.Fatal(err)
	}
	if res.Source != src2 {
		t.Error("expected the second (most recent) entry's source")
	}
	if res.Parameters[0].Default != "2" {
		t.Errorf("param default = %q, want '2' (from most-recent run)", res.Parameters[0].Default)
	}
}

func TestFromLogIgnoresNonCodeEntries(t *testing.T) {
	// A CLI tool with the same SHA shouldn't be returned — only
	// factorly.code entries are promotable. This protects against
	// SourceSHA being repurposed for non-code use later.
	logPath := writeLog(t, []*logger.Entry{
		{Tool: "shell.run", SourceSHA: "abc12300" + strings.Repeat("0", 56)},
		{Tool: "factorly.code",
			Params:    map[string]string{"code": "package main\nfunc Run(p map[string]string) (any, error) { return nil, nil }"},
			SourceSHA: "abc123ff" + strings.Repeat("0", 56)},
	})

	res, err := FromLog(logPath, "abc12300")
	if err == nil {
		t.Errorf("expected not-found error, got result with SHA %s", res.SHA)
	}
}

func TestFromLogAmbiguousPrefixErrors(t *testing.T) {
	// Two distinct full SHAs sharing a prefix → error, force operator
	// to narrow the prefix.
	logPath := writeLog(t, []*logger.Entry{
		{Tool: "factorly.code",
			Params:    map[string]string{"code": "package main\nfunc Run(p map[string]string) (any, error) { return 1, nil }"},
			SourceSHA: "cafe1111" + strings.Repeat("0", 56)},
		{Tool: "factorly.code",
			Params:    map[string]string{"code": "package main\nfunc Run(p map[string]string) (any, error) { return 2, nil }"},
			SourceSHA: "cafe2222" + strings.Repeat("0", 56)},
	})

	_, err := FromLog(logPath, "cafe")
	if err == nil {
		t.Fatal("expected ambiguity error")
	}
	if !strings.Contains(err.Error(), "ambiguous") {
		t.Errorf("error should say 'ambiguous'; got %v", err)
	}
}

func TestFromLogShortPrefixRejected(t *testing.T) {
	logPath := writeLog(t, []*logger.Entry{
		{Tool: "factorly.code", SourceSHA: "abc" + strings.Repeat("0", 61)},
	})
	if _, err := FromLog(logPath, "abc"); err == nil {
		t.Error("expected error for too-short prefix")
	}
}

func TestFromLogNotFound(t *testing.T) {
	logPath := writeLog(t, []*logger.Entry{
		{Tool: "factorly.code", SourceSHA: "feed" + strings.Repeat("0", 60)},
	})
	if _, err := FromLog(logPath, "babe"); err == nil {
		t.Error("expected not-found error")
	}
}

func TestFromLogHandlesMissingFile(t *testing.T) {
	if _, err := FromLog("/nonexistent/audit.jsonl", "abcd"); err == nil {
		t.Error("expected error for missing log file")
	}
}

func TestFromEntryInfersParameters(t *testing.T) {
	src := "package main\nfunc Run(p map[string]string) (any, error) { return p[\"a\"] + p[\"b\"], nil }"
	entry := &logger.Entry{
		Tool: "factorly.code",
		Params: map[string]string{
			"code":   src,
			"params": `{"a":"hello","b":"world"}`,
		},
	}
	res, err := FromEntry(entry)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Parameters) != 2 {
		t.Fatalf("expected 2 params, got %d", len(res.Parameters))
	}
	// Order must be stable (sorted) to avoid churning YAML diffs.
	if res.Parameters[0].Name != "a" || res.Parameters[1].Name != "b" {
		t.Errorf("params not sorted: %+v", res.Parameters)
	}
}

// TestFromEntryStringifiesNonStringValues confirms that numbers and
// bools in the run's params JSON round-trip into sensible string
// defaults. Without this, json.Unmarshal would produce float64 / bool
// values and the ToParamConfig.Default conversion would silently get
// the wrong string form ("30.000000" instead of "30").
func TestFromEntryStringifiesNonStringValues(t *testing.T) {
	entry := &logger.Entry{
		Tool: "factorly.code",
		Params: map[string]string{
			"code":   "package main\nfunc Run(p map[string]string) (any, error) { return nil, nil }",
			"params": `{"age":30,"verbose":true,"ratio":0.5}`,
		},
	}
	res, err := FromEntry(entry)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, p := range res.Parameters {
		got[p.Name] = p.Default
	}
	if got["age"] != "30" {
		t.Errorf("age default = %q, want \"30\"", got["age"])
	}
	if got["verbose"] != "true" {
		t.Errorf("verbose default = %q, want \"true\"", got["verbose"])
	}
	if got["ratio"] != "0.5" {
		t.Errorf("ratio default = %q, want \"0.5\"", got["ratio"])
	}
}

// TestFromEntryNoParamsField handles the case where the script was
// called without inner params (`{"code":"..."}` with no `params`).
// The inferred parameter list should be empty.
func TestFromEntryNoParamsField(t *testing.T) {
	entry := &logger.Entry{
		Tool: "factorly.code",
		Params: map[string]string{
			"code": "package main\nfunc Run(p map[string]string) (any, error) { return \"hi\", nil }",
		},
	}
	res, err := FromEntry(entry)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Parameters) != 0 {
		t.Errorf("expected no inferred params, got %+v", res.Parameters)
	}
}

// TestFromEntryRejectsMalformedParamsJSON catches a corrupted audit
// entry (e.g. someone hand-edited the log). Better to error than to
// silently lose param data.
func TestFromEntryRejectsMalformedParamsJSON(t *testing.T) {
	entry := &logger.Entry{
		Tool: "factorly.code",
		Params: map[string]string{
			"code":   "package main\nfunc Run(p map[string]string) (any, error) { return nil, nil }",
			"params": `{not valid json`,
		},
	}
	if _, err := FromEntry(entry); err == nil {
		t.Error("expected error for malformed params JSON")
	}
}

func TestFromEntryRejectsNonCodeEntry(t *testing.T) {
	entry := &logger.Entry{Tool: "shell.run", Params: map[string]string{"code": "x"}}
	if _, err := FromEntry(entry); err == nil {
		t.Error("expected error for non-factorly.code entry")
	}
}

func TestFromEntryRejectsMissingCodeParam(t *testing.T) {
	entry := &logger.Entry{Tool: "factorly.code", Params: map[string]string{"who": "x"}}
	if _, err := FromEntry(entry); err == nil {
		t.Error("expected error for missing code param")
	}
}

func TestFromEntryRejectsNil(t *testing.T) {
	if _, err := FromEntry(nil); err == nil {
		t.Error("expected error for nil entry")
	}
}
