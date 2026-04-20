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
	// --- Shell / system commands ---
	{
		prefix: "find ",
		filter: &Filter{
			MaxLines: 100,
		},
	},
	{
		prefix: "grep ",
		filter: &Filter{
			MaxLines: 100,
		},
	},
	{
		prefix: "rg ",
		filter: &Filter{
			MaxLines: 100,
		},
	},
	{
		prefix: "ps ",
		filter: &Filter{
			MaxLines: 50,
		},
	},
	{
		prefix: "docker ps",
		filter: &Filter{
			MaxLines: 50,
		},
	},
	{
		prefix: "docker logs",
		filter: &Filter{
			MaxLines: 100,
		},
	},
	{
		prefix: "kubectl get",
		filter: &Filter{
			MaxLines: 50,
		},
	},
	{
		prefix: "kubectl describe",
		filter: &Filter{
			MaxLines: 100,
		},
	},
	{
		prefix: "kubectl logs",
		filter: &Filter{
			MaxLines: 100,
		},
	},
	// --- Test runners ---
	{
		prefix: "pytest",
		filter: &Filter{
			StripLines: []*regexp.Regexp{
				regexp.MustCompile(`^plugins:`),
				regexp.MustCompile(`^collecting `),
				regexp.MustCompile(`^platform `),
			},
			MatchOutput: []MatchRule{
				{
					Pattern: regexp.MustCompile(`passed`),
					Message: "ok (all tests passed)",
					Unless:  regexp.MustCompile(`failed|error`),
				},
			},
			MaxLines: 100,
		},
	},
	{
		prefix: "npm test",
		filter: &Filter{
			StripLines: []*regexp.Regexp{
				regexp.MustCompile(`^npm warn`),
				regexp.MustCompile(`^\s*$`),
			},
			MaxLines: 100,
		},
	},
	{
		prefix: "pnpm test",
		filter: &Filter{
			StripLines: []*regexp.Regexp{
				regexp.MustCompile(`^\s*$`),
			},
			MaxLines: 100,
		},
	},
	// --- Package managers ---
	{
		prefix: "apt install",
		filter: &Filter{
			StripLines: []*regexp.Regexp{
				regexp.MustCompile(`^\s*$`),
				regexp.MustCompile(`^Hit:`),
				regexp.MustCompile(`^Get:`),
				regexp.MustCompile(`^Reading `),
				regexp.MustCompile(`^Building `),
			},
			MaxLines: 30,
		},
	},
	{
		prefix: "apt update",
		filter: &Filter{
			StripLines: []*regexp.Regexp{
				regexp.MustCompile(`^Hit:`),
				regexp.MustCompile(`^Get:`),
			},
			MatchOutput: []MatchRule{
				{
					Pattern: regexp.MustCompile(`All packages are up to date`),
					Message: "ok (up to date)",
				},
			},
			MaxLines: 30,
		},
	},
	{
		prefix: "brew install",
		filter: &Filter{
			StripLines: []*regexp.Regexp{
				regexp.MustCompile(`^==> Downloading`),
				regexp.MustCompile(`^Already downloaded`),
				regexp.MustCompile(`^###`),
			},
			MatchOutput: []MatchRule{
				{
					Pattern: regexp.MustCompile(`already installed`),
					Message: "ok (already installed)",
				},
			},
			MaxLines: 30,
		},
	},
	// --- Infrastructure ---
	{
		prefix: "terraform plan",
		filter: &Filter{
			StripLines: []*regexp.Regexp{
				regexp.MustCompile(`Refreshing state\.\.\.`),
				regexp.MustCompile(`^\s*$`),
			},
			MatchOutput: []MatchRule{
				{
					Pattern: regexp.MustCompile(`No changes`),
					Message: "ok (no changes)",
				},
			},
			MaxLines: 100,
		},
	},
	{
		prefix: "terraform apply",
		filter: &Filter{
			StripLines: []*regexp.Regexp{
				regexp.MustCompile(`Refreshing state\.\.\.`),
				regexp.MustCompile(`^\s*$`),
			},
			MaxLines: 100,
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
