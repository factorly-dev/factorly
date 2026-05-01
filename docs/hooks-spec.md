# Spec: `factorly hooks` — agent hook installation

> Status: **Draft** — not yet implemented.

## Problem

Agents bypass Factorly when using Bash directly — `git diff`, `npm test`, `cargo build` all run outside the proxy. No compression, no oversight, no audit logging. Only MCP tool calls go through Factorly.

Hooks solve this by rewriting Bash commands to `factorly exec` *inside* the agent's own loop. No new protocol, no shell aliases, no agent modifications. The agent calls Bash normally; the hook intercepts and routes through Factorly.

## Usage

```bash
# Install hooks for Claude Code (default)
factorly hooks install

# Install for a specific agent
factorly hooks install --agent claude-code
factorly hooks install --agent cursor
factorly hooks install --agent codex
factorly hooks install --agent gemini

# Preview without writing
factorly hooks install --dry-run

# Remove hooks
factorly hooks remove
factorly hooks remove --agent cursor
```

## What happens after installation

```
Agent: "Let me check the test results"
Agent calls: Bash { command: "go test ./..." }
                    │
                    ▼
            ┌───────────────┐
            │ PreToolUse    │
            │ hook fires    │
            └───────┬───────┘
                    │
                    ▼
            ┌───────────────────────┐
            │ factorly hooks rewrite│
            │                       │
            │ Input:  go test ./... │
            │ Output: factorly exec │
            │         -- go test    │
            │            ./...      │
            └───────────┬───────────┘
                        │
                        ▼
            ┌───────────────────────┐
            │ factorly exec         │
            │                       │
            │ • Built-in filter:    │
            │   keep PASS/FAIL/ok   │
            │ • Compression         │
            │ • Audit log           │
            │ • Exit code preserved │
            └───────────────────────┘
                        │
                        ▼
            Agent sees: "ok (all tests passed)"
            instead of: 200 lines of test output
```

## Supported agents

### Claude Code

**Hook format:** `~/.claude/settings.json`

```json
{
  "hooks": {
    "PreToolUse": [{
      "matcher": "Bash",
      "hooks": [{
        "type": "command",
        "command": "factorly hooks rewrite"
      }]
    }]
  }
}
```

**Protocol:** JSON on stdin, JSON on stdout.

Input from Claude Code:
```json
{
  "tool_name": "Bash",
  "tool_input": {
    "command": "git status"
  }
}
```

Output from `factorly hooks rewrite`:
```json
{
  "hookSpecificOutput": {
    "hookEventName": "PreToolUse",
    "permissionDecision": "allow",
    "updatedInput": {
      "command": "factorly exec -- git status"
    }
  }
}
```

### Cursor

**Hook format:** `~/.cursor/hooks.json`

```json
{
  "version": 1,
  "hooks": {
    "preToolUse": [{
      "command": "factorly hooks rewrite --agent cursor",
      "matcher": "Shell"
    }]
  }
}
```

**Protocol:** JSON on stdin, flat JSON on stdout.

Output format (different from Claude Code — no nesting):
```json
{
  "permission": "allow",
  "updated_input": {
    "command": "factorly exec -- git status"
  }
}
```

### Codex (OpenAI)

**Hook format:** `~/.codex/AGENTS.md`

Codex doesn't have JSON hooks. It uses markdown instruction files. Installation appends to `AGENTS.md`:

```markdown
## Shell Commands

When running shell commands, prefix them with `factorly exec --` for output compression and audit logging. Examples:

- `factorly exec -- git status` instead of `git status`
- `factorly exec -- npm test` instead of `npm test`
- `factorly exec -- make build` instead of `make build`

Do not prefix interactive commands (vim, ssh, psql, docker compose up) with factorly exec.
```

This is a prompt-based approach — relies on the LLM following instructions rather than programmatic interception. Less reliable but the only option for Codex.

### Gemini CLI

**Hook format:** JSON hooks similar to Claude Code.

```json
{
  "hooks": {
    "preToolUse": [{
      "command": "factorly hooks rewrite --agent gemini",
      "matcher": "run_shell_command"
    }]
  }
}
```

Output format:
```json
{
  "decision": "allow",
  "hookSpecificOutput": {
    "tool_input": {
      "command": "factorly exec -- git status"
    }
  }
}
```

## Commands

### `factorly hooks install`

```bash
factorly hooks install                    # auto-detect agent, or default to claude-code
factorly hooks install --agent cursor     # specific agent
factorly hooks install --agent codex
factorly hooks install --agent gemini
factorly hooks install --dry-run          # preview changes without writing
factorly hooks install --global           # install to user-level config (default)
factorly hooks install --local            # install to project-level config
```

**Behavior:**
1. Detect agent (or use `--agent` flag)
2. Read existing config file (or create if missing)
3. Check if hook already installed (idempotent)
4. Back up existing config (`.json.bak`)
5. Write hook entry
6. Print what was changed

