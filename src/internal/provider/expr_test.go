// Copyright 2026 Jordan Sherer <hi@jordansherer.com>
// SPDX-License-Identifier: gpl

package provider

import (
	"fmt"
	"testing"
)

func TestExprStringEquality(t *testing.T) {
	vars := map[string]string{"status": "healthy"}
	if !EvalCondition("status == 'healthy'", vars) {
		t.Error("expected true")
	}
	if EvalCondition("status == 'degraded'", vars) {
		t.Error("expected false")
	}
}

func TestExprStringInequality(t *testing.T) {
	vars := map[string]string{"output": "hello"}
	if !EvalCondition("output != ''", vars) {
		t.Error("non-empty should be != ''")
	}
	vars["output"] = ""
	if EvalCondition("output != ''", vars) {
		t.Error("empty should not be != ''")
	}
}

func TestExprNumericComparison(t *testing.T) {
	vars := map[string]string{"count": "15"}
	if !EvalCondition("count > 10", vars) {
		t.Error("15 > 10 should be true")
	}
	if EvalCondition("count > 20", vars) {
		t.Error("15 > 20 should be false")
	}
	if !EvalCondition("count <= 15", vars) {
		t.Error("15 <= 15 should be true")
	}
}

func TestExprBooleanLiterals(t *testing.T) {
	vars := map[string]string{}
	if !EvalCondition("true", vars) {
		t.Error("true should be true")
	}
	if EvalCondition("false", vars) {
		t.Error("false should be false")
	}
}

func TestExprNot(t *testing.T) {
	vars := map[string]string{"failed": ""}
	if !EvalCondition("not failed", vars) {
		t.Error("not empty-string should be true")
	}
	vars["failed"] = "yes"
	if EvalCondition("not failed", vars) {
		t.Error("not 'yes' should be false")
	}
}

func TestExprAnd(t *testing.T) {
	vars := map[string]string{"a": "1", "b": "2"}
	if !EvalCondition("a == '1' and b == '2'", vars) {
		t.Error("both true should be true")
	}
	if EvalCondition("a == '1' and b == '3'", vars) {
		t.Error("one false should be false")
	}
}

func TestExprOr(t *testing.T) {
	vars := map[string]string{"a": "1", "b": "2"}
	if !EvalCondition("a == '9' or b == '2'", vars) {
		t.Error("one true should be true")
	}
	if EvalCondition("a == '9' or b == '9'", vars) {
		t.Error("both false should be false")
	}
}

func TestExprMemberAccess(t *testing.T) {
	vars := map[string]string{"result": `{"code": 200, "status": "ok"}`}
	if !EvalCondition("result.code == '200'", vars) {
		t.Error("member access should resolve JSON field")
	}
	if !EvalCondition("result.status == 'ok'", vars) {
		t.Error("string member should resolve")
	}
}

func TestExprContains(t *testing.T) {
	vars := map[string]string{"output": "build failed with error"}
	if !EvalCondition("contains(output, 'error')", vars) {
		t.Error("should contain 'error'")
	}
	if EvalCondition("contains(output, 'success')", vars) {
		t.Error("should not contain 'success'")
	}
}

func TestExprEmptyVariable(t *testing.T) {
	vars := map[string]string{}
	// Missing variable is empty string, which is falsy
	if EvalCondition("missing", vars) {
		t.Error("missing variable should be falsy")
	}
}

func TestExprTruthiness(t *testing.T) {
	vars := map[string]string{}
	vars["a"] = ""
	if EvalCondition("a", vars) {
		t.Error("empty string should be falsy")
	}
	vars["a"] = "0"
	if EvalCondition("a", vars) {
		t.Error("'0' should be falsy")
	}
	vars["a"] = "false"
	if EvalCondition("a", vars) {
		t.Error("'false' should be falsy")
	}
	vars["a"] = "hello"
	if !EvalCondition("a", vars) {
		t.Error("non-empty string should be truthy")
	}
}

func TestExprDoubleQuoteStrings(t *testing.T) {
	vars := map[string]string{"name": "world"}
	if !EvalCondition(`name == "world"`, vars) {
		t.Error("double-quoted string should work")
	}
}

