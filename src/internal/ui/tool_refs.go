// Copyright 2026 Jordan Sherer <hi@jordansherer.com>
// SPDX-License-Identifier: gpl

package ui

import (
	"regexp"

	"github.com/factorly-dev/factorly/internal/config"
	"github.com/factorly-dev/factorly/internal/configyaml"
)

// toolReferenceCounts scans every tool config in cfg, renders it to
// YAML, and counts how many tools reference each `{{<backend>:KEY}}`
// at least once. backend is "vault" or "store" (any string works,
// but those are the call sites). Returns a map keyed by the
// referenced KEY → number of distinct tools that mention it.
//
// "References" means a template substring like `{{vault:FOO_BAR}}`
// anywhere in the rendered YAML — auth blocks, headers, body,
// params, base_url. We match exact form `{{<backend>:<key>}}` with
// no inner whitespace, which is what the resolver actually accepts.
//
// A tool that mentions the same KEY in three different fields counts
// once. Tools that fail to render (shouldn't happen for valid in-
// memory configs) are skipped silently — this is for UI hints, not
// correctness gates.
func toolReferenceCounts(cfg *config.Config, backend string) map[string]int {
	out := map[string]int{}
	if cfg == nil {
		return out
	}
	pat := backendRefPattern(backend)
	for name, tc := range cfg.Tools {
		yamlBytes, err := configyaml.RenderTool(name, tc)
		if err != nil {
			continue
		}
		mergeMatches(out, pat, yamlBytes)
	}
	return out
}

// oauthProviderReferenceCounts mirrors toolReferenceCounts but scans
// cfg.OAuthProviders entries instead — used to surface "used by N
// auths" badges on the vault/store lists. Each provider's ClientID,
// ClientSecret, AuthURL, TokenURL is examined for `{{<backend>:KEY}}`.
// One provider mentioning the same KEY twice counts once toward that
// KEY's total.
func oauthProviderReferenceCounts(cfg *config.Config, backend string) map[string]int {
	out := map[string]int{}
	if cfg == nil {
		return out
	}
	pat := backendRefPattern(backend)
	for _, p := range cfg.OAuthProviders {
		// Concatenating the four ref-bearing fields with a separator
		// is enough; we don't need YAML round-trip here — the fields
		// are all plain strings and the regex doesn't care about
		// surrounding context.
		blob := []byte(p.ClientID + "\x00" + p.ClientSecret + "\x00" + p.AuthURL + "\x00" + p.TokenURL)
		mergeMatches(out, pat, blob)
	}
	return out
}

// backendRefPattern returns the compiled regex matching
// `{{<backend>:KEY}}` template references. KEY allows the standard
// env-var alphabet plus dash/dot/slash chars resolver keys take. Be
// lenient on the right side; this is a UI hint, not a security check.
func backendRefPattern(backend string) *regexp.Regexp {
	return regexp.MustCompile(`\{\{` + regexp.QuoteMeta(backend) + `:([A-Za-z0-9_./-]+)\}\}`)
}

// mergeMatches finds all `{{<backend>:KEY}}` matches in blob and
// bumps out[KEY] by 1 for each DISTINCT key seen — i.e. one source
// (tool, provider, …) mentioning the same KEY in multiple spots
// only counts once toward that KEY's total.
func mergeMatches(out map[string]int, pat *regexp.Regexp, blob []byte) {
	matches := pat.FindAllSubmatch(blob, -1)
	if len(matches) == 0 {
		return
	}
	seen := map[string]bool{}
	for _, m := range matches {
		key := string(m[1])
		if seen[key] {
			continue
		}
		seen[key] = true
		out[key]++
	}
}
