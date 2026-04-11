# Spec: PreToolUse Hooks for Bash Interception

> Status: **Draft** — future ideation, not yet implemented.

## Problem

Agents bypass Factorly when using Bash directly (`git diff`, `npm test`, etc.) — only MCP tool calls go through the proxy. This means no compression, no governance, no audit logging for direct Bash usage.

## Insights

Possible to solve this with shell aliases (`alias git="_wrapper git"`) and agent PreToolUse hooks that rewrite Bash calls. Possible savings: 70-90% on git/npm/cargo output, 88% across a full session.

Shell aliases are fragile and invasive (modify `.zshrc`). **PreToolUse hooks are lighter** — agent-specific, no shell config changes, and Factorly already sits in the execution path for MCP calls.

## Proposed Design

### 1. `factorly exec` (shell exec mode)

Runs a command through Factorly's output processing pipeline:

```bash
factorly exec -- "git status"
factorly exec -- git diff --staged
```

- Executes via `sh -c per platform similar to wrap`
- Applies compression (ANSI strip, whitespace normalize, JSON compact, log dedup)
- Applies truncation (`FACTORLY_MAX_OUTPUT`, default 50000)
- Logs to JSONL audit trail
- Preserves exit code
- No config file needed — env vars control behavior

### 2. `factorly hooks install <agent>`

Installs a PreToolUse hook for the target agent:

```bash
factorly hooks install claude-code
factorly hooks install cursor
factorly hooks install --dry-run claude-code
```

For Claude Code, writes to `~/.claude/settings.json`:

```json
{
  "hooks": {
    "PreToolUse": [{
      "matcher": "Bash",
      "hooks": [{"type": "command", "command": "factorly hooks rewrite"}]
    }]
  }
}
```

### 3. `factorly hooks rewrite`

Hook handler called by the agent with JSON on stdin:

```json
{"tool_name": "Bash", "tool_input": {"command": "git status"}}
```

Returns rewritten command:

```json
{
  "hookSpecificOutput": {
    "permissionDecision": "allow",
    "updatedInput": {"command": "factorly exec -- git status"}
  }
}
```

**Passthrough (no rewrite):**
- Command already starts with `factorly`
- Tool is not Bash (Read, Edit, Grep pass through)
- Interactive commands: vim, nano, ssh, psql, docker compose up, npm run dev, top, htop, tail -f
- `FACTORLY_HOOK_DISABLED` env var is set

## Files to Create

| File | Purpose |
|------|---------|
| `src/cmd/factorly/exec_cmd.go` | `exec` command with -- similar to wrap, run command, compress, log |
| `src/cmd/factorly/hooks_cmd.go` | `hooks install` / `hooks remove` |
| `src/cmd/factorly/hooks_rewrite_cmd.go` | `hooks rewrite` JSON stdin→stdout |

## Future: Command-Specific Compression

Beyond generic compression, add patterns that understand command output structure:

| Pattern | Input | Output | Savings |
|---------|-------|--------|---------|
| git status | 500+ tokens with colors | `branch: main; +3 staged; ~2 modified; 1 untracked` | ~80% |
| git log | Full log entries | `abc1234 Fix bug (2h ago)` per line | ~70% |
| npm install | Verbose install output | `+42 packages (2.3s)` | ~85% |
| test runners | Full test output | `145 passed, 2 failed, 0 skipped` | ~90% |
| cargo build | Compile output | `compiled 42 crates, 3 warnings (4.2s)` | ~80% |

These would be new hints in `output.Compress()` or auto-detected from the command being run.

---

[← Back to README](README.md)