func TestExprParentheses(t *testing.T) {
	vars := map[string]string{"a": "1", "b": "2", "c": "3"}
	if !EvalCondition("(a == '1' or b == '9') and c == '3'", vars) {
		t.Error("parenthesized or should group correctly")
	}
	if EvalCondition("a == '1' and (b == '9' or c == '9')", vars) {
		t.Error("both inner false should make and false")
	}
}

func TestExprEmptyExpression(t *testing.T) {
	if EvalCondition("", map[string]string{}) {
		t.Error("empty expression should be false")
	}
}

// --- Complex / compound expressions ---

func TestExprComplexAndOrChain(t *testing.T) {
	vars := map[string]string{"status": "degraded", "retries": "3", "env": "prod"}

	// All three conditions true
	if !EvalCondition("status == 'degraded' and retries > 2 and env == 'prod'", vars) {
		t.Error("triple and with all true should be true")
	}
	// One false in the chain
	if EvalCondition("status == 'degraded' and retries > 5 and env == 'prod'", vars) {
		t.Error("triple and with one false should be false")
	}
	// Or takes precedence over surrounding context
	if !EvalCondition("status == 'healthy' or status == 'degraded'", vars) {
		t.Error("or with one true should be true")
	}
	// Mixed and/or: and binds tighter than or
	// "false and true or true" → "(false and true) or true" → true
	if !EvalCondition("status == 'healthy' and env == 'prod' or retries > 2", vars) {
		t.Error("and binds tighter than or: false-and-true or true = true")
	}
}

func TestExprNestedParentheses(t *testing.T) {
	vars := map[string]string{"a": "1", "b": "2", "c": "3", "d": "4"}

	if !EvalCondition("(a == '1' and b == '2') and (c == '3' and d == '4')", vars) {
		t.Error("nested groups all true should be true")
	}
	if EvalCondition("(a == '1' and b == '9') and (c == '3' and d == '4')", vars) {
		t.Error("first group false should make whole false")
	}
	// Deeply nested
	if !EvalCondition("((a == '1') and (b == '2' or c == '9'))", vars) {
		t.Error("deep nesting should work")
	}
}

func TestExprNotWithComparison(t *testing.T) {
	vars := map[string]string{"code": "404"}

	// `not` binds tighter than `==`, so `not code == '200'` means `(not code) == '200'`
	// which is `false == '200'` → false. Use parens for intended behavior:
	if !EvalCondition("not (code == '200')", vars) {
		t.Error("not (code==200) with code=404 should be true")
	}
	if EvalCondition("not (code == '404')", vars) {
		t.Error("not (code==404) with code=404 should be false")
	}
	// Without parens: `not code` evaluates truthiness of code
	if EvalCondition("not code", vars) {
		t.Error("not '404' (truthy) should be false")
	}
}

func TestExprNotWithAnd(t *testing.T) {
	vars := map[string]string{"enabled": "true", "locked": ""}

	if !EvalCondition("enabled and not locked", vars) {
		t.Error("enabled=truthy and not locked=empty should be true")
	}
	vars["locked"] = "yes"
	if EvalCondition("enabled and not locked", vars) {
		t.Error("enabled=truthy and not locked=yes should be false")
	}
}

func TestExprMemberAccessNested(t *testing.T) {
	vars := map[string]string{
		"response": `{"data": {"items": [1,2,3], "count": 3}, "status": "ok"}`,
	}

	if !EvalCondition("response.status == 'ok'", vars) {
		t.Error("top-level member should resolve")
	}
	// Deep nested access works: JSON parsed once at top level, then
	// map[string]any access chains through nested objects.
	if !EvalCondition("response.data.count == '3'", vars) {
		t.Error("deep dot notation should resolve through nested maps")
	}
}

func TestExprMemberAccessMissing(t *testing.T) {
	vars := map[string]string{"result": `{"code": 200}`}

	// Access a field that doesn't exist in the JSON
	if EvalCondition("result.nonexistent == 'something'", vars) {
		t.Error("missing field should not match")
	}
	// Compare missing field to empty — missing returns nil, toString(nil) = ""
	if !EvalCondition("result.nonexistent == ''", vars) {
		t.Error("missing field should equal empty string")
	}
}

