// Copyright 2026 Jordan Sherer <hi@jordansherer.com>
// SPDX-License-Identifier: gpl

// Package promote recovers a previously-run factorly.code script
// from the audit log and converts it into a registerable `type:
// code` tool definition. Used by the CLI's `factorly tools promote`
// command and the UI's "Save as tool" button on the /history page.
//
// The audit log is the source of truth: every factorly.code call
// records the full source under params["code"] plus a source_sha
// hash. Promote addresses an entry by SHA prefix (so an operator
// can grab "that script the agent just ran" without copying a long
// hash), recovers the source from the params payload, and infers a
// parameter schema from the OTHER keys in the same params payload —
// using the values the script was actually run with as defaults.
//
// Compile-validation is the caller's responsibility (call
// codeprov.Validate on Result.Source before writing). Persistence
// is also the caller's job; this package only does recovery +
// shaping.
package promote

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/factorly-dev/factorly/internal/config"
	"github.com/factorly-dev/factorly/internal/logger"
)

// shaPrefixMin is the minimum SHA prefix length accepted by FromLog.
// Shorter prefixes risk collisions in a busy audit log; 4 hex chars
// give 16 bits of distinguishability, which is enough at human scale
// while staying easy to type from a recent log view.
const shaPrefixMin = 4

// Result is the recovered representation of a single factorly.code
// run, shaped for downstream tool-config construction.
type Result struct {
	// Source is the full Go script body (the `code` param of the run).
	Source string
	// SHA is the full SHA-256 (hex) of Source. Matches the entry's
	// source_sha so callers can include it in audit trails.
	SHA string
	// Parameters is the inferred schema: one ParamConfig per non-`code`
	// key in the original run's params. Defaults are the actual values
	// the script ran with; types default to string. Operators refine
	// in the UI edit page.
	Parameters []config.ParamConfig
	// OriginalRun is the audit entry the recovery came from. Lets
	// callers print "promoted from run on <timestamp>" boilerplate.
	OriginalRun *logger.Entry
}

// FromLog scans the JSONL audit log at logPath for factorly.code
// entries whose source_sha begins with shaPrefix and returns the
// most recent match. Ambiguity (multiple matches with different
// full SHAs) is an error — the operator should narrow the prefix.
//
// Memory note: the buffered scanner is sized for very long lines
// because code params can be several KB; the default scanner would
// silently skip them.
func FromLog(logPath, shaPrefix string) (*Result, error) {
	if len(shaPrefix) < shaPrefixMin {
		return nil, fmt.Errorf("sha prefix %q is too short (need at least %d hex chars)", shaPrefix, shaPrefixMin)
	}
	shaPrefix = strings.ToLower(shaPrefix)

	f, err := os.Open(logPath) // #nosec G304 -- logPath is an explicit operator-supplied path
	if err != nil {
		return nil, fmt.Errorf("opening audit log %q: %w", logPath, err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	// 1MB max line: code params can be sizeable. Mirrors the buffer
	// size factorly logs uses (logs_cmd.go), just bigger to account
	// for embedded scripts.
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	var (
		bestEntry   *logger.Entry
		matchedSHAs = map[string]bool{}
	)
	for scanner.Scan() {
		var e logger.Entry
		if err := json.Unmarshal(scanner.Bytes(), &e); err != nil {
			continue // skip malformed lines silently, same as factorly logs
		}
		if e.Tool != "factorly.code" {
			continue
		}
		if !strings.HasPrefix(strings.ToLower(e.SourceSHA), shaPrefix) {
			continue
		}
		matchedSHAs[e.SourceSHA] = true
		entry := e // copy out of the loop-local
		bestEntry = &entry
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scanning audit log: %w", err)
	}

	if bestEntry == nil {
		return nil, fmt.Errorf("no factorly.code entry found with source_sha prefix %q", shaPrefix)
	}
	if len(matchedSHAs) > 1 {
		// Sort for a deterministic error message.
		shas := make([]string, 0, len(matchedSHAs))
		for s := range matchedSHAs {
			shas = append(shas, s[:12]+"…")
		}
		sort.Strings(shas)
		return nil, fmt.Errorf("sha prefix %q is ambiguous: matches %d distinct scripts (%s) — use a longer prefix",
			shaPrefix, len(matchedSHAs), strings.Join(shas, ", "))
	}

	return FromEntry(bestEntry)
}

// FromEntry builds a Result from an already-recovered audit entry.
// Used by the UI handler, which has the entry in hand and doesn't
// need to re-scan the log.
//
// Errors when the entry isn't a factorly.code call or when the
// source param is missing — both signal the caller addressed the
// wrong entry.
//
// Parameter inference: the factorly.code builtin's audit entry has
// two top-level params — `code` (the script) and `params` (a JSON
// string holding the inner params the script was invoked with).
// We parse that JSON to recover the inner key/value pairs and use
// each as a ParamConfig in the promoted tool, with the run-time
// value as the default.
func FromEntry(entry *logger.Entry) (*Result, error) {
	if entry == nil {
		return nil, fmt.Errorf("nil audit entry")
	}
	if entry.Tool != "factorly.code" {
		return nil, fmt.Errorf("entry is for tool %q, not factorly.code", entry.Tool)
	}
	src := entry.Params["code"]
	if src == "" {
		return nil, fmt.Errorf("entry has no `code` param — cannot recover source")
	}

	// Parse the inner params (JSON string). Missing or empty is fine —
	// the script was called with no inner params, and the promoted tool
	// will have no inferred parameters.
	inner := map[string]string{}
	if raw := entry.Params["params"]; raw != "" {
		// Inner params may be string-only (the canonical factorly.code
		// shape) or arbitrary JSON values. Decode into a generic map
		// first, then stringify each value so the inferred defaults
		// always land as strings (ParamConfig.Default is a string).
		var generic map[string]any
		if err := json.Unmarshal([]byte(raw), &generic); err != nil {
			return nil, fmt.Errorf("entry `params` is not valid JSON: %w", err)
		}
		for k, v := range generic {
			inner[k] = stringifyValue(v)
		}
	}

	// Sorted for stability across runs (otherwise the inferred YAML
	// would shuffle order on re-promote, churning git diffs).
	paramNames := make([]string, 0, len(inner))
	for k := range inner {
		paramNames = append(paramNames, k)
	}
	sort.Strings(paramNames)

	params := make([]config.ParamConfig, 0, len(paramNames))
	for _, name := range paramNames {
		params = append(params, config.ParamConfig{
			Name:    name,
			Type:    "string", // factorly.code's params map is always map[string]string
			Default: inner[name],
		})
	}

	return &Result{
		Source:      src,
		SHA:         entry.SourceSHA,
		Parameters:  params,
		OriginalRun: entry,
	}, nil
}

// stringifyValue collapses a JSON-decoded value back to a string.
// Numbers become their decimal representation, bools become "true"/
// "false", strings pass through, anything else (arrays, nested
// objects) becomes a JSON re-encoding so the operator can see what
// was passed and decide whether to refine the schema by hand.
func stringifyValue(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case bool:
		if x {
			return "true"
		}
		return "false"
	case float64:
		// json.Unmarshal turns all numbers into float64. Trim trailing
		// zeros for ints so "30" doesn't show up as "30.000000".
		if x == float64(int64(x)) {
			return fmt.Sprintf("%d", int64(x))
		}
		return fmt.Sprintf("%g", x)
	case nil:
		return ""
	default:
		b, _ := json.Marshal(v)
		return string(b)
	}
}
