# Store

The **store** is the fourth flavor of workspace data, alongside vault, env, and auth. Where vault holds user-managed secrets and env holds user-managed config, store holds **agent-managed** scratchpad — cross-run state the agent maintains itself.

| Type | Purpose | Writer |
|---|---|---|
| Vault | Secrets (encrypted) | User |
| Env | Configuration (plain) | User |
| Auth | OAuth tokens | Factorly |
| **Store** | Agent scratchpad / cross-run state | **Agent** |

Examples of what belongs in the store:

- "I researched these URLs already, don't re-fetch"
- "The Trello board ID for Factorly is `6a0d39c5…`"
- "Last successful deployment SHA: `8e3c…`"
- "User prefers brief responses, no emojis"

## When to use store vs. the alternatives

| You have... | Use |
|---|---|
| A secret (API key, token) | **Vault** — encrypted at rest |
| A configuration value the user sets | **Env** or workspace `vars:` |
| An OAuth token | **Auth** — Factorly manages refresh |
| Something the agent learned and wants to remember | **Store** |
| A vector embedding or RAG-style memory | Build your own tool — store is deliberately not that |

## CLI

Mirrors `factorly vault`:

```bash
factorly store set <key> <value> [--ttl 7d]
factorly store get <key>
factorly store search <query>           # substring match, case-insensitive
factorly store list
factorly store delete <key>
factorly store history <key>            # audit-log entries touching this key
```

All subcommands accept `--global` to target `~/.config/factorly/store.db` instead of the project store, and `--workspace <name>` to target a specific workspace store. See [Workspace scoping](#workspace-scoping) below.

Default TTL is **30 days with refresh-on-read** — entries the agent keeps consulting stay alive indefinitely. Pass `--ttl 0` for never-expire; pass `--ttl 5m` for short-lived state.

```bash
factorly store set --ttl 7d research:url:example.com "summary..."
factorly store set --ttl 0 deployment:last-sha 8e3c1f
factorly store set ephemeral:retry-count 3   # uses default 30d TTL
```

## Reference syntax

Tool configs can reference store values with `{{store:KEY}}`, the same pattern as `{{vault:KEY}}` and `{{env:VAR}}`:

```yaml
tools:
  greet:
    type: cli
    command: echo
    args: ["Hello {{store:USER_NAME|world}}"]
```

The agent writes `USER_NAME` via `factorly.store.save`, and downstream tool calls see it substituted at call time. This is a **bidirectional channel** between agent and config — see [Security](#security) below.

## Workspace scoping

The store is workspace-scoped, mirroring vault. Each workspace has its own bbolt file:

```
.factorly/store.db                      # project-scoped (default)
.factorly/workspaces/staging/store.db   # workspace "staging"
~/.config/factorly/store.db             # global fallback
```

When `--workspace staging` is active, **writes** go to the workspace store. **Reads** cascade: workspace store first, then project store. This means project-wide entries are visible inside a workspace context unless explicitly shadowed by a same-named workspace entry.

```bash
# Without --workspace: writes go to project store
factorly store set shared:value abc

# With --workspace staging: writes go to .factorly/workspaces/staging/store.db
factorly --workspace staging store set workspace-only-value xyz

# Reads in staging see both:
factorly --workspace staging store list
# → workspace-only-value
# → shared:value
```

### Targeting the global store explicitly

By default, `factorly store` writes to the project tier whenever a `.factorly/` directory exists in cwd. To write to the global store from inside a project, pass `--global`:

```bash
# Inside a project — would normally hit .factorly/store.db
factorly store --global set deployment:last-sha 8e3c1f
factorly store --global get deployment:last-sha
factorly store --global list
```

Precedence (mirrors `factorly vault`): `--global` > `--workspace` > project default > global fallback.

## Agent-facing builtins

Four built-in tools mirror the CLI, callable from `factorly.code` scripts or MCP clients:

| Tool | Parameters | Use |
|---|---|---|
| `factorly.store.save` | `key`, `value`, `ttl?` | Save a key/value pair |
| `factorly.store.search` | `query` | Substring search keys |
| `factorly.store.list` | — | List all keys |
| `factorly.store.delete` | `key` | Remove a key |

Example agent script:

```go
package main
import "factorly"

func Run(p map[string]string) (any, error) {
    factorly.Call("factorly.store.save", map[string]string{
        "key":   "research:url:example.com",
        "value": "summarized: ...",
        "ttl":   "7d",
    })
    return nil, nil
}
```

Default shadow is permissive (`confirm: false`) — agent ownership is the whole point. Operators can tighten via tool config:

```yaml
tools:
  factorly.store.save:
    shadow:
      confirm: true   # require approval for every agent write
```

## Storage backend

The store uses [`go.etcd.io/bbolt`](https://github.com/etcd-io/bbolt) — pure Go, single-writer/many-readers via mmap, ACID transactions, no separate process. Each scope is one bbolt file; the bbolt file lock serializes writers across MCP and CLI processes within a scope.

**File caps:** 10KB per entry, ~1000 entries per scope, ~10MB total per scope. These aren't enforced in v1 but are the design budget — anything bigger should live in a real database the user owns.

## Audit log

Every Set/Delete writes a JSONL audit entry with `Interface: "store"`, `Tool: "store.save"` / `store.delete`. Get and Search are **not** logged — they're high-frequency and low-information.

```bash
factorly store history research:url:example.com
# 2026-05-20 16:15:23  save      success
# 2026-05-21 09:02:41  save      success
# 2026-05-22 11:30:11  delete    success
```

## What store is NOT

Deliberately not building, ever:

- **Vector embeddings, semantic search.** Different product. If you need it, build a tool that talks to your preferred vector backend.
- **Tags, namespaces, hierarchical buckets.** Use key prefixes (`research:url:<url>`) for implicit namespacing.
- **Memory-framework concepts** (episodic/semantic/consolidation). Store is a primitive; frameworks build on it.
- **Rich documents.** Values are strings; JSON-encode if you need structure.
- **Cross-workspace sharing.** Workspaces are isolated by design.
- **Backup/restore tooling.** Single file — back up with your project.

The principle: **the smallest useful primitive that lets the agent remember things between runs.** Anything fancier, the user defines their own tool.

## Security

`{{store:KEY}}` resolves at tool-call time, which means agent-written values flow into tool configs. This is a **bidirectional channel** between agent and config — be aware of three implications:

1. **Audit-log every write.** This already happens; `factorly store history <key>` shows the change log.
2. **Don't reference `{{store:KEY}}` in privilege-granting locations.** Specifically, avoid using it in `shadow.allow_patterns`, file path overrides, or anywhere an attacker-controlled value would escalate capability. Use vault or env for those.
3. **The agent can mutate config behavior.** If a tool reads `base_url: "{{store:API_URL}}"` and the agent writes `API_URL`, the next call goes to whatever URL the agent chose. Design tool configs with this in mind, or restrict store writes via `factorly.store.save` shadow rules.

A v2 privilege-escalation guard is on the roadmap — for v1, the rule is "doc-warn and trust the operator's config review."

[← Back to Documentation](README.md)
