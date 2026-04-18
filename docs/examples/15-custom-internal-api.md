# Custom Internal API

Connect your agent to a company-internal REST API with bearer auth, path parameters, body parameters, and custom headers.

## Config

```yaml
# .factorly/tools/acme-api.yaml
tools:
  acme.list_projects:
    type: rest
    description: "List all projects"
    base_url: https://api.internal.acme.com
    method: GET
    path: /v2/projects
    auth:
      type: bearer
      token: "{{vault:ACME_API_TOKEN}}"
    headers:
      X-Team-Id: platform-eng
      Accept: application/json
    parameters:
      - name: status
        in: query
      - name: limit
        in: query

  acme.get_project:
    type: rest
    description: "Get a project by ID"
    base_url: https://api.internal.acme.com
    method: GET
    path: /v2/projects/{{project_id}}
    auth:
      type: bearer
      token: "{{vault:ACME_API_TOKEN}}"
    headers:
      X-Team-Id: platform-eng
    parameters:
      - name: project_id
        in: path
        required: true

  acme.create_deployment:
    type: rest
    description: "Create a deployment for a project"
    base_url: https://api.internal.acme.com
    method: POST
    path: /v2/projects/{{project_id}}/deployments
    auth:
      type: bearer
      token: "{{vault:ACME_API_TOKEN}}"
    headers:
      X-Team-Id: platform-eng
      Content-Type: application/json
    parameters:
      - name: project_id
        in: path
        required: true
      - name: environment
        in: body
        required: true
      - name: git_ref
        in: body
        required: true
      - name: notify
        in: body
    shadow:
      confirm: true
      rate_limit: 10/hour
```

## Usage

```bash
# Store the API token
factorly vault set ACME_API_TOKEN acme_sk_xxxxxxxxxxxx

# List active projects
factorly call acme.list_projects --status active --limit 10

# Get project details (path param)
factorly call acme.get_project --project_id proj_abc123

# Create a deployment (body params + path param)
factorly call acme.create_deployment \
  --project_id proj_abc123 \
  --environment staging \
  --git_ref main \
  --notify true
# ⚠ Tool "acme.create_deployment" requires confirmation. Proceed? (y/n)
```

## What happens

1. Factorly resolves `{{project_id}}` in the path and routes other parameters to query string (GET) or JSON body (POST) based on their `in` field.
2. The bearer token and custom `X-Team-Id` header are injected on every request — the agent only sees tool names and data.
3. `acme.create_deployment` requires confirmation and is rate limited to 10 calls per hour, preventing accidental deploy storms.

---

[← Back to Examples](README.md)
