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
| `status` | `success` or `error` |
| `duration_ms` | Execution time in milliseconds |
| `output` | Truncated response (max 500 chars) |
| `error` | Error message if failed (max 500 chars) |

## Location

Default: `~/.config/factorly/calls.jsonl`

Set `FACTORLY_NO_LOG=1` to disable logging.

## Security

- Log file is created with `0600` permissions (owner read/write only)
- Output and error fields are truncated to 500 characters to prevent secrets from leaking via large API responses
- Sensitive parameter values are redacted in verbose output but logged as-is in the call log (the log file is owner-only)

---

[← Back to README](../README.md)
