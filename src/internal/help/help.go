// Copyright 2026 Jordan Sherer <hi@jordansherer.com>
// SPDX-License-Identifier: gpl

// Package help renders factorly's self-describing onboarding text for
// agents and humans. Both the `factorly help` CLI and the
// `factorly.help` MCP builtin render from this same corpus so the
// instructions an agent reads at runtime match what a user gets when
// they bootstrap their system prompt.
//
// Design rules:
//   - Default (empty topic) returns a TIGHT overview + behaviors block.
//     Cheap for an agent to call on connection. Topic-specific deep
//     dives are opt-in.
//   - Output is plain markdown. No styling, no ANSI — the consumer
//     might be a terminal, an LLM, or a system-prompt copy/paste.
//   - Personalized: when a *config.Config is provided, the corpus
//     reflects THIS user's installed blueprints and tools. Generic
//     fallback when cfg is nil.
package help

import (
	"fmt"
	"sort"
	"strings"

	"github.com/factorly-dev/factorly/internal/blueprints"
	"github.com/factorly-dev/factorly/internal/builtins"
	"github.com/factorly-dev/factorly/internal/config"
)

// Inputs carries the live state Render reflects in personalized
// output. Both fields may be zero — a nil Config returns a generic
// view; an empty CfgPath skips the installed-blueprints lookup.
type Inputs struct {
	Config  *config.Config
	CfgPath string
}

// Topic addresses a section of the corpus. "" returns the default
// tight overview + behaviors. Anything else returns a focused
// section. Unknown topics fall back to the default with a one-line
// note so an agent's typo doesn't silently misfire.
type Topic string

const (
	TopicDefault    Topic = ""
	TopicVault      Topic = "vault"
	TopicShadow     Topic = "shadow"
	TopicWorkflows  Topic = "workflows"
	TopicBlueprints Topic = "blueprints"
	TopicTools      Topic = "tools"
	TopicWhatIs     Topic = "what-is"
	// TopicTool is rendered via RenderTool(name, cfg) — it's not a
	// fixed topic because the name comes from the caller. Listed
	// here so callers can advertise it.
	TopicTool Topic = "tool"
)

// AllTopics is the set Render accepts (plus "" and "tool:<name>").
// Used by the CLI's --help and the MCP builtin's parameter schema so
// the topic list stays in one place.
var AllTopics = []Topic{
	TopicWhatIs,
	TopicVault,
	TopicShadow,
	TopicWorkflows,
	TopicBlueprints,
	TopicTools,
}

// Render returns the help body for topic. Inputs may be zero — a
// nil Config yields a generic view; an empty CfgPath skips the
// installed-blueprints enumeration.
func Render(topic Topic, in Inputs) string {
	switch topic {
	case TopicDefault:
		return renderDefault(in)
	case TopicWhatIs:
		return whatIsFactorly
	case TopicVault:
		return vaultTopic
	case TopicShadow:
		return shadowTopic
	case TopicWorkflows:
		return workflowsTopic
	case TopicBlueprints:
		return renderBlueprints(in)
	case TopicTools:
		return renderTools(in.Config)
	default:
		return fmt.Sprintf("Unknown topic %q.\n\n%s", topic, renderDefault(in))
	}
}

