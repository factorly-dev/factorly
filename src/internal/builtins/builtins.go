// Copyright 2026 Jordan Sherer <hi@jordansherer.com>
// SPDX-License-Identifier: gpl

package builtins

import (
	"github.com/factorly-dev/factorly/internal/config"
)

// Options controls which built-in tools are registered.
type Options struct {
	Mode string // "stdio" or "http" — local tools only register in stdio mode
}

// Register adds built-in tools to the config. Built-ins execute in-process
// via the builtin provider — no subprocess needed for most operations.
func Register(cfg *config.Config, opts Options) {
	if cfg.DisableBuiltins {
		return
	}
	if cfg.Tools == nil {
		cfg.Tools = make(map[string]config.ToolConfig)
	}

	disabled := make(map[string]bool, len(cfg.DisabledBuiltins))
	for _, name := range cfg.DisabledBuiltins {
		disabled[name] = true
	}

	register := func(name string, tc config.ToolConfig) {
		if disabled[name] {
			return
		}
		// If the user already declared this builtin in their config
		// with a Shadow block, preserve their shadow over the builtin's
		// default. Lets users tighten or extend a builtin's oversight
		// (e.g., shadow.confirm: true on factorly.fetch, shadow.max_calls
		// on factorly.code) without losing the builtin's intrinsic
		// config (Type, Parameters, Description, MaxOutput, Compress).
		if existing, ok := cfg.Tools[name]; ok && existing.Shadow != nil {
			tc.Shadow = mergeShadow(tc.Shadow, existing.Shadow)
		}
		cfg.Tools[name] = tc
	}

	// Universal (all modes) — runs server-side where credentials live
	register("factorly.fetch", config.ToolConfig{
		Type:        "builtin",
		Description: "Fetch a URL (overseen, logged, compressed)",
		Compress:    []string{"all"},
		MaxOutput:   50000,
		Parameters: []config.ParamConfig{
			{Name: "url", Description: "URL to fetch", Required: true},
		},
	})

	// Local only (stdio mode) — runs on the agent's machine
	if opts.Mode == "http" {
		return
	}

	register("factorly.shell", config.ToolConfig{
		Type:        "builtin",
		Description: "Run a shell command (overseen, logged, compressed)",
		Compress:    []string{"all"},
		MaxOutput:   50000,
		Shadow:      &config.ShadowConfig{Confirm: true},
		Parameters: []config.ParamConfig{
			{Name: "command", Description: "Shell command to execute", Required: true},
		},
	})

	register("factorly.file.read", config.ToolConfig{
		Type:        "builtin",
		Description: "Read a local file (overseen, logged, compressed)",
		Compress:    []string{"all"},
		MaxOutput:   50000,
		Parameters: []config.ParamConfig{
			{Name: "path", Description: "File path to read", Required: true},
		},
	})

	register("factorly.file.write", config.ToolConfig{
		Type:        "builtin",
		Description: "Write content to a local file (overseen, logged, confirmable)",
		Shadow:      &config.ShadowConfig{Confirm: true},
		Parameters: []config.ParamConfig{
			{Name: "path", Description: "File path to write", Required: true},
			{Name: "content", Description: "Content to write", Required: true},
		},
	})

	register("factorly.clipboard", config.ToolConfig{
		Type:        "builtin",
		Description: "Copy text to the system clipboard (overseen, logged, confirmable)",
		Shadow:      &config.ShadowConfig{Confirm: true},
		Parameters: []config.ParamConfig{
			{Name: "text", Type: "text", Description: "Text to copy to clipboard", Required: true},
		},
	})

	// factorly.code runs an agent-supplied Go script through the same
	// yaegi sandbox used by `type: code` tools. Scripts can call other
	// registered tools via factorly.Call. Inner calls go through the
	// proxy's full shadow/vault/audit machinery — the script itself
	// can only reach what's already exposed as a tool.
	register("factorly.code", config.ToolConfig{
		Type:        "builtin",
		Description: factorlyCodeDescription,
		Compress:    []string{"all"},
		MaxOutput:   50000,
		Parameters: []config.ParamConfig{
			{Name: "code", Type: "text", Required: true, Description: factorlyCodeParamCodeDescription},
			{Name: "params", Type: "json", Description: factorlyCodeParamParamsDescription},
		},
	})

	// Store access for agents is intentionally NOT exposed as builtin
	// tools. Agents that want to read or write the workspace store do
	// so from inside a `factorly.code` script via the `factorly.Store`
	// SDK handle (see factorlyCodeDescription). The CLI keeps its
	// `factorly store {get,set,...}` subcommands for human use.
	//
	// Rationale: the per-method builtin tools (factorly.store.save,
	// .get, .list, .search, .delete) bloated the tool list with five
	// entries that mirrored what one SDK handle does more ergonomically.
	// If MCP-direct store access turns out to be a real need we can
	// re-add a thinner surface; for now we trim and see if anyone
	// notices.
}