func TestExprMemberAccessNonJSON(t *testing.T) {
	vars := map[string]string{"plain": "just a string"}

	// Trying member access on non-JSON returns nil
	if EvalCondition("plain.field == 'something'", vars) {
		t.Error("member access on non-JSON should not match")
	}
}

func TestExprContainsWithVariables(t *testing.T) {
	vars := map[string]string{
		"log":    "ERROR: connection refused at 10.0.0.1:5432",
		"search": "connection refused",
	}

	if !EvalCondition("contains(log, 'ERROR')", vars) {
		t.Error("should find ERROR in log")
	}
	if !EvalCondition("contains(log, search)", vars) {
		t.Error("should find variable value in log")
	}
	if EvalCondition("contains(log, 'SUCCESS')", vars) {
		t.Error("should not find SUCCESS")
	}
}

func TestExprContainsEmpty(t *testing.T) {
	vars := map[string]string{"text": "hello"}

	// Contains with empty needle is always true (Go strings.Contains behavior)
	if !EvalCondition("contains(text, '')", vars) {
		t.Error("contains with empty needle should be true")
	}
	// Contains on empty haystack
	vars["text"] = ""
	if EvalCondition("contains(text, 'x')", vars) {
		t.Error("contains on empty haystack should be false")
	}
}

func TestExprNumericEdgeCases(t *testing.T) {
	vars := map[string]string{"val": "0", "neg": "-5", "float": "3.14"}

	if EvalCondition("val > 0", vars) {
		t.Error("0 > 0 should be false")
	}
	if !EvalCondition("val >= 0", vars) {
		t.Error("0 >= 0 should be true")
	}
	if !EvalCondition("val == '0'", vars) {
		t.Error("string comparison with '0' should work")
	}
	// Negative numbers in variables
	if !EvalCondition("neg < 0", vars) {
		t.Error("-5 < 0 should be true")
	}
	// Float comparison
	if !EvalCondition("float > 3", vars) {
		t.Error("3.14 > 3 should be true")
	}
	if EvalCondition("float > 4", vars) {
		t.Error("3.14 > 4 should be false")
	}
}

func TestExprCompareVariableToVariable(t *testing.T) {
	vars := map[string]string{"expected": "200", "actual": "200"}

	if !EvalCondition("expected == actual", vars) {
		t.Error("same value variables should be equal")
	}
	vars["actual"] = "404"
	if EvalCondition("expected == actual", vars) {
		t.Error("different value variables should not be equal")
	}
}

func TestExprCompareNumberLiterals(t *testing.T) {
	vars := map[string]string{"code": "200"}

	// Compare to number literal (both sides become floats for comparison)
	if !EvalCondition("code == 200", vars) {
		t.Error("string '200' should equal number 200 via toComparable")
	}
}

func TestExprWhitespaceHandling(t *testing.T) {
	vars := map[string]string{"x": "1"}

	// Extra whitespace shouldn't matter
	if !EvalCondition("  x  ==  '1'  ", vars) {
		t.Error("extra whitespace should be ignored")
	}
	if !EvalCondition("x=='1'", vars) {
		t.Error("no whitespace should also work")
	}
}

func TestExprSpecialCharsInStrings(t *testing.T) {
	vars := map[string]string{"path": "/usr/local/bin"}

	if !EvalCondition("path == '/usr/local/bin'", vars) {
		t.Error("slashes in strings should work")
	}

	vars["msg"] = "it's working"
	if !EvalCondition(`msg == "it's working"`, vars) {
		t.Error("single quote inside double-quoted string should work")
	}
}

func TestExprMultipleContainsCombined(t *testing.T) {
	vars := map[string]string{"output": "tests passed: 42 ok, 0 failed"}

	if !EvalCondition("contains(output, 'passed') and not contains(output, 'error')", vars) {
		t.Error("passed without error should be true")
	}
	if EvalCondition("contains(output, 'passed') and contains(output, 'error')", vars) {
		t.Error("passed with error check should be false")
	}
}

