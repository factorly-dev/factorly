// Copyright 2026 Jordan Sherer <hi@jordansherer.com>
// SPDX-License-Identifier: gpl

package output

import (
	"regexp"
	"strings"
)

// builtinFilter defines a command prefix and its associated filter.
type builtinFilter struct {
	prefix string
	filter *Filter
}

// builtinFilters are default filters for common commands, applied when
// no user-defined filter exists for the tool. Matched by command prefix.
var builtinFilters = []builtinFilter{
	{
		prefix: "git status",
		filter: &Filter{
			StripLines: []*regexp.Regexp{
				regexp.MustCompile(`^\s+\(use "git `),
			},
			MaxLines: 30,
		},
	},
	{
		prefix: "git log",
		filter: &Filter{
			MaxLines: 50,
		},
	},
	{
		prefix: "git diff",
		filter: &Filter{
			MaxLines: 200,
		},
	},
	{
		prefix: "make",
		filter: &Filter{
			StripLines: []*regexp.Regexp{
				regexp.MustCompile(`^make\[\d+\]:\s+(Entering|Leaving) directory`),
			},
			MatchOutput: []MatchRule{
				{
					Pattern: regexp.MustCompile(`Nothing to be done`),
					Message: "ok (nothing to do)",
				},
			},
			MaxLines: 50,
		},
	},
	{
		prefix: "npm install",
		filter: &Filter{
			StripLines: []*regexp.Regexp{
				regexp.MustCompile(`^npm warn`),
				regexp.MustCompile(`^\s*$`),
			},
			MatchOutput: []MatchRule{
				{
					Pattern: regexp.MustCompile(`added \d+ packages`),
					Message: "ok (packages installed)",
					Unless:  regexp.MustCompile(`(?i)err!|error`),
				},
				{
					Pattern: regexp.MustCompile(`up to date`),
					Message: "ok (up to date)",
				},
			},
			MaxLines: 30,
		},
	},
	{
		prefix: "pnpm install",
		filter: &Filter{
			StripLines: []*regexp.Regexp{
				regexp.MustCompile(`^Progress:`),
				regexp.MustCompile(`^\s*$`),
			},
			MatchOutput: []MatchRule{
				{
					Pattern: regexp.MustCompile(`Already up to date`),
					Message: "ok (up to date)",
				},
			},
			MaxLines: 30,
		},
	},
	{
		prefix: "go test",
		filter: &Filter{
			KeepLines: []*regexp.Regexp{
				regexp.MustCompile(`^---`),
				regexp.MustCompile(`^FAIL`),
				regexp.MustCompile(`^PASS`),
				regexp.MustCompile(`^ok\s`),
				regexp.MustCompile(`^\?`),
				regexp.MustCompile(`^exit status`),
			},
			MatchOutput: []MatchRule{
				{
					Pattern: regexp.MustCompile(`(?m)^PASS$`),
					Message: "ok (all tests passed)",
					Unless:  regexp.MustCompile(`(?m)^FAIL`),
				},
			},
			MaxLines: 100,
		},
	},
	{
		prefix: "cargo test",
		filter: &Filter{
			StripLines: []*regexp.Regexp{
				regexp.MustCompile(`^\s+Compiling `),
				regexp.MustCompile(`^\s+Downloading `),
				regexp.MustCompile(`^\s+Fresh `),
			},
			MatchOutput: []MatchRule{
				{
					Pattern: regexp.MustCompile(`test result: ok`),
					Message: "ok (all tests passed)",
					Unless:  regexp.MustCompile(`FAILED|panicked`),
				},
			},
			MaxLines: 100,
		},
	},
	{
		prefix: "cargo build",
		filter: &Filter{
			StripLines: []*regexp.Regexp{
				regexp.MustCompile(`^\s+Compiling `),
				regexp.MustCompile(`^\s+Downloading `),
				regexp.MustCompile(`^\s+Fresh `),
			},
			MatchOutput: []MatchRule{
				{
					Pattern: regexp.MustCompile(`Finished`),
					Message: "ok (build finished)",
					Unless:  regexp.MustCompile(`^error`),
				},
			},
			MaxLines: 50,
		},
	},
	{
		prefix: "pip install",
		filter: &Filter{
			StripLines: []*regexp.Regexp{
				regexp.MustCompile(`^Requirement already satisfied`),
				regexp.MustCompile(`^\s*$`),
			},
			MatchOutput: []MatchRule{
				{
					Pattern: regexp.MustCompile(`Successfully installed`),
					Message: "ok (installed)",
					Unless:  regexp.MustCompile(`(?i)error`),
				},
			},
			MaxLines: 30,
		},
	},
}

// BuiltinFilter returns the built-in filter for the given command string,
// or nil if no built-in filter matches.
func BuiltinFilter(command string) *Filter {
	cmd := strings.TrimSpace(command)
	for _, bf := range builtinFilters {
		if strings.HasPrefix(cmd, bf.prefix) {
			return bf.filter
		}
	}
	return nil
}
