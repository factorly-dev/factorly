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

// ResolveCallerParam is the gated resolver used for caller-supplied
// param values (CLI args, MCP-tool params, agent inputs). It applies
// two rules the bootstrap-time Resolve doesn't:
//
//  1. References against secret backends ({{vault:K}}, {{op:...}},
//     external backends like {{aws-sm:...}}) are skipped unless
//     allowSecretBackends is true. The original template text flows
//     through unchanged — same effect as if the substitution had
//     never been attempted.
//  2. The returned secretRefs slice lists every {{backend:key}}
//     template the call resolved against a secret backend. Empty
//     when no secret refs were hit (the common case). The audit-log
//     layer uses this to replace resolved values with their template
//     strings before persisting Entry.Params.
//
// allowSecretBackends == true is the per-param opt-in path (the
// param's tool config carries hydrate_vault_refs: true). Default
// callers pass false.
//
// Safe backends (env, store, expr — see IsSafeBackendName) always
// resolve and are never reported in secretRefs.
func (r *Resolver) ResolveCallerParam(s string, allowSecretBackends bool) (resolved string, secretRefs []string, err error) {
	if r == nil {
		return s, nil, nil
	}
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

		// Secret-backend gating. When the caller didn't opt in we
		// return the match verbatim — no resolution attempt, no
		// default fallback. Defaults are intentionally NOT applied
		// here because the template is being preserved end-to-end:
		// the literal {{vault:K}} flows through to the provider.
		if IsSecretBackendName(backend) && !allowSecretBackends {
			return match
		}

		// Try ResolveFunc first (lightweight computed backends).
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
		// Full backend.
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
		// Successful resolution against a secret backend — record
		// the original template so the caller can redact the value
		// from the audit log later.
		if IsSecretBackendName(backend) {
			secretRefs = append(secretRefs, match)
		}
		return val
	})
	result = strings.ReplaceAll(result, placeholder, "{{")
	return result, secretRefs, resolveErr
}

// RedactToTemplate is like Redact but replaces the resolved value
// with the original template string (e.g. "{{vault:KEY}}") instead
// of "****". Used for audit logging where preserving the template
// gives the operator something to grep for. originalRefs is the
// list returned by ResolveCallerParam.
//
// We re-fetch the value from the backend (same shape as Redact) so
// the substitution is exact — string replacement against the live
// secret value. If the backend can no longer return the value
// (vault locked between resolution and logging), the resolved value
// stays in the string. That residual leak is documented; in
// practice the resolver opened the backend successfully moments
// earlier so it's still cached.
func (r *Resolver) RedactToTemplate(s string, originalRefs []string) string {
	if r == nil {
		return s
	}
	for _, ref := range originalRefs {
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
			s = strings.ReplaceAll(s, val, ref)
			return match
		})
	}
	return s
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
		if !IsSafeBackendName(m[1]) {
			return true
		}
	}
	return false
}

// IsSafeBackendName reports whether name refers to a backend that is
// safe to resolve against caller-supplied (potentially LLM-generated)
// param values. The allowlist is intentionally tight:
//
//   - "env"   — read-only, scoped to the host's environment
//   - "store" — agent's own scratchpad; reading from it can't leak
//     anything the agent didn't already put there
//   - "expr"  — pure computation, no I/O
//
// Anything else ("vault", "op", "aws-sm", "1password", ...) is treated
// as secret and requires the caller to explicitly opt in (via the
// param's hydrate_vault_refs flag in tool config) before its values
// are substituted into a caller-controlled input. Defaulting unknown
// backends to "secret" is deliberate: a future backend added without
// updating this list is safe-by-default.
func IsSafeBackendName(name string) bool {
	switch name {
	case "env", "store", "expr":
		return true
	default:
		return false
	}
}

// IsSecretBackendName is the inverse of IsSafeBackendName. Provided
// for read-site clarity when the caller is gating on "is this backend
// secret?" rather than "is this backend safe?".
func IsSecretBackendName(name string) bool {
	return !IsSafeBackendName(name)
}
