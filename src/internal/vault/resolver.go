// Copyright 2026 Jordan Sherer <hi@jordansherer.com>
// SPDX-License-Identifier: gpl

package vault

import (
	"fmt"
	"regexp"
	"strings"
)

// refPattern matches {{backend:content}} and {{backend:content|default}}.
// The content group uses .+? (non-greedy) to support both simple keys (MY_SECRET)
// and complex expressions (now('24h'), jsonpath(data, '$.field')).
var refPattern = regexp.MustCompile(`\{\{([A-Za-z0-9_][A-Za-z0-9_-]*):(.+?)(?:\|([^}]*))?\}\}`)

// ResolveFunc is a lightweight read-only resolver for computed backends
// like expr that don't need Get/Set/Delete/List/Close.
type ResolveFunc func(content string) (string, error)

// Resolver resolves {{backend:content}} references by dispatching to registered backends.
type Resolver struct {
	backends map[string]Backend
	funcs    map[string]ResolveFunc
}

func NewResolver() *Resolver {
	return &Resolver{
		backends: make(map[string]Backend),
		funcs:    make(map[string]ResolveFunc),
	}
}

func (r *Resolver) Register(prefix string, b Backend) {
	r.backends[prefix] = b
}

// RegisterFunc registers a lightweight resolver function for a prefix.
// Used for computed backends like "expr" that only resolve, never store.
func (r *Resolver) RegisterFunc(prefix string, fn ResolveFunc) {
	r.funcs[prefix] = fn
}

// Backend returns a registered backend by name, or nil if not found.
func (r *Resolver) Backend(name string) Backend {
	return r.backends[name]
}

// Resolve replaces all {{backend:key}} and {{backend:key|default}} references
// in s with their secret values. If a key is not found and a default is provided,
// the default is used instead of returning an error.
// Use \{{ to escape literal braces (e.g., \{{vault:TOKEN}} passes through as {{vault:TOKEN}}).
func (r *Resolver) Resolve(s string) (string, error) {
	// Protect escaped references before resolution
	const placeholder = "\x00ESCAPED_BRACE\x00"
	s = strings.ReplaceAll(s, "\\{{", placeholder)

	var resolveErr error
	result := refPattern.ReplaceAllStringFunc(s, func(match string) string {
		parts := refPattern.FindStringSubmatch(match)
		if len(parts) < 3 {
			return match
		}
		backend, key := parts[1], parts[2]
		defaultVal := ""
		hasDefault := len(parts) >= 4 && parts[3] != ""
		if hasDefault {
			defaultVal = parts[3]
		}
		// Try ResolveFunc first (lightweight computed backends like expr)
		if fn, ok := r.funcs[backend]; ok {
			val, err := fn(key)
			if err != nil {
				if hasDefault {
					return defaultVal
				}
				return match
			}
			return val
		}

		// Then try full Backend
		b, ok := r.backends[backend]
		if !ok {
			if hasDefault {
				return defaultVal
			}
			return match
		}
		val, err := b.Get(key)
		if err != nil {
			if hasDefault {
				return defaultVal
			}
			resolveErr = fmt.Errorf("resolving vault reference from %s backend: %w", backend, err)
			return match
		}
		return val
	})
	// Restore escaped references
	result = strings.ReplaceAll(result, placeholder, "{{")
	return result, resolveErr
}

// ResolveTracked resolves references like Resolve, but also returns which keys were
// successfully accessed (as "backend:key" strings).
func (r *Resolver) ResolveTracked(s string) (string, []string, error) {
	const placeholder = "\x00ESCAPED_BRACE\x00"
	s = strings.ReplaceAll(s, "\\{{", placeholder)

	var resolveErr error
	var accessed []string
	result := refPattern.ReplaceAllStringFunc(s, func(match string) string {
		parts := refPattern.FindStringSubmatch(match)
		if len(parts) < 3 {
			return match
		}
		backend, key := parts[1], parts[2]
		defaultVal := ""
		hasDefault := len(parts) >= 4 && parts[3] != ""
		if hasDefault {
			defaultVal = parts[3]
		}
		// Try ResolveFunc first
		if fn, ok := r.funcs[backend]; ok {
			val, err := fn(key)
			if err != nil {
				if hasDefault {
					return defaultVal
				}
				return match
			}
			// Don't track expr funcs as accessed keys (they're not secrets)
			return val
		}

		b, ok := r.backends[backend]
		if !ok {
			if hasDefault {
				return defaultVal
			}
			return match
		}
		val, err := b.Get(key)
		if err != nil {
			if hasDefault {
				return defaultVal
			}
			resolveErr = fmt.Errorf("resolving vault reference from %s backend: %w", backend, err)
			return match
		}
		accessed = append(accessed, backend+":"+key)
		return val
	})
	result = strings.ReplaceAll(result, placeholder, "{{")
	return result, accessed, resolveErr
}

// Redact resolves vault refs in originalRefs and replaces the individual
// secret values (what the vault backend returned) with "****" in s.
// Used for safe verbose logging.
func (r *Resolver) Redact(s string, originalRefs []string) string {
	if r == nil {
		return s
	}
	for _, ref := range originalRefs {
		// Extract each secret value from the ref
		refPattern.ReplaceAllStringFunc(ref, func(match string) string {
			parts := refPattern.FindStringSubmatch(match)
			if len(parts) < 3 {
				return match
			}
			backend, key := parts[1], parts[2]
			b, ok := r.backends[backend]
			if !ok {
				return match
			}
			val, err := b.Get(key)
			if err != nil || val == "" {
				return match
			}
			s = strings.ReplaceAll(s, val, "****")
			return match
		})
	}
	return s
}

// HasVaultRefs returns true if the string contains any {{backend:key}} references
// that require vault access. Excludes references that DON'T need the vault
// opened to resolve:
//   - {{env:VAR}}: resolved against the env backend at config load
//   - {{store:KEY}}: resolved against the bbolt store (no password)
//   - {{expr:...}}: pure computation, no I/O
//
// Anything else (vault, op, external backends like aws-sm) requires the
// vault backend or external secret backends; callers gate the password
// prompt + backend open on this returning true.
func HasVaultRefs(s string) bool {
	matches := refPattern.FindAllStringSubmatch(s, -1)
	for _, m := range matches {
		if len(m) < 2 {
			continue
		}
		switch m[1] {
		case "env", "store", "expr":
			continue
		default:
			return true
		}
	}
	return false
}