// RenderTool returns detailed docs for one tool by name. Returns "" if
// the tool isn't in cfg so callers can fall back to a generic message.
func RenderTool(name string, cfg *config.Config) string {
	if cfg == nil {
		return ""
	}
	tc, ok := cfg.Tools[name]
	if !ok {
		return ""
	}
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n", name)
	if tc.Description != "" {
		fmt.Fprintf(&b, "%s\n\n", tc.Description)
	}
	fmt.Fprintf(&b, "**Type:** `%s`\n\n", tc.Type)
	if len(tc.Parameters) > 0 {
		b.WriteString("## Parameters\n\n")
		for _, p := range tc.Parameters {
			required := ""
			if p.Required {
				required = " (required)"
			}
			fmt.Fprintf(&b, "- `%s`", p.Name)
			if p.Type != "" {
				fmt.Fprintf(&b, " *(%s)*", p.Type)
			}
			b.WriteString(required)
			if p.Description != "" {
				fmt.Fprintf(&b, " — %s", p.Description)
			}
			if p.Default != "" {
				fmt.Fprintf(&b, " (default: `%s`)", p.Default)
			}
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}
	if tc.Shadow != nil {
		b.WriteString("## Oversight\n\n")
		confirmList, confirmAll := tc.Shadow.ConfirmList()
		switch {
		case confirmAll:
			b.WriteString("- Every call requires user approval (`shadow.confirm: true`). The call queues; the user approves or denies. This is expected, not an error.\n")
		case len(confirmList) > 0:
			fmt.Fprintf(&b, "- Some sub-tool names require user approval: %s\n", strings.Join(confirmList, ", "))
		}
		if tc.Shadow.RateLimit != "" {
			fmt.Fprintf(&b, "- Rate limit: %s\n", tc.Shadow.RateLimit)
		}
		if len(tc.Shadow.Deny) > 0 {
			fmt.Fprintf(&b, "- Denied param patterns: %s\n", strings.Join(tc.Shadow.Deny, ", "))
		}
		b.WriteString("\n")
	}
	return b.String()
}

// renderDefault is the tight overview + behaviors block. This is what
// an agent gets on the bare `factorly.help()` call — short enough to
// be cheap, dense enough to set behavior. Topic-specific deep dives
// live separately.
func renderDefault(in Inputs) string {
	var b strings.Builder
	b.WriteString(whatIsFactorly)
	b.WriteString("\n\n## Behaviors to expect\n\n")
	b.WriteString(behaviorsBlock)
	b.WriteString("\n\n## What's installed here\n\n")
	b.WriteString(renderInstalledSummary(in))
	b.WriteString("\n\n## Topics for deeper detail\n\n")
	b.WriteString("Call `factorly.help` again with a `topic` parameter for any of:\n")
	for _, t := range AllTopics {
		fmt.Fprintf(&b, "- `%s`\n", t)
	}
	b.WriteString("\nOr `factorly.help(tool: \"<name>\")` for per-tool docs.\n")
	return b.String()
}

const whatIsFactorly = `# What factorly is

Factorly is a local runtime for tool calls. It wraps every tool below
with credential injection, audit logging, and governance rules. When
you call a tool, factorly resolves any ` + "`{{vault:KEY}}`" + ` references
from the encrypted vault, applies the configured policy, executes the
call, and writes a structured audit entry.

You don't need credentials. The operator wired them up. You don't need
to know which tier the vault lives in. Factorly handles it.`

const behaviorsBlock = `- **Credentials are already wired up.** If a tool needs an API key,
  it's resolved from the vault at call time. Don't ask the user for
  tokens; they're already where they need to be.
- **Some calls require approval.** Tools with ` + "`shadow.confirm: true`" + `
  queue for the user to approve or deny. This is normal — wait for
  the response; don't treat a slow call as a failure.
- **Audit is automatic.** Every call you make is logged with params,
  response, duration, and a hash-chained audit entry. You don't need
  to log anything yourself.
- **Repeated patterns become workflows.** If you find yourself calling
  the same N tools in sequence, suggest the user save it as a workflow
  (` + "`factorly workflow add`" + `). It runs faster, is auditable as a unit,
  and is shareable as a blueprint.
- **Don't fabricate tools.** Only call tools that appear in your tool
  list. If you need something that isn't there, tell the user — they
  can install a blueprint or add a tool.`

const vaultTopic = `# The vault model

Factorly's vault is encrypted on disk and resolves ` + "`{{vault:KEY}}`" + `
references at call time. You never see the resolved value — the
agent layer is deliberately kept out of the credential.

When a tool's config says ` + "`token: \"{{vault:GITHUB_TOKEN}}\"`" + `, factorly
resolves it the moment the call executes. The request goes out with
the real token in its Authorization header; the response comes back
to you. The credential is never serialized into your conversation,
never sent in a tool call's params, never echoed in the audit log.

**What this means for you:**
- Don't ask the user for API keys or tokens. They're already set.
- If a tool fails with an authentication error, the message will say
  which vault key is missing — pass that hint to the user verbatim.
- Never store credentials in the agent-writable store (` + "`{{store:K}}`" + `).
  Use the vault.`

const shadowTopic = `# Oversight (shadow rules)

Tools can carry shadow rules that gate every call:

- ` + "`shadow.confirm: true`" + ` — the call queues for human approval. The
  user sees a confirm modal; you wait for their decision. Don't retry
  on the timeout; the user is offline or busy.
- ` + "`shadow.rate_limit: \"N/period\"`" + ` — calls are throttled. If you're
  blocked, slow down rather than retry instantly.
- ` + "`shadow.deny`" + ` — certain param values are rejected (e.g. ` + "`rm -rf`" + ` in
  a shell command). The error message names what's blocked.
- ` + "`shadow.max_calls`" + ` — limits how many sub-tool calls a code-tool
  script can make. Relevant if you're inside ` + "`factorly.code`" + `.

If a call returns a shadow error, surface the reason to the user
verbatim. They can adjust the policy in their config.`

const workflowsTopic = `# Workflows

A workflow is a named, deterministic sequence of tool calls with
per-step error handling, conditionals, and shared state. Workflows
are first-class: they appear in your tool list with type ` + "`workflow`" + `,
execute as a single call, and produce one coalesced audit entry that
groups every step.

**When to suggest a workflow to the user:**
- They've asked you to do the same multi-step task more than once.
- The sequence is deterministic — same steps every time, only inputs
  vary.
- It's worth replaying ` + "`factorly logs replay <hash>`" + `-style for
  debugging or regression testing.

Suggest something like:
> *"I've done this triage sequence three times now. Want me to save it
> as a workflow? You'd be able to run it with one command and we'd
> have a stable audit trail."*

You can write the workflow YAML directly via ` + "`factorly.file.write`" + `
into ` + "`.factorly/`" + ` (or describe it for the user to create).`

func renderBlueprints(in Inputs) string {
	var b strings.Builder
	b.WriteString("# Blueprints\n\n")
	b.WriteString("Blueprints are reusable bundles of tools + workflows the operator can install with one command. ")
	b.WriteString("Each blueprint declares its own vault key requirements; the operator wires them up at install time.\n\n")
	installed := listInstalledBlueprints(in.CfgPath)
	if len(installed) == 0 {
		b.WriteString("**Installed here:** none yet.\n\n")
		b.WriteString("If you need tools for a specific service (GitHub, Slack, Linear, Gmail, Stripe, …), tell the user — they can run `factorly blueprint install <name>` to add it.\n")
		return b.String()
	}
	b.WriteString("**Installed here:**\n\n")
	for _, bp := range installed {
		name := bp.Name
		if name == "" {
			name = bp.Filename
		}
		if bp.Description != "" {
			fmt.Fprintf(&b, "- **%s** — %s\n", name, bp.Description)
		} else {
			fmt.Fprintf(&b, "- **%s**\n", name)
		}
	}
	return b.String()
}

// listInstalledBlueprints wraps blueprints.List with a stable sort by
// display name so the rendered output is deterministic. Empty cfgPath
// or a read error returns nil — the caller renders an empty-state.
func listInstalledBlueprints(cfgPath string) []blueprints.BlueprintHeader {
	if cfgPath == "" {
		return nil
	}
	list, err := blueprints.List(cfgPath)
	if err != nil {
		return nil
	}
	sort.Slice(list, func(i, j int) bool {
		a, b := list[i].Name, list[j].Name
		if a == "" {
			a = list[i].Filename
		}
		if b == "" {
			b = list[j].Filename
		}
		return a < b
	})
	return list
}

func renderTools(cfg *config.Config) string {
	var b strings.Builder
	b.WriteString("# Tools available here\n\n")
	if cfg == nil || len(cfg.Tools) == 0 {
		b.WriteString("No tools configured yet. Tell the user — they can add one with `factorly tools add`, install a blueprint with `factorly blueprint install <name>`, or import an OpenAPI spec.\n")
		return b.String()
	}
	// Group by prefix (e.g. "github.*", "linear.*"), with built-ins
	// in their own group so an agent sees the factorly-provided
	// surface separately from user-installed tools.
	groups := map[string][]string{}
	for name, tc := range cfg.Tools {
		if tc.Hidden {
			continue
		}
		prefix := "(other)"
		if i := strings.Index(name, "."); i > 0 {
			prefix = name[:i]
		}
		groups[prefix] = append(groups[prefix], name)
	}
	prefixes := make([]string, 0, len(groups))
	for p := range groups {
		prefixes = append(prefixes, p)
	}
	sort.Strings(prefixes)
	// Put factorly.* group first — it's the runtime's own surface.
	prefixes = bringFactorlyFirst(prefixes)
	for _, prefix := range prefixes {
		names := groups[prefix]
		sort.Strings(names)
		fmt.Fprintf(&b, "## `%s.*`\n\n", prefix)
		for _, name := range names {
			tc := cfg.Tools[name]
			if tc.Description != "" {
				fmt.Fprintf(&b, "- `%s` — %s\n", name, tc.Description)
			} else {
				fmt.Fprintf(&b, "- `%s`\n", name)
			}
		}
		b.WriteString("\n")
	}
	b.WriteString("Call `factorly.help(tool: \"<name>\")` for per-tool parameter docs.\n")
	return b.String()
}

// renderInstalledSummary is the one-block snapshot inside the default
// view. Just counts + a one-liner, not the full list — agents that
// want detail call back with topic=tools or topic=blueprints.
func renderInstalledSummary(in Inputs) string {
	if in.Config == nil {
		return "No config loaded.\n"
	}
	toolCount, builtinCount, workflowCount, hiddenCount := 0, 0, 0, 0
	for name, tc := range in.Config.Tools {
		switch {
		case tc.Hidden:
			hiddenCount++
		case tc.Type == "workflow":
			workflowCount++
		case builtins.IsBuiltinTool(name):
			builtinCount++
		default:
			toolCount++
		}
	}
	var b strings.Builder
	fmt.Fprintf(&b, "- %d user-defined %s\n", toolCount, pluralize(toolCount, "tool", "tools"))
	fmt.Fprintf(&b, "- %d %s\n", workflowCount, pluralize(workflowCount, "workflow", "workflows"))
	fmt.Fprintf(&b, "- %d factorly %s (factorly.fetch, factorly.shell, etc.)\n", builtinCount, pluralize(builtinCount, "built-in", "built-ins"))
	if hiddenCount > 0 {
		fmt.Fprintf(&b, "- %d hidden %s (not exposed over MCP)\n", hiddenCount, pluralize(hiddenCount, "tool", "tools"))
	}
	installed := listInstalledBlueprints(in.CfgPath)
	if len(installed) > 0 {
		names := make([]string, 0, len(installed))
		for _, bp := range installed {
			name := bp.Name
			if name == "" {
				name = bp.Filename
			}
			names = append(names, name)
		}
		fmt.Fprintf(&b, "- Blueprints installed: %s\n", strings.Join(names, ", "))
	}
	return b.String()
}

// pluralize picks singular vs plural based on n.
func pluralize(n int, singular, plural string) string {
	if n == 1 {
		return singular
	}
	return plural
}

// bringFactorlyFirst reorders prefixes so "factorly" comes first if
// present. Agents reading the tools list see the runtime's own
// surface before user installations.
func bringFactorlyFirst(prefixes []string) []string {
	for i, p := range prefixes {
		if p == "factorly" && i > 0 {
			out := append([]string{"factorly"}, prefixes[:i]...)
			out = append(out, prefixes[i+1:]...)
			return out
		}
	}
	return prefixes
}
