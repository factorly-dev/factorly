# Spec: `type: workflow` — governed sequential tool pipelines

> Status: **Implemented** — sequential pipelines with variable passing, state persistence, and per-step oversight.

## Overview

A workflow is a new tool type — a sequence of governed tool calls with variable passing. Defined in the same `tools:` section as any other tool, callable via `factorly call`, and exposed to agents via MCP.

```yaml
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

Deterministic, repeatable, auditable. No LLM decision-making — every step is defined upfront. Each step executes through the proxy with full governance (shadow policy, rate limiting, logging, output filters).

## Config

### Step definition

```go
type StepConfig struct {
    Tool   string            `yaml:"tool"`
    Params map[string]string `yaml:"params,omitempty"`
    Store  string            `yaml:"store,omitempty"` // variable name for output
}
```

Steps are added to `ToolConfig`:
```go
Steps []StepConfig `yaml:"steps,omitempty"` // for type: workflow
```

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

Step params use `{{var}}` syntax. Variables come from:

1. **Input parameters** — caller-provided (`{{branch}}`)
2. **Stored outputs** — previous step output saved via `store:` (`{{repos}}`)
3. **Parameter defaults** — from `default:` on parameter definitions

## State machine

Each workflow run is a state machine with persisted state.

### Step states

```
pending → running → completed
                  → failed
                  → skipped (remaining steps after failure)
```

### Workflow states

```
running → completed (all steps done)
        → failed (a step failed)
```

### Runtime state

```go
type WorkflowState struct {
    WorkflowName string            `json:"workflow"`
    RunID        string            `json:"run_id"`
    Status       string            `json:"status"`
    Steps        []StepState       `json:"steps"`
    Variables    map[string]string `json:"variables"`
    StartedAt    time.Time         `json:"started_at"`
    CompletedAt  time.Time         `json:"completed_at,omitempty"`
    Error        string            `json:"error,omitempty"`
}

type StepState struct {
    Index    int           `json:"index"`
    Tool     string        `json:"tool"`
    Status   string        `json:"status"`
    Duration time.Duration `json:"duration_ms,omitempty"`
    Output   string        `json:"output,omitempty"`
    Error    string        `json:"error,omitempty"`
}
```

### Persisted state

After each step completes, state is written to `.factorly/workflows/<run-id>.json` via atomic write (tmp + rename). This enables:

- **Resume** — `factorly call workflow --resume <run-id>` skips completed steps
- **Inspect** — view step-by-step progress of a running or completed workflow
- **Audit** — full record of what happened, when, and what failed

## Governance

Each step is a separate `proxy.Execute` call — so each step individually:

- Gets its own shadow policy check (deny/confirm/rate limit)
- Gets its own audit log entry
- Gets output filtering/compression
- Shares the workflow's agent ID (passed via context)

The workflow tool itself also gets a top-level log entry.

## Events + observability

### Audit logging

- Each step logged via proxy (automatic — tool call entries)
- Workflow start/complete/fail logged as `interface: "workflow"` entries

### Verbose output

In verbose mode, state transitions stream to stderr:

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

### Output

The workflow output includes the full execution trace:

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

The agent sees the full execution trace — not just the final output.

## Error handling

Stop on first error. Remaining steps are marked `skipped`. State is persisted:

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

## Architecture

### Provider

The workflow provider holds a reference to the proxy (via interface to avoid import cycles):

```go
type WorkflowProvider struct {
    proxy WorkflowExecutor
    steps map[string][]WorkflowStep
}

type WorkflowExecutor interface {
    ExecuteWithContext(ctx context.Context, toolName string, params map[string]string, iface string) (*Result, error)
}
```

### Bootstrap wiring

In `bootstrapProviders`:
1. Collect workflow tool definitions during config iteration
2. Create the proxy with all other providers
3. Create `WorkflowProvider` with proxy reference
4. Register as `providers["workflow"]`

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
    steps:
      - tool: github.list_prs
        params: { state: "closed", since: "7d" }
        store: prs
      - tool: anthropic.ask
        params:
          prompt: "Write a weekly summary from these PRs: {{prs}}"
        store: report
      - tool: gmail.send_message
        params:
          to: "team@company.com"
          subject: "Weekly Report"
          body: "{{report}}"
```

## Future (not in v1)

- Conditionals (`if:` on steps)
- Parallelism (`parallel: [step1, step2]`)
- Retry logic (`retry: 3` on steps)
- Loop/iteration (`foreach:`)
- Nested workflows (a step calling another workflow — works naturally but untested)
- Scheduled workflows (cron)

---

[← Back to Documentation](README.md)
