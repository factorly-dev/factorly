# Workflow Expressions

Use `{{expr:...}}` in step params to transform data between steps — extract JSON fields, format timestamps, manipulate strings.

## Config

```yaml
# .factorly/factorly.yaml
tools:
  daily.summary:
    type: workflow
    description: Fetch today's calendar events and post a summary to Slack
    parameters:
      - name: channel
        default: "#general"
    steps:
      - tool: google-calendar.list_events
        params:
          calendarId: primary
          timeMin: "{{expr:now()}}"
          timeMax: "{{expr:now('24h')}}"
          maxResults: "10"
        store: events

      - tool: slack.post_message
        if: "jsonpath(events, '$.items') != ''"
        params:
          channel: "{{channel}}"
          text: "{{expr:concat('You have ', len(events), ' events today')}}"
```

## Run it

```bash
factorly call daily.summary
factorly call daily.summary --channel '#engineering'
```

## More examples

### Extract and transform JSON

```yaml
steps:
  - tool: api.get_user
    params:
      id: "{{user_id}}"
    store: user

  - tool: notification.send
    params:
      # Extract nested JSON field
      email: "{{expr:jsonpath(user, '$.contact.email')}}"
      # Uppercase the name
      greeting: "{{expr:concat('Hello ', upper(jsonpath(user, '$.name')))}}"
      # Default if missing
      role: "{{expr:default(jsonpath(user, '$.role'), 'member')}}"
```

### Date-based API calls

```yaml
steps:
  - tool: analytics.query
    params:
      start_date: "{{expr:today('-7')}}"
      end_date: "{{expr:today()}}"
    store: metrics

  - tool: report.generate
    params:
      title: "{{expr:concat('Weekly Report: ', today('-7'), ' to ', today())}}"
      data: "{{metrics}}"
```

### String manipulation

```yaml
steps:
  - tool: git.log
    store: log_output

  - tool: slack.post
    params:
      # First 500 chars of output
      text: "{{expr:left(log_output, 500)}}"
      # Extract filename from path
      filename: "{{expr:cut('/home/user/docs/report.pdf', '/', '-1')}}"
      # Clean up whitespace
      summary: "{{expr:trim(log_output)}}"
```

### Conditional with expressions

```yaml
steps:
  - tool: api.health_check
    store: health

  - tool: pagerduty.alert
    # Only alert if status is not healthy
    if: "jsonpath(health, '$.status') != 'healthy'"
    params:
      severity: "critical"
      message: "{{expr:concat('Service down: ', jsonpath(health, '$.message'))}}"
```

## Available functions

See the full [expressions reference](../expressions.md#value-functions) for all 20 functions: `concat`, `cut`, `default`, `find`, `join`, `jsonpath`, `left`, `len`, `lower`, `now`, `replace`, `right`, `substr`, `today`, `trim`, `upper`, and more.

## How it works

1. `{{expr:...}}` is a resolver backend — same system as `{{vault:KEY}}` and `{{env:VAR}}`
2. In workflows, expressions have access to stored step outputs and input params
3. In any tool call, stateless functions like `now()`, `today()`, `upper()` work
4. Expressions that fail return empty string — no workflow crashes

---

[← Back to Examples](README.md)