// factorlyCodeDescription teaches an MCP agent enough to compose a
// working script without trial-and-error. Kept in one place so the
// reference docs and the agent-facing string stay in sync.
const factorlyCodeDescription = `Execute a Go script that can call other registered factorly tools.

The script runs in a sandboxed go interpreter. It must declare:

    package main
    import "factorly"  // host SDK; see below
    func Run(params map[string]string) (any, error) { ... }

The Run function's return value becomes this tool's output:
  - string → passes through verbatim
  - anything else → JSON-marshaled
  - nil → empty
  - returning an error → ExitCode 1 with the error message

Allowed imports: fmt, strings, strconv, time, encoding/json, errors,
and "factorly" (host SDK). Everything else (os, net, reflect, unsafe,
io, http, etc.) is denied at compile time.

The factorly host package exposes:
  factorly.Call(name, params map[string]string) (*Result, error)
      Call another registered tool. Returns (*Result, nil) on
      success (check res.IsError() for tool-level failure with
      res.Error / res.ExitCode) or (nil, err) on infrastructure
      failure.
  factorly.ListTools() []ToolInfo
      Discovery API: returns a snapshot of every currently-callable
      tool. Use this before composing a script when you're not sure
      which tool name to call. The snapshot is stable for the
      duration of one Run; hidden and shadow-denied tools are
      excluded so what you see is what you can call.

      ToolInfo {
          Name        string         // pass to factorly.Call
          Description string         // what the tool does
          Parameters  []ParamInfo    // what to pass
      }
      ParamInfo {
          Name        string         // map key for factorly.Call's params
          Type        string         // "string" (default), "boolean", "integer", "json", "text"
          Required    bool
          Description string
          Default     string
      }

      Discovery example — find a tool by name prefix and inspect its params:

          for _, t := range factorly.ListTools() {
              if strings.HasPrefix(t.Name, "gmail.") {
                  // t.Name, t.Description, t.Parameters available here
              }
          }

  factorly.Store
      First-class handle for the workspace store — the canonical way
      for scripts to read and write cross-run state (research
      caching, user preferences, last-known-good values). Reads
      cascade workspace → project; writes target the active tier.
      Refresh-on-read keeps frequently-touched entries alive.

          factorly.Store.Get(key string) (string, error)
              Returns factorly.ErrStoreNotFound when the key is
              missing or expired.
          factorly.Store.Set(key, value string) error
              Default TTL (30 days, refresh-on-read).
          factorly.Store.SetWithTTL(key, value string, ttl time.Duration) error
              Explicit TTL; pass 0 for never-expire.
          factorly.Store.Delete(key string) error
              Idempotent — no error if the key is absent.
          factorly.Store.List() ([]string, error)
              Sorted list of keys in the active tier (cascaded).

      Example — dedupe research across runs:

          key := "research:url:" + p["url"]
          if seen, err := factorly.Store.Get(key); err == nil {
              return "already researched: " + seen, nil
          }
          // ... do the research ...
          factorly.Store.Set(key, summary)

Example — fetch a URL and return part of the JSON response:

    package main
    import (
        "encoding/json"
        "errors"
        "factorly"
    )
    func Run(p map[string]string) (any, error) {
        res, err := factorly.Call("factorly.fetch", map[string]string{"url": p["url"]})
        if err != nil { return nil, err }
        if res.IsError() { return nil, errors.New(res.Error) }
        var body struct{ Message string ` + "`" + `json:"message"` + "`" + ` }
        if err := json.Unmarshal([]byte(res.Output), &body); err != nil { return nil, err }
        return body.Message, nil
    }

Use factorly.ListTools() first if you need to discover which tools
are available. Each inner factorly.Call is subject to that tool's
shadow rules (confirm, rate limit, deny). The script itself runs
under a max_calls cap (default 100) and any timeout configured on
this tool.`

const factorlyCodeParamCodeDescription = `The full Go source for the script. Must start with a 'package <name>' declaration and export a Run function with signature:

    func Run(params map[string]string) (any, error)

Allowed imports: fmt, strings, strconv, time, encoding/json, errors, and "factorly" (host SDK exposing Call and ListTools). Anything else fails at compile time.`

const factorlyCodeParamParamsDescription = `JSON object whose keys/values become the params map passed to Run. Values are stringified — use strconv.Atoi / strconv.ParseBool inside the script if you need numeric or boolean types. Empty/omitted is fine; the script will see an empty map.`

// mergeShadow combines the builtin's default Shadow with a user-supplied
// one. User fields win when explicitly set; otherwise builtin defaults
// fill in. Used so a user can write `factorly.code: { shadow: { max_calls:
// 200 } }` and get the builtin's other shadow defaults preserved.
//
// Nil safety: if either side is nil, the other is returned (or a clone
// thereof). If both are nil, returns nil.
func mergeShadow(base, user *config.ShadowConfig) *config.ShadowConfig {
	if user == nil {
		return base
	}
	if base == nil {
		return user
	}
	merged := *base // shallow copy of the builtin default
	if len(user.Deny) > 0 {
		merged.Deny = user.Deny
	}
	if user.Confirm != nil {
		merged.Confirm = user.Confirm
	}
	if user.RateLimit != "" {
		merged.RateLimit = user.RateLimit
	}
	if user.MaxCalls != 0 {
		merged.MaxCalls = user.MaxCalls
	}
	if len(user.LogParams) > 0 {
		merged.LogParams = user.LogParams
	}
	if len(user.AllowPatterns) > 0 {
		merged.AllowPatterns = user.AllowPatterns
	}
	if len(user.AllowPaths) > 0 {
		merged.AllowPaths = user.AllowPaths
	}
	if len(user.AllowURLs) > 0 {
		merged.AllowURLs = user.AllowURLs
	}
	return &merged
}
