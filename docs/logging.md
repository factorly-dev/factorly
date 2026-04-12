# Call Log

Every tool call — whether through `factorly call` or `factorly serve` — is logged to `~/.config/factorly/calls.jsonl`:

```json
{"timestamp":"2026-04-03T09:15:32Z","interface":"cli","tool":"web.fetch","params":{"url":"https://example.com"},"status":"success","duration_ms":215,"output":"<!doctype html>..."}
```

## Fields

| Field | Description |
|-------|-------------|
| `timestamp` | ISO 8601 timestamp |
| `interface` | `cli` or `mcp` |
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
factorly logs --stats            # summary statistics
factorly logs -f                 # follow mode (tail -f)
```

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

## Security

- Log file is created with `0600` permissions (owner read/write only)
- Output and error fields are truncated to 500 characters to prevent secrets from leaking via large API responses
- Sensitive parameter values are redacted in verbose output but logged as-is in the call log (the log file is owner-only)

---

[← Back to README](../README.md)
