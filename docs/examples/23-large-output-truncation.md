# Large Output Truncation

Cap tool output with `max_output` to prevent context-window blowout. Factorly keeps the most useful parts: the beginning and the end.

## Config

```yaml
# .factorly/factorly.yaml
tools:
  db.dump:
    type: cli
    command: pg_dump
    args: ["--schema-only", "{{database}}"]
    max_output: 50000
    parameters:
      - name: database
        required: true
```

## Usage

```bash
# Tool output exceeds 50,000 bytes — Factorly truncates automatically
factorly call db.dump --database myapp
```

```
CREATE TABLE users (
    id serial PRIMARY KEY,
    name text NOT NULL,
    ...
)

... (60% of output from the start) ...

--- [factorly] truncated: 124,387 → 50,000 bytes (head 60% + tail 40%) ---

... (40% of output from the end) ...

CREATE INDEX idx_orders_user_id ON orders(user_id);
GRANT SELECT ON ALL TABLES IN SCHEMA public TO readonly;
```

```bash
# Set a global fallback for all tools via environment variable
export FACTORLY_MAX_OUTPUT=100000
factorly call db.dump --database myapp
```

```bash
# Check savings in the audit log
factorly logs --stats
```

```
Output Savings:
  8 calls processed, 487.2 KB → 198.4 KB (59% saved)
```

## What happens

1. The tool runs to completion and produces its full output.
2. If the output exceeds `max_output` bytes (50,000 by default), Factorly truncates it: 60% from the head and 40% from the tail, joined by a truncation marker showing original and final sizes.
3. The head+tail split keeps both the opening context (schema definitions, headers) and the closing context (final results, summaries).
4. Per-tool `max_output` takes priority. If not set, `FACTORLY_MAX_OUTPUT` env var is the global fallback. The built-in default is 50,000 bytes.
5. Both `original_bytes` and `processed_bytes` are logged in the JSONL audit trail for tracking savings over time.

---

[← Back to Examples](README.md)
