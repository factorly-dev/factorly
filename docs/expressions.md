# Expressions

Expressions are used in workflow `if:` conditions and `switch:` cases to make decisions based on step outputs and input parameters.

## Syntax

Expressions are plain strings — no special wrapper syntax. They evaluate to true or false.

```yaml
steps:
  - tool: api.check
    store: status

  - tool: deploy
    if: "status == 'healthy' and branch == 'main'"
```

## Variables

Variables resolve from workflow state: stored step outputs (`store:`) and input parameters.

```yaml
parameters:
  - name: env
steps:
  - tool: get.status
    store: result
  - tool: notify
    if: "result != '' and env == 'prod'"
```

Missing variables resolve to empty string (falsy).

## Truthiness

When a bare variable is used as a condition (no comparison operator), it's evaluated for truthiness:

| Value | Truthy? |
|-------|---------|
| `"hello"` | Yes |
| `"true"` | Yes |
| `"42"` | Yes |
| `" "` (space) | Yes |
| `""` (empty) | No |
| `"0"` | No |
| `"false"` | No |
| missing variable | No |

```yaml
- tool: commit
  if: "changes"  # runs only if 'changes' is non-empty
```

## Comparisons

| Operator | Meaning |
|----------|---------|
| `==` | Equal |
| `!=` | Not equal |
| `>` | Greater than |
| `<` | Less than |
| `>=` | Greater than or equal |
| `<=` | Less than or equal |

String comparison uses exact string matching. Numeric comparison converts both sides to numbers.

```yaml
if: "status == 'healthy'"
if: "code != '200'"
if: "retries > 3"
if: "score <= 100"
```

## Boolean operators

| Operator | Meaning |
|----------|---------|
| `and` | Both sides true |
| `or` | Either side true |
| `not` | Negate |

`and` binds tighter than `or`. Use parentheses to override.

```yaml
if: "branch == 'main' and tests == 'pass'"
if: "status == 'error' or status == 'timeout'"
if: "not failed"
if: "(a or b) and c"
```

## Member access (dot notation)

Access fields in JSON-structured stored outputs using dot notation:

```yaml
steps:
  - tool: api.get
    store: response   # output: {"code": 200, "data": {"count": 5}}

  - tool: notify
    if: "response.code == '200'"

  - tool: process
    if: "response.data.count > 0"
```

Works with nested objects — each level resolves through the JSON structure.

## Functions

### `contains(haystack, needle)`

Returns true if the string contains the substring.

```yaml
if: "contains(output, 'error')"
if: "contains(log, 'FAIL') and not contains(log, 'expected')"
```

Both arguments can be variables or string literals.

### `jsonpath(data, path)`

Evaluates a JSONPath expression against a JSON string. Uses the full JSONPath spec (powered by [ohler55/ojg](https://github.com/ohler55/ojg)).

```yaml
steps:
  - tool: api.list_users
    store: response   # {"users": [{"name": "alice", "active": true}, ...]}

  - tool: notify
    if: "jsonpath(response, '$.users[0].active') == 'true'"

  - tool: alert
    if: "jsonpath(response, '$.errors') != ''"
```

**JSONPath examples:**

| Expression | Selects |
|-----------|---------|
| `$.field` | Top-level field |
| `$.data.count` | Nested field |
| `$.items[0]` | First array element |
| `$.items[*].name` | All name fields (returns JSON array) |
| `$.users[0].active` | Nested array + field |

**Behavior:**
- Single result → returns the value directly (comparable with `==`, `>`, etc.)
- Multiple results → returns JSON array string
- No match → returns nil (falsy)
- Invalid JSON or path → returns nil (falsy, fail-closed)

**Numeric comparison works:**

```yaml
if: "jsonpath(metrics, '$.cpu') > 80"
if: "jsonpath(response, '$.data.count') >= 1"
```

**Combine with contains:**

```yaml
if: "contains(jsonpath(response, '$.message'), 'success')"
```

## Parentheses

Group expressions to control evaluation order:

```yaml
if: "(status == 'error' or status == 'timeout') and env == 'prod'"
if: "not (code == '200')"
```

Without parentheses, `and` binds tighter than `or`:
- `a or b and c` means `a or (b and c)`
- `not x == 'y'` means `(not x) == 'y'` — use `not (x == 'y')` instead

## Use in workflows

### `if:` on steps

Skip a step when the condition is false:

```yaml
steps:
  - tool: git.status
    params: { command: "git status --porcelain" }
    store: changes

  - tool: git.commit
    params: { command: "git commit -am 'auto'" }
    if: "changes != ''"

  - tool: notify
    params: { message: "committed" }
    if: "changes != ''"
```

Skipped steps show `"status": "skipped"` in the workflow output.

### `require:` on steps

Stop the entire workflow when the condition is false — a gate:

```yaml
steps:
  - tool: git.status
    store: changes

  - tool: git.commit
    require: "changes != ''"
    params: { message: "auto" }

  - tool: git.push             # never reached if no changes
```

Unlike `if:` (which skips one step), `require:` halts the workflow. Remaining steps are all skipped. Status is `"completed"` — it's an intentional early exit, not an error.

### `switch:` steps

Multi-branch conditional — first matching condition executes:

```yaml
steps:
  - tool: api.health
    store: status

  - switch:
      - condition: "status == 'healthy'"
        tool: slack.post
        params: { text: "All systems operational" }
      - condition: "status == 'degraded'"
        tool: pagerduty.warn
        params: { severity: "warning" }
      - condition: "true"
        tool: pagerduty.alert
        params: { severity: "critical" }
```

Use `condition: "true"` as a default/else case.

Each switch case supports `tool`, `params`, and `store` (same as a regular step). If no condition matches, the switch step is skipped.

## Error handling

Expressions that fail to parse or evaluate return false (fail-closed). This means:

- Typos in variable names → falsy (step skipped)
- Malformed expressions → falsy
- Invalid JSON in jsonpath → falsy
- Division by zero, overflow → falsy

No panics, no workflow crashes from bad expressions.

---

[← Back to Documentation](README.md)