func TestExprDeeplyNestedParens(t *testing.T) {
	vars := map[string]string{"a": "1", "b": "2", "c": "3", "d": "4", "e": "5"}

	// Three levels deep
	if !EvalCondition("((a == '1' and b == '2') or (c == '9')) and (d == '4')", vars) {
		t.Error("3-level nesting should work")
	}
	// Four levels
	if !EvalCondition("(((a == '1')))", vars) {
		t.Error("redundant nesting should still work")
	}
	// Complex nested with mixed operators
	if !EvalCondition("(a == '1' and (b == '2' or (c == '3' and d == '4'))) and e == '5'", vars) {
		t.Error("deeply mixed nesting should resolve correctly")
	}
	// Nested not
	if !EvalCondition("not (not (a == '1'))", vars) {
		t.Error("double not should cancel out")
	}
	if EvalCondition("not (not (a == '9'))", vars) {
		t.Error("double not of false should be false")
	}
}

func TestExprNestedNotWithBooleanOps(t *testing.T) {
	vars := map[string]string{"x": "yes", "y": ""}

	// not inside and/or
	if !EvalCondition("x and not y", vars) {
		t.Error("truthy and not falsy should be true")
	}
	if !EvalCondition("not y and x", vars) {
		t.Error("not falsy and truthy should be true")
	}
	if EvalCondition("not x and not y", vars) {
		t.Error("not truthy and not falsy: false and true = false")
	}
	if !EvalCondition("not x or not y", vars) {
		t.Error("not truthy or not falsy: false or true = true")
	}
}

func TestExprUnclosedString(t *testing.T) {
	vars := map[string]string{"x": "hello"}
	// Unclosed string — tokenizer should handle gracefully
	// Should not panic, returns false (fail-closed)
	_ = EvalCondition("x == 'unclosed", vars)
	// If we get here without panic, it's fine
}

func TestExprGarbageInput(t *testing.T) {
	vars := map[string]string{}
	// Various malformed expressions — none should panic
	cases := []string{
		"==",
		"and",
		"or or or",
		"((((",
		"))))",
		"== == ==",
		"not not not",
		"contains()",
		"contains(x)",
		"...",
		"a.b.c.d.e",
		"'' == ''",
	}
	for _, expr := range cases {
		// Should not panic
		_ = EvalCondition(expr, vars)
	}
}

func TestExprOnlyOperators(t *testing.T) {
	vars := map[string]string{}
	// Degenerate: just an operator
	if EvalCondition("==", vars) {
		t.Error("bare operator should be false")
	}
	if EvalCondition("> 5", vars) {
		t.Error("operator without left should be false")
	}
}

func TestExprVeryLongExpression(t *testing.T) {
	vars := map[string]string{}
	for i := 0; i < 20; i++ {
		vars[fmt.Sprintf("v%d", i)] = fmt.Sprintf("%d", i)
	}
	// 20-variable chain
	expr := "v0 == '0' and v1 == '1' and v2 == '2' and v3 == '3' and v4 == '4' and v5 == '5' and v6 == '6' and v7 == '7' and v8 == '8' and v9 == '9'"
	if !EvalCondition(expr, vars) {
		t.Error("long and-chain should work")
	}
	// One wrong value breaks it
	vars["v5"] = "wrong"
	if EvalCondition(expr, vars) {
		t.Error("long chain with one wrong should fail")
	}
}

func TestExprUnicodeInStrings(t *testing.T) {
	vars := map[string]string{"msg": "café ☕"}
	if !EvalCondition("msg == 'café ☕'", vars) {
		t.Error("unicode in string comparison should work")
	}
	if !EvalCondition("contains(msg, '☕')", vars) {
		t.Error("unicode in contains should work")
	}
}

func TestExprNewlinesInVariables(t *testing.T) {
	vars := map[string]string{"output": "line1\nline2\nline3"}
	if !EvalCondition("contains(output, 'line2')", vars) {
		t.Error("multiline variable should work with contains")
	}
	if !EvalCondition("output != ''", vars) {
		t.Error("multiline variable should be non-empty")
	}
}

