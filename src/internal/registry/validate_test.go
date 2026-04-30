// Copyright 2026 Jordan Sherer <hi@jordansherer.com>
// SPDX-License-Identifier: gpl

package registry

import (
	"testing"
)

func ptrFloat(f float64) *float64 { return &f }
func ptrInt(i int) *int           { return &i }

func TestValidateIntegerValid(t *testing.T) {
	tool := &Tool{
		Parameters: []Parameter{{Name: "count", Type: "integer"}},
	}
	params := map[string]string{"count": "42"}
	r := tool.ValidateAndCoerce(params)
	if r.HasErrors() {
		t.Fatalf("unexpected errors: %v", r.Errors)
	}
	if params["count"] != "42" {
		t.Errorf("expected 42, got %s", params["count"])
	}
}

func TestValidateIntegerFromFloat(t *testing.T) {
	tool := &Tool{
		Parameters: []Parameter{{Name: "count", Type: "integer"}},
	}
	params := map[string]string{"count": "3.0"}
	r := tool.ValidateAndCoerce(params)
	if r.HasErrors() {
		t.Fatalf("unexpected errors: %v", r.Errors)
	}
	if params["count"] != "3" {
		t.Errorf("expected 3, got %s", params["count"])
	}
	if !r.WasModified() {
		t.Error("expected modified")
	}
	if r.Modified["count"] != "3.0" {
		t.Errorf("expected original 3.0, got %s", r.Modified["count"])
	}
}

func TestValidateIntegerInvalid(t *testing.T) {
	tool := &Tool{
		Parameters: []Parameter{{Name: "count", Type: "integer"}},
	}
	params := map[string]string{"count": "abc"}
	r := tool.ValidateAndCoerce(params)
	if !r.HasErrors() {
		t.Fatal("expected errors")
	}
}

func TestValidateIntegerMinClamp(t *testing.T) {
	tool := &Tool{
		Parameters: []Parameter{{Name: "page", Type: "integer", Min: ptrFloat(1)}},
	}
	params := map[string]string{"page": "0"}
	r := tool.ValidateAndCoerce(params)
	if r.HasErrors() {
		t.Fatalf("unexpected errors: %v", r.Errors)
	}
	if params["page"] != "1" {
		t.Errorf("expected 1, got %s", params["page"])
	}
}

func TestValidateIntegerMaxClamp(t *testing.T) {
	tool := &Tool{
		Parameters: []Parameter{{Name: "per_page", Type: "integer", Max: ptrFloat(100)}},
	}
	params := map[string]string{"per_page": "500"}
	r := tool.ValidateAndCoerce(params)
	if r.HasErrors() {
		t.Fatalf("unexpected errors: %v", r.Errors)
	}
	if params["per_page"] != "100" {
		t.Errorf("expected 100, got %s", params["per_page"])
	}
}

func TestValidateIntegerInRange(t *testing.T) {
	tool := &Tool{
		Parameters: []Parameter{{Name: "n", Type: "integer", Min: ptrFloat(1), Max: ptrFloat(10)}},
	}
	params := map[string]string{"n": "5"}
	r := tool.ValidateAndCoerce(params)
	if r.HasErrors() || r.WasModified() {
		t.Error("in-range value should pass unchanged")
	}
}

func TestValidateNumberValid(t *testing.T) {
	tool := &Tool{
		Parameters: []Parameter{{Name: "temp", Type: "number"}},
	}
	params := map[string]string{"temp": "0.7"}
	r := tool.ValidateAndCoerce(params)
	if r.HasErrors() {
		t.Fatalf("unexpected errors: %v", r.Errors)
	}
}

func TestValidateNumberInvalid(t *testing.T) {
	tool := &Tool{
		Parameters: []Parameter{{Name: "temp", Type: "number"}},
	}
	params := map[string]string{"temp": "hot"}
	r := tool.ValidateAndCoerce(params)
	if !r.HasErrors() {
		t.Fatal("expected errors")
	}
}

func TestValidateNumberClamp(t *testing.T) {
	tool := &Tool{
		Parameters: []Parameter{{Name: "temp", Type: "number", Min: ptrFloat(0), Max: ptrFloat(2)}},
	}
	params := map[string]string{"temp": "5.0"}
	r := tool.ValidateAndCoerce(params)
	if r.HasErrors() {
		t.Fatalf("unexpected errors: %v", r.Errors)
	}
	if params["temp"] != "2" {
		t.Errorf("expected 2, got %s", params["temp"])
	}
}

