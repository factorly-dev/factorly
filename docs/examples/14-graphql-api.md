# GraphQL API Integration

Integrate a GraphQL API (Linear) using Factorly's REST tool type. GraphQL is just a POST to a single endpoint with the query in the request body.

## Config

```yaml
# .factorly/tools/linear.yaml
tools:
  linear.list_issues:
    type: rest
    description: "List assigned issues from Linear"
    base_url: https://api.linear.app
    method: POST
    path: /graphql
    auth:
      type: bearer
      token: "{{vault:LINEAR_API_KEY}}"
    headers:
      Content-Type: application/json
    parameters:
      - name: query
        in: body
        required: true

  linear.create_issue:
    type: rest
    description: "Create a new issue in Linear"
    base_url: https://api.linear.app
    method: POST
    path: /graphql
    auth:
      type: bearer
      token: "{{vault:LINEAR_API_KEY}}"
    headers:
      Content-Type: application/json
    parameters:
      - name: query
        in: body
        required: true
      - name: variables
        in: body
```

## Usage

```bash
# Store your Linear API key
factorly vault set LINEAR_API_KEY lin_api_xxxxxxxxxxxx

# List your assigned issues
factorly call linear.list_issues \
  --query '{ "query": "{ viewer { assignedIssues(first: 10) { nodes { title state { name } } } } }" }'

# Create an issue
factorly call linear.create_issue \
  --query '{ "query": "mutation($input: IssueCreateInput!) { issueCreate(input: $input) { issue { id title } } }" }' \
  --variables '{ "input": { "teamId": "TEAM_ID", "title": "Fix login bug", "priority": 2 } }'
```

## What happens

1. Factorly sends a `POST` to `https://api.linear.app/graphql` with the `query` (and optional `variables`) in the JSON body.
2. The `Authorization: Bearer` header is injected from the vault — the agent never sees the API key.
3. The tool type is `rest`, not a special GraphQL type. GraphQL APIs are just REST endpoints that accept a query string in the body.
4. Each operation (list, create) is a separate Factorly tool, giving you independent oversight — you could deny `linear.create_issue` while allowing reads.

---

[← Back to Examples](README.md)
