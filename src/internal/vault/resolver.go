package vault

import (
	"fmt"
	"regexp"
)

var refPattern = regexp.MustCompile(`\{\{([A-Za-z0-9_][A-Za-z0-9_-]*):([A-Za-z0-9_./-]+)\}\}`)

// Resolver resolves {{backend:key}} references by dispatching to registered backends.
type Resolver struct {
	backends map[string]Backend
}

func NewResolver() *Resolver {
	return &Resolver{backends: make(map[string]Backend)}
}

func (r *Resolver) Register(prefix string, b Backend) {
	r.backends[prefix] = b
}

// Backend returns a registered backend by name, or nil if not found.
func (r *Resolver) Backend(name string) Backend {
	return r.backends[name]
}

// Resolve replaces all {{backend:key}} references in s with their secret values.
func (r *Resolver) Resolve(s string) (string, error) {
	var resolveErr error
	result := refPattern.ReplaceAllStringFunc(s, func(match string) string {
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
		if err != nil {
			resolveErr = fmt.Errorf("resolving vault reference from %s backend: %w", backend, err)
			return match
		}
		return val
	})
	return result, resolveErr
}

// HasVaultRefs returns true if the string contains any {{backend:key}} references.
func HasVaultRefs(s string) bool {
	return refPattern.MatchString(s)
}
