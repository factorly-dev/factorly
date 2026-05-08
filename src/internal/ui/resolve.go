// Copyright 2026 Jordan Sherer <hi@jordansherer.com>
// SPDX-License-Identifier: gpl

package ui

// resolveRef resolves all backend references (e.g., {{vault:KEY}}, {{op:KEY}})
// in a string using the shared vault.Resolver.
func (s *Server) resolveRef(val string) string {
	if s.resolver == nil || val == "" {
		return val
	}
	resolved, err := s.resolver.Resolve(val)
	if err != nil {
		return val
	}
	return resolved
}

// resolveRefT resolves all backend refs in a value and appends accessed keys to the tracker.
func (s *Server) resolveRefT(val string, keys *[]string) string {
	if s.resolver == nil || val == "" {
		return val
	}
	resolved, accessed, err := s.resolver.ResolveTracked(val)
	if err != nil {
		return val
	}
	*keys = append(*keys, accessed...)
	return resolved
}

// resolveRefsTracked resolves backend references in a string slice and collects accessed keys.
func (s *Server) resolveRefsTracked(vals []string, keys *[]string) []string {
	if s.resolver == nil || len(vals) == 0 {
		return vals
	}
	out := make([]string, len(vals))
	for i, v := range vals {
		out[i] = s.resolveRefT(v, keys)
	}
	return out
}

// resolveRefMapTracked resolves backend references in a map and collects accessed keys.
func (s *Server) resolveRefMapTracked(m map[string]string, keys *[]string) map[string]string {
	if s.resolver == nil || len(m) == 0 {
		return m
	}
	out := make(map[string]string, len(m))
	for k, v := range m {
		out[k] = s.resolveRefT(v, keys)
	}
	return out
}
