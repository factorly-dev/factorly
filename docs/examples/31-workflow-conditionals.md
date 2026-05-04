# Workflow Conditionals

Use `if:` and `switch:` to make workflows that branch based on results.

## Skip steps with `if:`

Only commit if there are uncommitted changes:

```yaml
tools:
  git.status:
    type: cli
    command: git
    args: ["status", "--porcelain"]

  git.commit:
    type: cli
    command: git
    args: ["commit", "-am", "{{message}}"]
    parameters:
      - name: message
        required: true

  auto.commit:
    type: workflow
    description: Auto-commit if there are changes
    parameters:
      - name: message
        default: "auto-commit"
    steps:
      - tool: git.status
        store: changes

      - tool: git.commit
        params: { message: "{{message}}" }
        if: "changes != ''"
```

```bash
factorly call auto.commit --message "update deps"
```

If there are no changes, the commit step is skipped — no error.

## Branch with `switch:`

Route notifications based on service health:

```yaml
tools:
  api.health:
    type: rest
    base_url: https://api.example.com
    method: GET
    path: /health

  slack.post:
    type: rest
    base_url: https://hooks.slack.com
    method: POST
    path: /services/{{webhook_id}}
    parameters:
      - name: webhook_id
      - name: text
        required: true

  pagerduty.alert:
    type: rest
    base_url: https://events.pagerduty.com
    method: POST
    path: /v2/enqueue
    parameters:
      - name: severity
      - name: summary

  health.check:
    type: workflow
    description: Check service health and route alerts
    steps:
      - tool: api.health
        store: response

      - switch:
          - condition: "jsonpath(response, '$.status') == 'healthy'"
            tool: slack.post
            params:
              webhook_id: "T00/B00/xxx"
              text: "✓ All systems healthy"
          - condition: "jsonpath(response, '$.status') == 'degraded'"
            tool: pagerduty.alert
            params:
              severity: warning
              summary: "Service degraded"
          - condition: "true"
            tool: pagerduty.alert
            params:
              severity: critical
              summary: "Service down or unknown status"
```

```bash
factorly call health.check
```

## Conditional deploy pipeline

Only deploy if tests pass and we're on the right branch:

```yaml
tools:
  git.branch:
    type: cli
    command: git
    args: ["rev-parse", "--abbrev-ref", "HEAD"]

  make.test:
    type: cli
    command: make
    args: ["test"]

  make.deploy:
    type: cli
    command: make
    args: ["deploy-staging"]

  deploy.staging:
    type: workflow
    description: Deploy to staging with safety gates
    parameters:
      - name: branch
        required: true
    steps:
      - tool: git.branch
        store: current_branch

      - tool: make.test
        store: test_output
        if: "current_branch == branch"

      - tool: make.deploy
        if: "contains(test_output, 'PASS') and not contains(test_output, 'FAIL')"

      - tool: slack.post
        params:
          webhook_id: "T00/B00/xxx"
          text: "Deployed {{branch}} to staging"
        if: "contains(test_output, 'PASS')"
```

```bash
factorly call deploy.staging --branch main
```

If you're on the wrong branch, tests don't run. If tests fail, deploy doesn't run.

## Switch with stored output

Use switch output in later steps:

```yaml
tools:
  config.get:
    type: cli
    command: echo
    args: ["{{value}}"]
    parameters:
      - name: value

  format.output:
    type: cli
    command: printf
    args: ["Tier: {{tier}}, Price: ${{price}}/mo"]
    parameters:
      - name: tier
      - name: price

  pricing.lookup:
    type: workflow
    description: Get pricing based on tier
    parameters:
      - name: tier
        required: true
        enum: [free, pro, enterprise]
    steps:
      - switch:
          - condition: "tier == 'free'"
            tool: config.get
            params: { value: "0" }
            store: price
          - condition: "tier == 'pro'"
            tool: config.get
            params: { value: "29" }
            store: price
          - condition: "tier == 'enterprise'"
            tool: config.get
            params: { value: "99" }
            store: price

      - tool: format.output
        params: { tier: "{{tier}}", price: "{{price}}" }
```

```bash
$ factorly call pricing.lookup --tier pro
# → "Tier: pro, Price: $29/mo"
```

## Combine `if:` and `switch:`

```yaml
tools:
  make.test:
    type: cli
    command: make
    args: ["test"]

  make.lint:
    type: cli
    command: make
    args: ["lint"]

  ci.notify:
    type: workflow
    description: Run CI and notify based on result
    steps:
      - tool: make.test
        store: output

      - switch:
          - condition: "contains(output, 'PASS')"
            tool: slack.post
            params: { text: "✓ CI passed" }
          - condition: "contains(output, 'FAIL')"
            tool: slack.post
            params: { text: "✗ CI failed" }

      - tool: make.lint
        if: "contains(output, 'PASS')"
        store: lint

      - tool: slack.post
        params: { text: "Lint: {{lint}}" }
        if: "lint != ''"
```

## What happens

- `if:` evaluates an [expression](../expressions.md) — false means the step is skipped (no error)
- `switch:` evaluates conditions in order — first match executes, no match = skip
- Skipped steps show `"status": "skipped"` in the output JSON
- Switch cases support `store:` — output flows to later steps
- All expressions resolve from workflow variables (input params + stored outputs)
- Invalid expressions fail closed (treated as false, step skipped)

---

[← Back to Examples](README.md)
