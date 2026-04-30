// Copyright 2026 Jordan Sherer <hi@jordansherer.com>
// SPDX-License-Identifier: gpl

package registry

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// ValidationResult holds the outcome of parameter validation.
type ValidationResult struct {
	Modified map[string]string // param name → original value (pre-coercion)
	Errors   []string          // validation errors (non-coercible)
}

// HasErrors returns true if any parameter failed validation.
func (r *ValidationResult) HasErrors() bool { return len(r.Errors) > 0 }

// WasModified returns true if any parameter was coerced.
func (r *ValidationResult) WasModified() bool { return len(r.Modified) > 0 }

// ValidateAndCoerce checks all parameters against their validation rules.
// It modifies params in-place where coercion is possible.
func (t *Tool) ValidateAndCoerce(params map[string]string) *ValidationResult {
	result := &ValidationResult{
		Modified: make(map[string]string),
	}
	for _, pd := range t.Parameters {
		val, exists := params[pd.Name]
		if !exists {
			continue
		}
		if !hasValidation(pd) {
			continue
		}
		newVal, errs := validateParam(pd, val)
		if len(errs) > 0 {
			for _, e := range errs {
				result.Errors = append(result.Errors, fmt.Sprintf("%s: %s", pd.Name, e))
			}
			continue
		}
		if newVal != val {
			result.Modified[pd.Name] = val
			params[pd.Name] = newVal
		}
	}
	return result
}

// hasValidation returns true if the parameter has any validation rules configured.
func hasValidation(pd Parameter) bool {
	return pd.Type != "" || pd.Min != nil || pd.Max != nil ||
		pd.MinLength != nil || pd.MaxLength != nil ||
		pd.Pattern != "" || len(pd.Enum) > 0
}

func validateParam(pd Parameter, val string) (coerced string, errs []string) {
	coerced = val

	switch pd.Type {
	case "integer":
		n, err := strconv.ParseInt(val, 10, 64)
		if err != nil {
			// Try float → int (e.g., "3.0" → 3)
			f, ferr := strconv.ParseFloat(val, 64)
			if ferr != nil {
				return val, []string{fmt.Sprintf("expected integer, got %q", val)}
			}
			n = int64(f)
			coerced = strconv.FormatInt(n, 10)
		}
		if pd.Min != nil && float64(n) < *pd.Min {
			n = int64(*pd.Min)
			coerced = strconv.FormatInt(n, 10)
		}
		if pd.Max != nil && float64(n) > *pd.Max {
			n = int64(*pd.Max)
			coerced = strconv.FormatInt(n, 10)
		}

	case "number":
		f, err := strconv.ParseFloat(val, 64)
		if err != nil {
			return val, []string{fmt.Sprintf("expected number, got %q", val)}
		}
		if pd.Min != nil && f < *pd.Min {
			f = *pd.Min
			coerced = strconv.FormatFloat(f, 'f', -1, 64)
		}
		if pd.Max != nil && f > *pd.Max {
			f = *pd.Max
			coerced = strconv.FormatFloat(f, 'f', -1, 64)
		}

	case "boolean":
		switch strings.ToLower(val) {
		case "true", "1", "yes":
			coerced = "true"
		case "false", "0", "no":
			coerced = "false"
		default:
			return val, []string{fmt.Sprintf("expected boolean, got %q", val)}
		}

	case "json":
		if !json.Valid([]byte(val)) {
			return val, []string{"invalid JSON"}
		}
	}

	// String length checks (for string type or untyped)
	if pd.Type == "string" || pd.Type == "" {
		if pd.MinLength != nil && len(coerced) < *pd.MinLength {
			errs = append(errs, fmt.Sprintf("too short: %d < %d", len(coerced), *pd.MinLength))
		}
		if pd.MaxLength != nil && len(coerced) > *pd.MaxLength {
			coerced = coerced[:*pd.MaxLength]
		}
	}

	// Pattern (regex) — reject only
	if pd.Pattern != "" {
		re, err := regexp.Compile(pd.Pattern)
		if err == nil && !re.MatchString(coerced) {
			errs = append(errs, fmt.Sprintf("does not match pattern %q", pd.Pattern))
		}
	}

	// Enum — reject only
	if len(pd.Enum) > 0 {
		found := false
		for _, e := range pd.Enum {
			if coerced == e {
				found = true
				break
			}
		}
		if !found {
			errs = append(errs, fmt.Sprintf("must be one of: %s", strings.Join(pd.Enum, ", ")))
		}
	}

	return coerced, errs
}
