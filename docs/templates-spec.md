# Spec: Tool Templates

> Status: **In Progress** — `factorly tools import templates`

## Problem

Setting up Factorly tool configs for popular services requires writing YAML from scratch — knowing the API base URL, auth type, endpoint paths, and parameter schemas. This friction kills adoption. A developer should be able to run `factorly tools import templates linear` and have working tools in 30 seconds.

## UX

```bash
# List available templates
factorly tools import templates

# Install a template (interactive)
factorly tools import templates linear

# Non-interactive
factorly tools import templates linear --api-key "$LINEAR_API_KEY"

# Preview without writing
factorly tools import templates linear --dry-run
```

### Interactive flow

```
$ factorly tools import templates linear

  Linear — Issue tracking and project management

  Auth: API key
  Guide: Get one at https://linear.app/settings/api
  API key: ********

  ✓ Stored API key in vault as LINEAR_API_KEY

  Which tools to install? (6 available)
  1) All (6 tools)
  2) Choose individually
  > 1

  Created 6 tools:
    linear.list_issues       List issues with filters
    linear.create_issue      Create a new issue
    ...

  Shadow governance applied to write/delete operations

  Written to .factorly/tools/linear.yaml
  Run 'factorly tools' to see all tools.
```

## Architecture

Templates use **Go metadata + embedded YAML**:

- **Go files** carry metadata: name, display name, description, category, auth type, auth guide, vault key, OAuth config
- **YAML files** contain the actual tool definitions — the same format users write by hand
- The command reads metadata for prompts, then writes the YAML as-is to `.factorly/tools/`
- No code generation — the YAML is both the template and the output

```go
type Template struct {
    Name        string
    DisplayName string
    Description string
    Category    string       // "engineering" | "business"
    AuthType    string       // "api_key" | "oauth" | "bearer"
    AuthGuide   string
    VaultKey    string       // for api_key/bearer
    OAuthConfig *OAuthConfig // for oauth
    YAML        string       // embedded tool definitions (//go:embed)
}
```

```
src/internal/templates/
├── templates.go        # Template struct, All(), Get(), ToolCount(), FilterYAML()
├── linear.go           # metadata + //go:embed yaml/linear.yaml
├── github.go
├── ...
├── yaml/
│   ├── linear.yaml     # bare tool definitions (directly usable as config)
│   ├── github.yaml
│   └── ...
└── templates_test.go
```

## Template Categories

### Self-service (API key / token)

These templates work with a simple paste-a-token setup:

| Template | Auth | Tools | Description |
|----------|------|-------|-------------|
| **linear** | API key | 6 | Issue tracking and project management |
| **github** | bearer (PAT) | 8 | Code hosting, issues, PRs, repositories |
| **slack** | bearer (bot token) | 6 | Team messaging, channels, notifications |
| **stripe** | bearer (secret key) | 8 | Payments, subscriptions, billing |
| **telegram** | bearer (bot token) | 6 | Bot messaging, mobile notifications |
| **notion** | bearer (integration token) | 6 | Docs, databases, project management |

Additional self-service templates: Airtable, Coda, Discord, Jira, HubSpot, Freshdesk, SendGrid, Shopify, Intercom, Calendly, Asana, ClickUp, Trello, monday.com, Zendesk, Apollo, Attio

### OAuth required

These templates require OAuth app registration with the provider:

| Template | Provider | Scopes |
|----------|----------|--------|
| Gmail, Sheets, Calendar, Drive, Docs, BigQuery | Google | service-specific |
| Teams, Outlook, OneDrive, SharePoint | Microsoft | Graph API scopes |
| Salesforce | Salesforce | api, refresh_token |
| DocuSign | DocuSign | signature |
| Dropbox | Dropbox | files.content.read/write |

OAuth templates prompt for client_id and client_secret (stored in vault), generate an `oauth_providers` entry in `factorly.yaml`, and direct the user to run `factorly auth login <provider>`.

**Future: Hosted OAuth** — Factorly-the-service could register OAuth apps once and handle auth on behalf of users, eliminating the app registration friction.

## Shadow Governance Defaults

Write and delete actions in template YAML include `shadow: confirm: true`. Read and search actions have no governance by default.

## Future

- **Remote template registry** — `factorly tools import templates --update` to fetch community templates
- **Hosted OAuth** — Factorly manages OAuth app registrations for Google, Microsoft, etc.
- **Template contributions** — YAML-based templates are easy to contribute (just a YAML file + Go metadata)

---

[← Back to README](README.md)