func TestExprEmptyStringLiteralComparison(t *testing.T) {
	vars := map[string]string{"x": "", "y": "something"}
	if !EvalCondition("x == ''", vars) {
		t.Error("empty var should equal empty string literal")
	}
	if EvalCondition("y == ''", vars) {
		t.Error("non-empty var should not equal empty string")
	}
}

func TestExprBareVariableTruthiness(t *testing.T) {
	// This is the `if: "x"` pattern — just a variable name, no operator
	cases := []struct {
		name  string
		val   string
		truth bool
	}{
		{"non-empty string", "hello", true},
		{"empty string", "", false},
		{"zero string", "0", false},
		{"false string", "false", false},
		{"true string", "true", true},
		{"whitespace", " ", true}, // space is truthy (non-empty, not "0"/"false")
		{"number string", "42", true},
		{"negative number", "-1", true},
		{"json object", `{"key":"val"}`, true},
		{"newline only", "\n", true},  // non-empty
		{"null string", "null", true}, // not one of the falsy strings
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			vars := map[string]string{"x": tc.val}
			got := EvalCondition("x", vars)
			if got != tc.truth {
				t.Errorf("EvalCondition(\"x\") with x=%q: got %v, want %v", tc.val, got, tc.truth)
			}
		})
	}
}

func TestExprBareVariableMissing(t *testing.T) {
	// Variable not in map at all — should be falsy
	vars := map[string]string{"other": "exists"}
	if EvalCondition("missing", vars) {
		t.Error("undefined variable should be falsy")
	}
}

func TestExprBareTrue(t *testing.T) {
	// Literal "true" without any variable — always true (used as switch default)
	if !EvalCondition("true", map[string]string{}) {
		t.Error("bare 'true' should always be true")
	}
}

func TestExprBareFalse(t *testing.T) {
	if EvalCondition("false", map[string]string{}) {
		t.Error("bare 'false' should always be false")
	}
}

func TestExprContainsWithLiterals(t *testing.T) {
	vars := map[string]string{}
	// Both args as literals
	if !EvalCondition("contains('hello world', 'world')", vars) {
		t.Error("contains with two string literals should work")
	}
	if EvalCondition("contains('hello', 'xyz')", vars) {
		t.Error("contains with non-matching literals should be false")
	}
}

func TestExprChainedComparisons(t *testing.T) {
	vars := map[string]string{"x": "5"}
	// x > 3 and x < 10 (range check)
	if !EvalCondition("x > 3 and x < 10", vars) {
		t.Error("range check 3 < 5 < 10 should be true")
	}
	if EvalCondition("x > 3 and x < 4", vars) {
		t.Error("range check 3 < 5 < 4 should be false")
	}
}

func TestExprJSONPathBasic(t *testing.T) {
	vars := map[string]string{
		"response": `{"status": "ok", "code": 200, "data": {"count": 5}}`,
	}

	// Top-level string field
	if !EvalCondition("jsonpath(response, '$.status') == 'ok'", vars) {
		t.Error("should extract status field")
	}
	// Top-level number field
	if !EvalCondition("jsonpath(response, '$.code') == '200'", vars) {
		t.Error("should extract code field as comparable")
	}
	// Nested field
	if !EvalCondition("jsonpath(response, '$.data.count') == '5'", vars) {
		t.Error("should extract nested field")
	}
}

func TestExprJSONPathArray(t *testing.T) {
	vars := map[string]string{
		"data": `{"users": [{"name": "alice", "active": true}, {"name": "bob", "active": false}]}`,
	}

	// Array index
	if !EvalCondition("jsonpath(data, '$.users[0].name') == 'alice'", vars) {
		t.Error("should extract array element field")
	}
	if !EvalCondition("jsonpath(data, '$.users[1].name') == 'bob'", vars) {
		t.Error("should extract second array element")
	}
	// Boolean field
	if !EvalCondition("jsonpath(data, '$.users[0].active') == 'true'", vars) {
		t.Error("should extract boolean as string")
	}
}

