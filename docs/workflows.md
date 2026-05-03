# Workflows

Chain multiple tool calls into a single governed pipeline. Workflows are deterministic, repeatable, and auditable — every step is defined upfront, executes through the proxy with full oversight, and persists state for debugging.

## Quick start

```yaml
# .factorly/factorly.yaml
tools:
  daily.summary:
    type: workflow
    description: Check repos and post summary to Slack
    steps:
      - tool: github.list_repos
        params: { owner: "factorly-dev" }
        store: repos

      - tool: anthropic.ask
        params:
          prompt: "Summarize: {{repos}}"
        store: summary

      - tool: slack.post_message
        params:
          channel: "#engineering"
          text: "{{summary}}"
```

```bash
factorly call daily.summary
```

## How it works

1. Workflow receives input parameters from the caller
2. Steps execute sequentially, each through the full proxy pipeline (parameter validation → shadow policy → execute → output filter → log)
3. `store:` saves step output as a named variable for later steps
4. `{{var}}` in step params is substituted from stored outputs + input params
5. On failure, remaining steps are skipped
6. State is persisted to `.factorly/workflows/<run-id>.json` after each step
7. Output includes the full execution trace as JSON

## Config

### Step fields

| Field | Required | Description |
|-------|----------|-------------|
| `tool` | Yes* | Tool name to call (must exist in config) |
| `params` | No | Parameters to pass — supports `{{var}}` substitution |
| `store` | No | Save step output as a named variable |
| `if` | No | [Expression](expressions.md) — skip step when false |
| `switch` | No | List of conditional branches (replaces `tool`) |

*`tool` is required unless `switch` is set.

### Parameters

Workflows accept parameters like any tool:

```yaml
tools:
  deploy.staging:
    type: workflow
    description: Deploy a branch to staging
    parameters:
      - name: branch
        required: true
    steps:
      - tool: git.checkout
        params: { branch: "{{branch}}" }
      - tool: factorly.shell
        params: { command: "make deploy-staging" }
```

```bash
factorly call deploy.staging --branch feature/new-ui
```

### Variable substitution

Step params use `{{var}}` syntax. Variables resolve from (in order):

1. **Stored outputs** — previous step output saved via `store:`
2. **Input parameters** — caller-provided values
3. **Parameter defaults** — from `default:` on parameter definitions

## What can be a step?

Any tool in your config: CLI, REST, MCP, or another workflow.

```yaml
tools:
  fetch.data:
    type: rest
    base_url: https://api.example.com
    method: GET
    path: /data

  summarize:
    type: workflow
    steps:
      - tool: fetch.data           # REST tool
        store: raw
      - tool: factorly.shell       # Built-in CLI tool
        params: { command: "echo '{{raw}}' | jq '.items'" }
        store: filtered
      - tool: anthropic.ask        # REST tool (LLM)
        params: { prompt: "Summarize: {{filtered}}" }
```

## Conditionals

Steps can be conditional. See the [Expressions](expressions.md) doc for full syntax.

### Skip a step with `if:`

```yaml
steps:
  - tool: git.status
    store: changes
  - tool: git.commit
    if: "changes != ''"
```

### Branch with `switch:`

```yaml
steps:
  - tool: api.health
    store: status
  - switch:
      - condition: "status == 'healthy'"
        tool: slack.post
        params: { text: "All good" }
      - condition: "true"
        tool: pagerduty.alert
        params: { severity: "critical" }
```

First matching condition executes. Use `condition: "true"` as a default. Switch cases support `tool`, `params`, and `store`.

## How agents see workflows

Agents see workflows as a single tool — steps are opaque. The workflow appears in `factorly tools` and MCP `tools/list` with its own name, description, and parameters:

```
$ factorly tools
  daily.summary     workflow   Check repos and post summary to Slack
  deploy.staging    workflow   Deploy a branch to staging
```

When an agent calls a workflow, it passes parameters and gets back the JSON execution trace. The agent doesn't need to know how the workflow works internally, and can't bypass step-level oversight.

## Output

The workflow returns a JSON execution trace:

```json
{
  "status": "completed",
  "steps": [
    {"tool": "github.list_repos", "status": "completed", "duration_ms": 215},
    {"tool": "anthropic.ask", "status": "completed", "duration_ms": 3200},
    {"tool": "slack.post_message", "status": "completed", "duration_ms": 342}
  ],
  "result": "Message posted to #engineering"
}
```