func TestValidateBooleanCoerce(t *testing.T) {
	tool := &Tool{
		Parameters: []Parameter{{Name: "dry_run", Type: "boolean"}},
	}
	cases := map[string]string{
		"true": "true", "TRUE": "true", "1": "true", "yes": "true",
		"false": "false", "FALSE": "false", "0": "false", "no": "false",
	}
	for input, expected := range cases {
		params := map[string]string{"dry_run": input}
		r := tool.ValidateAndCoerce(params)
		if r.HasErrors() {
			t.Errorf("input %q: unexpected errors: %v", input, r.Errors)
		}
		if params["dry_run"] != expected {
			t.Errorf("input %q: expected %q, got %q", input, expected, params["dry_run"])
		}
	}
}

func TestValidateBooleanInvalid(t *testing.T) {
	tool := &Tool{
		Parameters: []Parameter{{Name: "flag", Type: "boolean"}},
	}
	params := map[string]string{"flag": "maybe"}
	r := tool.ValidateAndCoerce(params)
	if !r.HasErrors() {
		t.Fatal("expected errors")
	}
}

func TestValidateJSONValid(t *testing.T) {
	tool := &Tool{
		Parameters: []Parameter{{Name: "body", Type: "json"}},
	}
	params := map[string]string{"body": `{"key": "value"}`}
	r := tool.ValidateAndCoerce(params)
	if r.HasErrors() {
		t.Fatalf("unexpected errors: %v", r.Errors)
	}
}

func TestValidateJSONInvalid(t *testing.T) {
	tool := &Tool{
		Parameters: []Parameter{{Name: "body", Type: "json"}},
	}
	params := map[string]string{"body": `{broken`}
	r := tool.ValidateAndCoerce(params)
	if !r.HasErrors() {
		t.Fatal("expected errors")
	}
}

func TestValidateEnum(t *testing.T) {
	tool := &Tool{
		Parameters: []Parameter{{Name: "state", Enum: []string{"open", "closed", "all"}}},
	}

	params := map[string]string{"state": "open"}
	r := tool.ValidateAndCoerce(params)
	if r.HasErrors() {
		t.Fatalf("valid enum should pass: %v", r.Errors)
	}

	params = map[string]string{"state": "invalid"}
	r = tool.ValidateAndCoerce(params)
	if !r.HasErrors() {
		t.Fatal("invalid enum should fail")
	}
}

func TestValidatePattern(t *testing.T) {
	tool := &Tool{
		Parameters: []Parameter{{Name: "owner", Pattern: `^[a-zA-Z0-9-]+$`}},
	}

	params := map[string]string{"owner": "octocat"}
	r := tool.ValidateAndCoerce(params)
	if r.HasErrors() {
		t.Fatalf("valid pattern should pass: %v", r.Errors)
	}

	params = map[string]string{"owner": "octo cat!"}
	r = tool.ValidateAndCoerce(params)
	if !r.HasErrors() {
		t.Fatal("invalid pattern should fail")
	}
}

func TestValidateMaxLengthTruncate(t *testing.T) {
	tool := &Tool{
		Parameters: []Parameter{{Name: "msg", Type: "string", MaxLength: ptrInt(5)}},
	}
	params := map[string]string{"msg": "hello world"}
	r := tool.ValidateAndCoerce(params)
	if r.HasErrors() {
		t.Fatalf("unexpected errors: %v", r.Errors)
	}
	if params["msg"] != "hello" {
		t.Errorf("expected 'hello', got %q", params["msg"])
	}
	if !r.WasModified() {
		t.Error("expected modified")
	}
}

func TestValidateMinLengthReject(t *testing.T) {
	tool := &Tool{
		Parameters: []Parameter{{Name: "code", Type: "string", MinLength: ptrInt(3)}},
	}
	params := map[string]string{"code": "ab"}
	r := tool.ValidateAndCoerce(params)
	if !r.HasErrors() {
		t.Fatal("too-short string should fail")
	}
}

func TestValidateNoRulesPassthrough(t *testing.T) {
	tool := &Tool{
		Parameters: []Parameter{{Name: "anything"}},
	}
	params := map[string]string{"anything": "whatever"}
	r := tool.ValidateAndCoerce(params)
	if r.HasErrors() || r.WasModified() {
		t.Error("no rules should passthrough unchanged")
	}
}

func TestValidateMissingParamSkipped(t *testing.T) {
	tool := &Tool{
		Parameters: []Parameter{{Name: "required_field", Type: "integer", Required: true}},
	}
	params := map[string]string{} // missing
	r := tool.ValidateAndCoerce(params)
	if r.HasErrors() {
		t.Error("missing params should be skipped by validation (required check is separate)")
	}
}

func TestValidateMultipleErrors(t *testing.T) {
	tool := &Tool{
		Parameters: []Parameter{
			{Name: "a", Type: "integer"},
			{Name: "b", Type: "boolean"},
		},
	}
	params := map[string]string{"a": "nope", "b": "maybe"}
	r := tool.ValidateAndCoerce(params)
	if len(r.Errors) != 2 {
		t.Errorf("expected 2 errors, got %d: %v", len(r.Errors), r.Errors)
	}
}

