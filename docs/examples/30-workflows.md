# Workflows

Chain multiple tool calls into a single governed pipeline. Each step runs through the proxy with full governance — shadow policy, rate limiting, logging, and output filtering apply to every step individually.

## Config

```yaml
# .factorly/factorly.yaml
tools:
  report.weekly:
    type: workflow
    description: Generate weekly report and email it
    parameters:
      - name: team
        description: Team name for the report
        default: engineering
    steps:
      - tool: github.list_prs
        params: { state: "closed", owner: "factorly-dev" }
        store: prs

      - tool: anthropic.ask
        params:
          prompt: "Write a weekly summary for the {{team}} team from these PRs: {{prs}}"
        store: report

      - tool: gmail.send_message
        params:
          to: "{{team}}@company.com"
          subject: "Weekly Report"
          body: "{{report}}"
```

## Usage

```bash
# Run the workflow
factorly call report.weekly --team platform

# Verbose — see each step as it runs
factorly call report.weekly --team platform -v
```

**Verbose output:**
```
[workflow] report.weekly started (run: a1b2c3d4)
[workflow]   1/3 github.list_prs       running...
[workflow]   1/3 github.list_prs       completed  215ms
[workflow]   2/3 anthropic.ask         running...
[workflow]   2/3 anthropic.ask         completed  3.2s
[workflow]   3/3 gmail.send_message    running...
[workflow]   3/3 gmail.send_message    completed  342ms
[workflow] report.weekly completed (3 steps, 3.8s)
```

## Output

The workflow output includes the full execution trace:

```json
{
  "status": "completed",
  "steps": [
    {"tool": "github.list_prs", "status": "completed", "duration_ms": 215},
    {"tool": "anthropic.ask", "status": "completed", "duration_ms": 3200},
    {"tool": "gmail.send_message", "status": "completed", "duration_ms": 342}
  ],
  "result": "Message sent to platform@company.com"
}
```

## Variable passing

Steps pass data via `store:` and `{{var}}`:

```yaml
# .factorly/factorly.yaml
tools:
  ci.pipeline:
    type: workflow
    description: Run tests and notify
    steps:
      - tool: factorly.shell
        params: { command: "go test ./..." }
        store: test_output

      - tool: anthropic.ask
        params:
          prompt: "Summarize these test results: {{test_output}}"
        store: summary

      - tool: slack.post_message
        params:
          channel: "#ci"
          text: "{{summary}}"
```

## Error handling

Workflows stop on first error. Remaining steps are skipped:

```json
{
  "status": "failed",
  "error": "step 2 (anthropic.ask): API error 429",
  "steps": [
    {"tool": "factorly.shell", "status": "completed", "duration_ms": 5200},
    {"tool": "anthropic.ask", "status": "failed", "error": "API error 429"},
    {"tool": "slack.post_message", "status": "skipped"}
  ]
}
```

## How agents use workflows

Agents see workflows as a single tool — they don't see individual steps. The workflow appears in `factorly tools` and MCP `tools/list` like any other tool, with its own name, description, and parameters:

```
$ factorly tools
  report.weekly     workflow   Generate weekly report and email it
  echo.test         cli        Echo a message
```

When an agent calls a workflow, it passes the workflow's parameters. The steps are opaque — the agent gets back the JSON execution trace as the result. This means:

- The agent doesn't need to know how the workflow works internally
- Individual steps are governed independently (the agent can't bypass step-level deny/confirm)
- The workflow is a single auditable unit in the log

## What can be a step?

Any tool registered in your config can be a step — CLI, REST, MCP, or even another workflow:

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
      - tool: fetch.data           # REST tool as a step
        store: raw
      - tool: factorly.shell       # Built-in tool as a step
        params: { command: "echo '{{raw}}' | jq '.items'" }
        store: filtered
      - tool: anthropic.ask        # Another REST tool
        params: { prompt: "Summarize: {{filtered}}" }
```

## State persistence

Each workflow run persists state to `.factorly/workflows/<run-id>.json`. State is updated after each step completes:

```bash
# View state of a completed or failed run
cat .factorly/workflows/a1b2c3d4.json | jq
```

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

This is useful for debugging failed workflows — you can see exactly which step failed, what data was available, and what the error was.

## Oversight

Each step goes through the proxy individually — shadow policy, rate limiting, and loop detection apply per step:

```yaml
# .factorly/factorly.yaml
tools:
  deploy.staging:
    type: workflow
    description: Deploy to staging with approval
    parameters:
      - name: branch
        required: true
    steps:
      - tool: factorly.shell
        params: { command: "git checkout {{branch}}" }
      - tool: factorly.shell
        params: { command: "make deploy-staging" }
        # This step will trigger confirm if factorly.shell has shadow.confirm: true
```

## What happens

1. Workflow receives input parameters from the caller
2. Steps execute sequentially through the proxy
3. Each step: parameter validation → shadow policy check → execute → output filter → log
4. `store:` saves step output as a variable for later steps
5. `{{var}}` in step params is substituted from input params + stored outputs
6. On failure, remaining steps are skipped and the state is persisted
7. State is saved to `.factorly/workflows/<run-id>.json` after each step
8. Output includes the full execution trace as JSON

## Tips

- **Keep workflows deterministic.** Every step is defined upfront — no LLM decision-making in the pipeline itself. If you need branching logic, let the agent decide which workflow to call.
- **Use `store:` liberally.** Named variables make workflows readable and debuggable. The persisted state shows exactly what data flowed between steps.
- **Combine with parameter validation.** Add `min`/`max`/`enum` rules on workflow parameters to catch bad inputs before any step runs.
- **Shadow policy applies per step.** If `factorly.shell` has `confirm: true`, each shell step in the workflow will trigger confirmation independently.

---

[← Back to Examples](README.md)
