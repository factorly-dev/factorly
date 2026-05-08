// Copyright 2026 Jordan Sherer <hi@jordansherer.com>
// SPDX-License-Identifier: gpl

package ui

// resolveRef resolves a single {{vault:KEY}} reference using the vault backend.
func (s *Server) resolveRef(val string) string {
	v, _ := s.resolveRefTracked(val)
	return v
}

// resolveRefTracked resolves a {{vault:KEY}} reference and returns the
// vault key name if one was accessed (empty string otherwise).
func (s *Server) resolveRefTracked(val string) (string, string) {
	if s.vault == nil || val == "" {
		return val, ""
	}
	if len(val) > 10 && val[:8] == "{{vault:" && val[len(val)-2:] == "}}" {
		key := val[8 : len(val)-2]
		if resolved, err := s.vault.Get(key); err == nil {
			return resolved, key
		}
		return val, key // still track the key even if resolution failed
	}
	return val, ""
}

// resolveRefsTracked resolves vault references in a string slice and collects accessed keys.
func (s *Server) resolveRefsTracked(vals []string, keys *[]string) []string {
	if s.vault == nil || len(vals) == 0 {
		return vals
	}
	out := make([]string, len(vals))
	for i, v := range vals {
		resolved, key := s.resolveRefTracked(v)
		out[i] = resolved
		if key != "" {
			*keys = append(*keys, key)
		}
	}
	return out
}

// resolveRefMapTracked resolves vault references in a map and collects accessed keys.
func (s *Server) resolveRefMapTracked(m map[string]string, keys *[]string) map[string]string {
	if s.vault == nil || len(m) == 0 {
		return m
	}
	out := make(map[string]string, len(m))
	for k, v := range m {
		resolved, vaultKey := s.resolveRefTracked(v)
		out[k] = resolved
		if vaultKey != "" {
			*keys = append(*keys, vaultKey)
		}
	}
	return out
}

// resolveRefT resolves a ref and appends the vault key to the tracker.
func (s *Server) resolveRefT(val string, keys *[]string) string {
	resolved, key := s.resolveRefTracked(val)
	if key != "" {
		*keys = append(*keys, key)
	}
	return resolved
}
