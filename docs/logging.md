# Call Log

Every tool call — whether through `factorly call` or `factorly serve` — is logged to `~/.config/factorly/calls.jsonl`:

```json
{"timestamp":"2026-04-03T09:15:32Z","interface":"cli","tool":"web.fetch","params":{"url":"https://example.com"},"status":"success","duration_ms":215,"output":"<!doctype html>..."}
```

## Fields

| Field | Description |
|-------|-------------|
| `timestamp` | ISO 8601 timestamp |
| `interface` | `cli`, `mcp`, `exec`, or `vault` |
| `tool` | Tool name |
| `params` | Parameters passed to the tool |
| `status` | `success`, `error`, or `blocked` |
| `duration_ms` | Execution time in milliseconds |
| `output` | Truncated response (max 500 chars) |
| `error` | Error message if failed (max 500 chars) |
| `shadow_action` | Governance outcome: `allowed`, `denied`, `confirmed`, `rate_limited`, `loop_warning`, `loop_blocked` |
| `highlight_params` | Selected params from `log_params` config (for audit filtering) |
| `agent_id` | MCP session identifier (tracks per-agent activity) |
| `original_bytes` | Output size before compression/truncation |
| `processed_bytes` | Output size after compression/truncation |
| `prev_hash` | SHA-256 hash of the previous log entry (hash chain) |
| `hash` | SHA-256 hash of this entry (for tamper detection) |

## Location

Default: `~/.config/factorly/calls.jsonl`

Set `FACTORLY_NO_LOG=1` to disable logging.

## Viewing Logs

Use `factorly logs` to view and query the audit log:

```bash
factorly logs                    # last 20 entries
factorly logs -n 50              # last 50 entries
factorly logs --tool github      # filter by tool name
factorly logs --status blocked   # filter by status
factorly logs --detail           # show full entry details
factorly logs -f                 # follow mode (tail -f)
factorly logs stats              # summary statistics (includes chain integrity)
factorly logs verify             # verify hash chain integrity
factorly logs repair             # repair broken hash chain
```

### Examples

**Recent calls** — `factorly logs` shows the most recent entries with tool name, status, and duration:

```
$ factorly logs -n 5
  2026-04-17 09:15:32  github.list_repos      success   215ms
  2026-04-17 09:15:34  github.get_repo         success   187ms
  2026-04-17 09:15:40  exec                    success   312ms
  2026-04-17 09:15:42  clipboard.copy          success    45ms
  2026-04-17 09:15:50  factorly.shell          blocked     0ms
```

**Summary statistics** — `factorly logs stats` gives a quick overview of call volume, success rates, most-used tools, output savings, and blocked calls:

```
$ factorly logs stats
  Log: ~/.config/factorly/calls.jsonl (194 entries)

  By Status:
    success    162  (83.5%)
    error      29  (14.9%)
    blocked    3  (1.5%)

  By Tool (top 10):
    github.list_repos              42
    slack.post_message             31
    github.get_repo                28
    exec                           22
    stripe.list_charges            15
    gmail.send_message             12
    linear.list_issues             9
    github.profile                 8
    jira.get_issue                 6
    notion.query_database          5

  Output Savings:
    21 calls processed, 34.4 KB → 33.4 KB (3% saved)

  Blocked Calls:
    3 total (3 denied)
```

**Filter by status** — find all blocked calls to investigate governance triggers:

```
$ factorly logs --status blocked
  2026-04-17 08:02:11  factorly.shell          blocked     0ms
  2026-04-17 08:14:55  factorly.read_file      blocked     0ms
  2026-04-17 09:15:50  factorly.shell          blocked     0ms
```

**Filter by tool** — narrow down to calls for a specific tool:

```
$ factorly logs --tool github
  2026-04-17 09:15:32  github.list_repos      success   215ms
  2026-04-17 09:15:34  github.get_repo         success   187ms
  2026-04-17 09:16:01  github.profile          success   142ms
```

## Vault audit trail

Vault operations (`get`, `set`, `delete`, `list`) are logged with `interface: "vault"`. Secret values are never logged — only the operation and key name.

```bash
factorly logs --tool vault
```

```
  12:34:05  vault.set     success  —
  12:34:10  vault.get     success  —
  12:35:02  vault.list    success  —
  12:35:15  vault.delete  success  —
```

The `params` field contains the key name (e.g., `{"key": "GITHUB_TOKEN"}`) but never the secret value.

## Output savings

When output processing is enabled (compression or truncation), the log records `original_bytes` and `processed_bytes` for each call. Query savings with `jq`:

```bash
cat ~/.config/factorly/calls.jsonl | jq 'select(.original_bytes) | {tool, saved: (.original_bytes - .processed_bytes)}'
```

Run `factorly tools status` for an aggregate output savings summary across all tools.

## Agent identity

MCP sessions are tracked by `agent_id` (session ID). Each agent gets independent rate-limit quotas. Use the field to filter logs per agent:

```bash
cat ~/.config/factorly/calls.jsonl | jq 'select(.agent_id == "session-abc")'
```

## Hash chain integrity

Every log entry includes a SHA-256 hash chain. Each entry's `hash` field is computed from its `prev_hash` (the previous entry's hash) plus the entry payload. The first entry chains from a zero hash.

This creates a tamper-evident log — modifying or deleting any entry breaks the chain from that point forward.

```bash
# Verify the full chain
$ factorly logs verify
Chain verified: 194 entries OK

# Stats also show chain status
$ factorly logs stats
  ...
  Hash Chain:
    194 entries verified
```

Entries written before hash chaining was added are skipped during verification.

### Repairing a broken chain

If the chain is broken (e.g., from an upgrade or a bug), repair it:

```bash
$ factorly logs repair
Chain reset marker appended. Run --verify to confirm.

$ factorly logs verify
Chain verified: 5 entries OK (224 entries before chain reset skipped)
```

Repair appends a `chain_reset` marker to the end of the log. The marker's `prev_hash` is a SHA-256 hash of the entire file contents, binding it to the complete history. New entries chain from the marker. Existing entries are never modified.

## Security

- Log file is created with `0600` permissions (owner read/write only)
- Output and error fields are truncated to 500 characters to prevent secrets from leaking via large API responses
- Sensitive parameter values are redacted in verbose output but logged as-is in the call log (the log file is owner-only)
- Hash chain provides tamper-evident audit trail (AARM R5)

---

[← Back to README](../README.md)
