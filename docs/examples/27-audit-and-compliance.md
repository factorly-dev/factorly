# Audit and Compliance

Use `log_params` for compliance tagging, vault audit trails, and log queries to track every tool call across your team.

## Config

```yaml
# .factorly/factorly.yaml
tools:
  customer.lookup:
    type: rest
    base_url: https://api.internal.com
    method: GET
    path: /customers/{{customer_id}}
    auth:
      type: bearer
      token: "{{vault:INTERNAL_API_KEY}}"
    parameters:
      - name: customer_id
        in: path
        required: true
      - name: reason
        required: true
    shadow:
      log_params: [customer_id, reason]
      rate_limit: 50/hour
```

## Usage

```bash
# Call the tool — customer_id and reason are highlighted in the audit log
factorly call customer.lookup --customer_id C-9042 --reason "support ticket #4817"

# View vault operations
factorly logs --tool vault
```

```
  09:12:01  vault.set     success  —   key=INTERNAL_API_KEY
  09:12:15  vault.get     success  —   key=INTERNAL_API_KEY
  09:14:30  vault.list    success  —
```

```bash
# View aggregate stats
factorly logs --stats
```

```
Log: ~/.config/factorly/audit.jsonl (194 entries)

By Status:
  success    162  (83.5%)
  error      29  (14.9%)
  blocked    3  (1.5%)

Blocked Calls:
  3 total (3 denied)
```

```bash
# Filter by status to find oversight triggers
factorly logs --status blocked
```

```
  08:02:11  factorly.shell      blocked   0ms
  08:14:55  factorly.read_file  blocked   0ms
  09:15:50  factorly.shell      blocked   0ms
```

### JSONL entry structure

Each call produces one line in the audit log (`<project>/.factorly/audit.jsonl` for project configs, `~/.config/factorly/audit.jsonl` for the global fallback):

```json
{
  "timestamp": "2026-04-18T09:14:32Z",
  "interface": "mcp",
  "tool": "customer.lookup",
  "params": {"customer_id": "C-9042", "reason": "support ticket #4817"},
  "status": "success",
  "duration_ms": 187,
  "output": "{\"id\":\"C-9042\",\"name\":\"Acme Corp\"...}",
  "shadow_action": "allowed",
  "highlight_params": {"customer_id": "C-9042", "reason": "support ticket #4817"},
  "agent_id": "session-abc",
  "original_bytes": 1240,
  "processed_bytes": 1240
}
```

## What happens

1. `log_params: [customer_id, reason]` copies those parameter values into a `highlight_params` field in the audit log. This makes compliance-sensitive data easy to query without parsing the full `params` object.
2. Vault operations (`set`, `get`, `delete`, `list`) are logged with `interface: "vault"`. The key name is recorded but the secret value is never logged.
3. `factorly logs --status blocked` surfaces every denied call — useful for reviewing what agents attempted but oversight rules prevented.
4. `factorly logs --tool vault` shows all vault access, letting you audit who accessed which secrets and when.
5. The log file is created with `0600` permissions (owner read/write only). Output and error fields are truncated to 500 characters to limit secret exposure.

---

[← Back to Examples](README.md)
