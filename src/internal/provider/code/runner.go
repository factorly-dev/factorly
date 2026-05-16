// Copyright 2026 Jordan Sherer <hi@jordansherer.com>
// SPDX-License-Identifier: gpl

package code

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"regexp"

	"github.com/traefik/yaegi/interp"
)

// runFn is the typed signature scripts must export.
type runFn = func(map[string]string) (any, error)

var packageDeclRe = regexp.MustCompile(`^\s*package\s+([A-Za-z_][A-Za-z0-9_]*)`)

// extractPackageName returns the package identifier declared at the top
// of src. Scripts must start with `package <ident>`; without it yaegi
// rejects the source.
func extractPackageName(src string) (string, error) {
	m := packageDeclRe.FindStringSubmatch(src)
	if m == nil {
		return "", fmt.Errorf("code script must begin with a `package <name>` declaration")
	}
	return m[1], nil
}

// validateScript compiles src in a throwaway interpreter and confirms
// it exports a `Run(map[string]string) (any, error)` function. It is
// called at registration time so config load surfaces bad scripts early,
// rather than waiting for first invocation.
//
// The throwaway interpreter is built with the real stdlib subset + a
// no-op factorly.Call stub so type checking and import resolution match
// what runScript will see at execution time.
func validateScript(src string) error {
	pkgName, err := extractPackageName(src)
	if err != nil {
		return err
	}
	i := interp.New(interp.Options{})
	if err := i.Use(stdlibSubset()); err != nil {
		return fmt.Errorf("stdlib setup: %w", err)
	}
	noopCall := func(string, map[string]string) (*Result, error) {
		return &Result{}, nil
	}
	noopList := func() []ToolInfo { return nil }
	if err := i.Use(factorlyExports(noopCall, noopList)); err != nil {
		return fmt.Errorf("factorly setup: %w", err)
	}
	if _, err := i.Eval(src); err != nil {
		return fmt.Errorf("compile: %w", err)
	}
	runVal, err := i.Eval(pkgName + ".Run")
	if err != nil {
		return fmt.Errorf("Run lookup: script must declare `func Run(params map[string]string) (any, error)`: %w", err)
	}
	if _, ok := runVal.Interface().(runFn); !ok {
		return fmt.Errorf("Run signature mismatch: must be `func(params map[string]string) (any, error)`, got %s", runVal.Type())
	}
	return nil
}

// runScript executes src inside a fresh interpreter with call wired to
// the supplied closure. The script's `Run(params)` is invoked and its
// (any, error) result returned. Yaegi's EvalWithContext is used for
// timeout/cancellation: it runs in a goroutine internally, recovers
// panics into an interp.Panic error, and cancels via ctx.
func runScript(ctx context.Context, src string, params map[string]string, call callFunc, tools []ToolInfo) (any, error) {
	pkgName, err := extractPackageName(src)
	if err != nil {
		return nil, err
	}
	i := interp.New(interp.Options{})
	if err := i.Use(stdlibSubset()); err != nil {
		return nil, fmt.Errorf("stdlib setup: %w", err)
	}
	// Snapshot tools at script-start so multiple calls to
	// factorly.ListTools within one Run see a stable view.
	toolsSnap := append([]ToolInfo(nil), tools...)
	list := func() []ToolInfo { return toolsSnap }
	if err := i.Use(factorlyExports(call, list)); err != nil {
		return nil, fmt.Errorf("factorly setup: %w", err)
	}
	if _, err := i.EvalWithContext(ctx, src); err != nil {
		return nil, fmt.Errorf("compile: %w", normalizePanic(err))
	}
	runVal, err := i.EvalWithContext(ctx, pkgName+".Run")
	if err != nil {
		return nil, fmt.Errorf("Run lookup: %w", normalizePanic(err))
	}
	fn, ok := runVal.Interface().(runFn)
	if !ok {
		return nil, fmt.Errorf("Run signature: must be `func(params map[string]string) (any, error)`, got %s", runVal.Type())
	}

	// Invoke Run. yaegi's EvalWithContext only wraps Eval calls — when
	// we reflect-invoke fn directly, panics inside the interpreted code
	// surface here as Go panics. Recover and tag them so the provider
	// can return a clean Result{ExitCode: 2}.
	type runResult struct {
		val any
		err error
	}
	resCh := make(chan runResult, 1)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				resCh <- runResult{nil, panicErr{value: r}}
			}
		}()
		v, e := fn(params)
		resCh <- runResult{v, e}
	}()
	select {
	case <-ctx.Done():
		// Best-effort stop; interpreter may continue briefly but the
		// caller has already accepted the deadline.
		return nil, ctx.Err()
	case r := <-resCh:
		return r.val, r.err
	}
}

// panicErr is the sentinel wrapping a panic value recovered from
// interpreted code. The provider Execute path uses errors.As to surface
// it as a Result{ExitCode: 2}.
type panicErr struct {
	value any
}

func (p panicErr) Error() string {
	return fmt.Sprintf("panic: %v", p.value)
}

// normalizePanic rewrites yaegi's internal Panic wrapper into a panicErr
// so the provider can detect panics uniformly. Yaegi wraps panics raised
// during EvalWithContext in an interp.Panic value; we surface the inner
// message.
func normalizePanic(err error) error {
	if err == nil {
		return nil
	}
	var p interp.Panic
	if errors.As(err, &p) {
		return panicErr{value: p.Value}
	}
	return err
}

// marshalResultOutput converts whatever the script returned into the
// string body of a provider.Result.Output. The rules:
//
//	("hello", nil)             -> "hello"
//	(struct/map/etc, nil)      -> json.Marshal(v)
//	(nil, nil)                 -> ""
func marshalResultOutput(v any) (string, error) {
	if v == nil {
		return "", nil
	}
	switch s := v.(type) {
	case string:
		return s, nil
	case []byte:
		return string(s), nil
	case fmt.Stringer:
		return s.String(), nil
	}
	// reflect-check for nil pointer/interface that survived the typed nil check.
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Ptr, reflect.Map, reflect.Slice, reflect.Interface:
		if rv.IsNil() {
			return "", nil
		}
	}
	b, err := json.Marshal(v)
	if err != nil {
		return "", fmt.Errorf("marshaling script result: %w", err)
	}
	return string(b), nil
}