**Auto-detection order:** Check which config directories exist (`~/.claude/`, `~/.cursor/`, `~/.codex/`, `~/.gemini/`). If multiple found, prompt or require `--agent`.

### `factorly hooks remove`

```bash
factorly hooks remove                     # auto-detect
factorly hooks remove --agent cursor      # specific agent
```

Removes the Factorly hook entry from the agent's config. Other hooks are preserved.

### `factorly hooks rewrite`

Called by the agent's hook system. Not user-facing.

```bash
# Agent passes JSON on stdin, factorly responds on stdout
echo '{"tool_name":"Bash","tool_input":{"command":"git status"}}' | factorly hooks rewrite
```

**Rewrite logic:**

1. Parse JSON from stdin
2. Extract command string
3. Decide: rewrite, passthrough, or block

**Passthrough (no rewrite) when:**
- Command already starts with `factorly`
- Tool is not Bash/Shell (Read, Edit, Grep, etc.)
- Command is interactive: `vim`, `nano`, `ssh`, `psql`, `docker compose up`, `npm run dev`, `top`, `htop`, `tail -f`, `less`, `man`
- `FACTORLY_HOOKS_DISABLED` env var is set

**Rewrite:** Prefix with `factorly exec --`:
- `git status` → `factorly exec -- git status`
- `npm test` → `factorly exec -- npm test`
- `curl -s https://api.example.com` → `factorly exec -- curl -s https://api.example.com`

**Block:** When shadow policy denies the command (future — requires reading config).

### `factorly hooks status`

```bash
factorly hooks status
```

Shows which agents have hooks installed:

```
  Claude Code:  ✓ installed (~/.claude/settings.json)
  Cursor:       ✗ not installed
  Codex:        ✓ installed (~/.codex/AGENTS.md)
  Gemini:       ✗ not installed
```

## Architecture

### Files

| File | Purpose |
|------|---------|
| `src/cmd/factorly/hooks_cmd.go` | `hooks install`, `hooks remove`, `hooks status` |
| `src/cmd/factorly/hooks_rewrite_cmd.go` | `hooks rewrite` — JSON stdin/stdout handler |
| `src/internal/hooks/agents.go` | Agent detection and config paths |
| `src/internal/hooks/claude.go` | Claude Code hook format |
| `src/internal/hooks/cursor.go` | Cursor hook format |
| `src/internal/hooks/codex.go` | Codex AGENTS.md format |
| `src/internal/hooks/gemini.go` | Gemini hook format |
| `src/internal/hooks/rewrite.go` | Command rewrite logic + passthrough detection |

### Interactive command detection

Maintain a list of commands that should not be rewritten (they need a TTY):

```go
var interactiveCommands = []string{
    "vim", "nvim", "nano", "emacs",
    "ssh", "psql", "mysql", "redis-cli", "mongo",
    "docker compose up", "docker compose logs -f",
    "npm run dev", "npm start", "yarn dev",
    "top", "htop", "btop",
    "tail -f", "less", "man", "watch",
    "python", "python3", "node", "irb", "rails console",
}
```

Match by prefix: if the command starts with any of these, passthrough.

### Safety

- **Atomic writes** — write to temp file, rename (same pattern as vault)
- **Backup** — save `.bak` before modifying config files
- **Idempotent** — check if hook already present before adding
- **No shell modification** — hooks live in agent config, not `.zshrc` or `.bashrc`
- **Stdin size limit** — cap at 1MB to prevent memory issues from malformed input
- **Stdout only** — use `os.Stdout.Write()` not `fmt.Println()` to avoid protocol corruption

## What you get

After `factorly hooks install`:

| Before (no hooks) | After (hooks installed) |
|---|---|
| `git status` → raw output, no log | `git status` → filtered output, logged |
| `npm test` → 200 lines, no compression | `npm test` → "ok (all tests passed)", compressed |
| `make build` → noisy, no oversight | `make build` → filtered, shadow policy applies |
| `curl https://api.com` → raw JSON | `curl https://api.com` → compacted JSON |
| No audit trail for Bash commands | Every Bash command in the audit log |

## MVP scope

**Phase 1:** Claude Code only
- `factorly hooks install` (Claude Code)
- `factorly hooks remove`
- `factorly hooks rewrite` (JSON protocol)
- Interactive command passthrough
- `--dry-run` flag

**Phase 2:** Multi-agent
- Cursor adapter
- Codex adapter (prompt-based)
- Gemini adapter
- `factorly hooks status`
- Auto-detection

**Phase 3:** Advanced
- Shadow policy enforcement in hooks (block before rewrite)
- `--local` project-level hooks
- Hook analytics (how many commands rewritten, savings from hooks)

---

[← Back to Documentation](README.md)
