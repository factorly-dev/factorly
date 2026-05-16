// Copyright 2026 Jordan Sherer <hi@jordansherer.com>
// SPDX-License-Identifier: gpl

package code

import (
	"reflect"

	"github.com/traefik/yaegi/interp"
	"github.com/traefik/yaegi/stdlib"
)

// allowedStdlib enumerates the stdlib packages a code-tool script can
// import. Anything outside this list is denied at script-load time so
// scripts cannot reach the filesystem, network, or process state. Keep
// this list small; widen only when real scripts demand it.
var allowedStdlib = map[string]bool{
	"fmt/fmt":            true,
	"strings/strings":    true,
	"strconv/strconv":    true,
	"time/time":          true,
	"encoding/json/json": true,
	"errors/errors":      true,
}

// stdlibSubset returns a filtered copy of stdlib.Symbols containing only
// the entries in allowedStdlib. It panics with a clear message if any
// expected package is missing from the installed yaegi version — a build
// problem we want loud, not silent.
func stdlibSubset() interp.Exports {
	out := make(interp.Exports, len(allowedStdlib))
	for key := range allowedStdlib {
		syms, ok := stdlib.Symbols[key]
		if !ok {
			// Yaegi's stdlib has the package keyed differently than we
			// expected. Surface the mismatch instead of silently denying
			// the import.
			panic("code-provider: stdlib package " + key + " not present in yaegi.stdlib.Symbols")
		}
		out[key] = syms
	}
	return out
}

// callFunc is the signature scripts see as factorly.Call. The host wires
// each script execution with a closure that increments the per-script
// call counter and invokes the proxy.
type callFunc func(name string, params map[string]string) (*Result, error)

// listToolsFunc is the signature scripts see as factorly.ListTools. The
// host wires each script execution with a snapshot of currently-visible
// tools so the snapshot is stable for the duration of one Run.
type listToolsFunc func() []ToolInfo

// factorlyExports builds the in-script "factorly" package. The map key
// "factorly/factorly" follows yaegi's "importpath/pkgname" convention so
// the script can write `import "factorly"`.
func factorlyExports(call callFunc, list listToolsFunc) interp.Exports {
	return interp.Exports{
		"factorly/factorly": {
			"Call":      reflect.ValueOf(call),
			"ListTools": reflect.ValueOf(list),
			"Result":    reflect.ValueOf((*Result)(nil)),
			"ToolInfo":  reflect.ValueOf((*ToolInfo)(nil)),
			"ParamInfo": reflect.ValueOf((*ParamInfo)(nil)),
		},
	}
}