Verbose mode (`-v`) streams step progress to stderr:

```
[workflow] daily.summary started (run: abc123)
[workflow]   1/3 github.list_repos    running...
[workflow]   1/3 github.list_repos    completed  215ms
[workflow]   2/3 anthropic.ask        running...
[workflow]   2/3 anthropic.ask        completed  3.2s
[workflow]   3/3 slack.post_message   running...
[workflow]   3/3 slack.post_message   completed  342ms
[workflow] daily.summary completed (3 steps, 3.8s)
```

## Error handling

Workflows stop on first error. Remaining steps are marked `skipped`:

```json
{
  "status": "failed",
  "error": "step 2 (anthropic.ask): API error 429",
  "steps": [
    {"tool": "github.list_repos", "status": "completed"},
    {"tool": "anthropic.ask", "status": "failed", "error": "API error 429"},
    {"tool": "slack.post_message", "status": "skipped"}
  ]
}
```

## Oversight

Each step is a separate proxy call — oversight applies per step individually:

- Shadow policy (deny/confirm/rate limit) checked per step
- Each step gets its own audit log entry
- Output filtering and compression applied per step
- Parameter validation runs on each step's tool

```yaml
tools:
  deploy.prod:
    type: workflow
    steps:
      - tool: factorly.shell
        params: { command: "git checkout main" }
      - tool: factorly.shell
        params: { command: "make deploy-prod" }
        # Triggers confirm because factorly.shell has shadow.confirm: true
```

## State persistence

Each run persists state to `.factorly/workflows/<run-id>.json`, updated after each step:

```json
{
  "workflow": "report.weekly",
  "run_id": "a1b2c3d4",
  "status": "completed",
  "started_at": "2026-05-01T10:00:00Z",
  "completed_at": "2026-05-01T10:00:04Z",
  "steps": [
    {"index": 0, "tool": "github.list_prs", "status": "completed", "duration_ms": 215},
    {"index": 1, "tool": "anthropic.ask", "status": "completed", "duration_ms": 3200},
    {"index": 2, "tool": "gmail.send_message", "status": "completed", "duration_ms": 342}
  ],
  "variables": {
    "team": "platform",
    "prs": "[{\"title\": \"Add workflows\", ...}]",
    "report": "This week the platform team merged 3 PRs..."
  }
}
```

Useful for debugging failed workflows — see exactly which step failed, what data was available, and what the error was.

## State machine

### Step states

```
pending → running → completed
                  → failed
                  → skipped (remaining steps after a failure)
```

### Workflow states

```
running → completed (all steps succeeded)
        → failed (a step failed or was denied)
```

## Examples

### CI pipeline

```yaml
tools:
  ci.run:
    type: workflow
    description: Run CI pipeline
    steps:
      - tool: factorly.shell
        params: { command: "go vet ./..." }
        store: vet
      - tool: factorly.shell
        params: { command: "go test ./..." }
        store: test
      - tool: slack.post_message
        params:
          channel: "#ci"
          text: "CI passed: {{test}}"
```

### Report generation

```yaml
tools:
  report.weekly:
    type: workflow
    description: Generate weekly report from GitHub and email it
    parameters:
      - name: team
        default: engineering
    steps:
      - tool: github.list_prs
        params: { state: "closed", since: "7d" }
        store: prs
      - tool: anthropic.ask
        params:
          prompt: "Write a weekly summary for {{team}} from: {{prs}}"
        store: report
      - tool: gmail.send_message
        params:
          to: "{{team}}@company.com"
          subject: "Weekly Report"
          body: "{{report}}"
```

### Deploy with approval

```yaml
tools:
  deploy.staging:
    type: workflow
    description: Deploy to staging with oversight
    parameters:
      - name: branch
        required: true
        pattern: "^[a-zA-Z0-9/_-]+$"
    steps:
      - tool: factorly.shell
        params: { command: "git checkout {{branch}}" }
      - tool: factorly.shell
        params: { command: "make test" }
        store: tests
      - tool: factorly.shell
        params: { command: "make deploy-staging" }
```

## Future

- Conditionals (`if:` on steps)
- Parallelism (`parallel: [step1, step2]`)
- Retry logic (`retry: 3` on steps)
- Loop/iteration (`foreach:`)
- Scheduled workflows (cron)

---

[← Back to Documentation](README.md)