func TestExprJSONPathTruthiness(t *testing.T) {
	vars := map[string]string{
		"response": `{"items": [1, 2, 3], "empty": [], "nil_field": null}`,
	}

	// Non-empty result is truthy
	if !EvalCondition("jsonpath(response, '$.items[0]')", vars) {
		t.Error("existing element should be truthy")
	}
	// Null field
	if EvalCondition("jsonpath(response, '$.nil_field')", vars) {
		t.Error("null field should be falsy")
	}
	// Non-existent path
	if EvalCondition("jsonpath(response, '$.nonexistent')", vars) {
		t.Error("missing path should be falsy")
	}
}

func TestExprJSONPathInvalidJSON(t *testing.T) {
	vars := map[string]string{"broken": "not json at all"}

	// Should fail gracefully (return nil → falsy)
	if EvalCondition("jsonpath(broken, '$.field')", vars) {
		t.Error("invalid JSON should be falsy")
	}
}

func TestExprJSONPathInvalidExpression(t *testing.T) {
	vars := map[string]string{"data": `{"x": 1}`}

	// Bad JSONPath expression — should fail gracefully
	if EvalCondition("jsonpath(data, 'not a path[')", vars) {
		t.Error("invalid jsonpath expr should be falsy")
	}
}

func TestExprJSONPathWithContains(t *testing.T) {
	vars := map[string]string{
		"response": `{"message": "deployment successful to prod-us-east-1"}`,
	}

	// Combine jsonpath extraction with contains
	if !EvalCondition("contains(jsonpath(response, '$.message'), 'successful')", vars) {
		t.Error("should find 'successful' in extracted message")
	}
	if EvalCondition("contains(jsonpath(response, '$.message'), 'failed')", vars) {
		t.Error("should not find 'failed'")
	}
}

func TestExprJSONPathMultipleResults(t *testing.T) {
	vars := map[string]string{
		"data": `{"items": [{"id": 1}, {"id": 2}, {"id": 3}]}`,
	}

	// Wildcard returns multiple results as JSON array string
	result := EvalCondition("jsonpath(data, '$.items[*].id') != ''", vars)
	if !result {
		t.Error("multiple results should produce non-empty string")
	}
}

func TestExprJSONPathComparison(t *testing.T) {
	vars := map[string]string{
		"metrics": `{"cpu": 85, "memory": 60, "disk": 45}`,
	}

	// Numeric comparison on extracted value
	if !EvalCondition("jsonpath(metrics, '$.cpu') > 80", vars) {
		t.Error("cpu 85 > 80 should be true")
	}
	if EvalCondition("jsonpath(metrics, '$.memory') > 80", vars) {
		t.Error("memory 60 > 80 should be false")
	}
}

func TestExprRealWorldConditions(t *testing.T) {
	// Simulate real workflow conditions

	// Git: check if there are changes
	vars := map[string]string{"changes": "M  src/main.go\nM  go.mod\n"}
	if !EvalCondition("changes != ''", vars) {
		t.Error("non-empty git changes should trigger commit")
	}

	// HTTP: check response code
	vars = map[string]string{"response": `{"status_code": 200, "body": "ok"}`}
	if !EvalCondition("response.status_code == '200'", vars) {
		t.Error("200 response should be healthy")
	}

	// CI: check test output
	vars = map[string]string{"test_output": "PASS\nok  github.com/example 0.5s"}
	if !EvalCondition("contains(test_output, 'PASS') and not contains(test_output, 'FAIL')", vars) {
		t.Error("PASS without FAIL should be passing")
	}

	// Deploy: multi-condition gate
	vars = map[string]string{"branch": "main", "tests": "pass", "approval": "yes"}
	if !EvalCondition("branch == 'main' and tests == 'pass' and approval == 'yes'", vars) {
		t.Error("all gates satisfied should allow deploy")
	}
	vars["approval"] = ""
	if EvalCondition("branch == 'main' and tests == 'pass' and approval == 'yes'", vars) {
		t.Error("missing approval should block deploy")
	}
}
