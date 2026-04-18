# Output Compression

Reduce tool output size before it reaches the agent using `compress`. This saves tokens and keeps context windows clean.

## Config

```yaml
# .factorly/factorly.yaml
tools:
  api.users:
    type: rest
    base_url: https://api.example.com
    method: GET
    path: /users
    compress: [json]

  build.logs:
    type: cli
    command: cat
    args: ["build.log"]
    compress: [logs]

  ci.output:
    type: cli
    command: ./run-tests.sh
    compress: [all]
```

## Usage

```bash
# JSON compression — pretty-printed JSON is compacted
factorly call api.users
```

**Before** (594 bytes):
```json
{
  "users": [
    {
      "id": 1,
      "name": "Alice",
      "email": "alice@example.com"
    },
    {
      "id": 2,
      "name": "Bob",
      "email": "bob@example.com"
    }
  ]
}
```

**After** (109 bytes):
```json
{"users":[{"id":1,"name":"Alice","email":"alice@example.com"},{"id":2,"name":"Bob","email":"bob@example.com"}]}
```

```bash
# Log compression — repeated lines are collapsed
factorly call build.logs
```

**Before** (12 lines):
```
[INFO] Compiling module auth...
[INFO] Compiling module auth...
[INFO] Compiling module auth...
[INFO] Compiling module auth...
[INFO] Compiling module api...
[WARN] Deprecated function in api/handler.go:42
[INFO] Compiling module api...
[INFO] Compiling module api...
[INFO] Compiling module api...
[INFO] Linking...
[INFO] Linking...
[INFO] Build complete.
```

**After** (6 lines):
```
[INFO] Compiling module auth... (×4)
[INFO] Compiling module api...
[WARN] Deprecated function in api/handler.go:42
[INFO] Compiling module api... (×3)
[INFO] Linking... (×2)
[INFO] Build complete.
```

```bash
# Check aggregate savings
factorly logs --stats
```

```
Output Savings:
  21 calls processed, 34.4 KB → 33.4 KB (3% saved)
```

## What happens

1. `compress: [json]` — detects JSON output and compacts whitespace. Indented API responses become single-line JSON.
2. `compress: [logs]` — collapses consecutive repeated lines into one line with a repeat count, and strips ANSI color codes.
3. `compress: [all]` — applies both JSON compaction and log collapsing, plus ANSI stripping.
4. Original and processed byte counts are recorded in the audit log (`original_bytes`, `processed_bytes`), visible in `factorly logs --stats`.

---

[← Back to Examples](README.md)