func TestValidateMinZero(t *testing.T) {
	tool := &Tool{
		Parameters: []Parameter{{Name: "n", Type: "integer", Min: ptrFloat(0)}},
	}
	params := map[string]string{"n": "-5"}
	r := tool.ValidateAndCoerce(params)
	if r.HasErrors() {
		t.Fatalf("unexpected errors: %v", r.Errors)
	}
	if params["n"] != "0" {
		t.Errorf("expected 0, got %s", params["n"])
	}
}

func TestValidateNumberMinClamp(t *testing.T) {
	tool := &Tool{
		Parameters: []Parameter{{Name: "temp", Type: "number", Min: ptrFloat(0)}},
	}
	params := map[string]string{"temp": "-0.5"}
	r := tool.ValidateAndCoerce(params)
	if r.HasErrors() {
		t.Fatalf("unexpected errors: %v", r.Errors)
	}
	if params["temp"] != "0" {
		t.Errorf("expected 0, got %s", params["temp"])
	}
}

func TestValidateIntegerBothMinMax(t *testing.T) {
	tool := &Tool{
		Parameters: []Parameter{{Name: "n", Type: "integer", Min: ptrFloat(1), Max: ptrFloat(100)}},
	}
	// Below min
	params := map[string]string{"n": "-10"}
	r := tool.ValidateAndCoerce(params)
	if r.HasErrors() {
		t.Fatalf("unexpected errors: %v", r.Errors)
	}
	if params["n"] != "1" {
		t.Errorf("expected 1, got %s", params["n"])
	}
	// Above max
	params = map[string]string{"n": "999"}
	r = tool.ValidateAndCoerce(params)
	if r.HasErrors() {
		t.Fatalf("unexpected errors: %v", r.Errors)
	}
	if params["n"] != "100" {
		t.Errorf("expected 100, got %s", params["n"])
	}
}

func TestValidateEmptyStringWithType(t *testing.T) {
	tool := &Tool{
		Parameters: []Parameter{{Name: "val", Type: "integer"}},
	}
	params := map[string]string{"val": ""}
	r := tool.ValidateAndCoerce(params)
	if !r.HasErrors() {
		t.Fatal("empty string for integer should fail")
	}
}

func TestValidateExtraParamsPassthrough(t *testing.T) {
	tool := &Tool{
		Parameters: []Parameter{{Name: "known", Type: "integer"}},
	}
	params := map[string]string{"known": "5", "extra": "anything"}
	r := tool.ValidateAndCoerce(params)
	if r.HasErrors() {
		t.Fatalf("unexpected errors: %v", r.Errors)
	}
	if params["extra"] != "anything" {
		t.Error("extra params should be untouched")
	}
}

func TestValidateMaxLengthExactBoundary(t *testing.T) {
	tool := &Tool{
		Parameters: []Parameter{{Name: "msg", Type: "string", MaxLength: ptrInt(5)}},
	}
	params := map[string]string{"msg": "hello"}
	r := tool.ValidateAndCoerce(params)
	if r.HasErrors() || r.WasModified() {
		t.Error("exact boundary should pass unchanged")
	}
}

func TestValidateEmptyEnumPassthrough(t *testing.T) {
	tool := &Tool{
		Parameters: []Parameter{{Name: "x", Enum: []string{}}},
	}
	params := map[string]string{"x": "anything"}
	r := tool.ValidateAndCoerce(params)
	if r.HasErrors() {
		t.Error("empty enum list should passthrough")
	}
}

func TestValidateIntegerNegativeMax(t *testing.T) {
	// Max can be negative
	tool := &Tool{
		Parameters: []Parameter{{Name: "n", Type: "integer", Max: ptrFloat(-1)}},
	}
	params := map[string]string{"n": "5"}
	r := tool.ValidateAndCoerce(params)
	if r.HasErrors() {
		t.Fatalf("unexpected errors: %v", r.Errors)
	}
	if params["n"] != "-1" {
		t.Errorf("expected -1, got %s", params["n"])
	}
}

func TestValidateCoercionPreservesOriginal(t *testing.T) {
	tool := &Tool{
		Parameters: []Parameter{
			{Name: "a", Type: "integer", Max: ptrFloat(10)},
			{Name: "b", Type: "boolean"},
		},
	}
	params := map[string]string{"a": "50", "b": "YES"}
	r := tool.ValidateAndCoerce(params)
	if r.HasErrors() {
		t.Fatalf("unexpected errors: %v", r.Errors)
	}
	if params["a"] != "10" || params["b"] != "true" {
		t.Errorf("coercion failed: a=%s b=%s", params["a"], params["b"])
	}
	if r.Modified["a"] != "50" || r.Modified["b"] != "YES" {
		t.Errorf("originals not preserved: %v", r.Modified)
	}
}
