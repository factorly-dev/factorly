# Tool Templates

Pre-built tool configurations for popular services. Install a template and get working tools in 30 seconds.

## Quick Start

```bash
# List available templates
factorly tools import templates

# Install (interactive — prompts for API key, stores in vault)
factorly tools import templates linear

# Preview without writing
factorly tools import templates linear --dry-run

# Non-interactive
factorly tools import templates linear --api-key "$LINEAR_API_KEY" --all
```

## Available Templates

### Engineering

| Template | Auth | Tools | Description |
|----------|------|-------|-------------|
| `linear` | API key | 6 | Issue tracking and project management |
| `github` | Token (PAT) | 8 | Code hosting, issues, PRs, repositories |
| `slack` | Bot token | 6 | Team messaging, channels, notifications |
| `discord` | Bot token | 5 | Server messaging and management |
| `telegram` | Bot token | 6 | Bot messaging, mobile notifications |
| `notion` | Integration token | 6 | Docs, databases, project management |
| `jira` | API token | 7 | Issues, projects, and workflows |
| `airtable` | Token | 6 | Bases, tables, and records |
| `coda` | API key | 5 | Docs, tables, and rows |
| `bigquery` | OAuth | 4 | SQL queries, datasets, tables |

### Business

| Template | Auth | Tools | Description |
|----------|------|-------|-------------|
| `stripe` | Secret key | 8 | Payments, subscriptions, billing |
| `hubspot` | Private app token | 7 | CRM, contacts, deals |
| `salesforce` | OAuth | 6 | Enterprise CRM, SOQL queries |
| `calendly` | Token | 5 | Scheduling and calendar management |
| `intercom` | Token | 6 | Customer messaging and support |
| `zendesk` | Token | 6 | Support tickets and help desk |
| `freshdesk` | API key | 6 | Ticket management and support |
| `shopify` | Admin token | 6 | Orders, products, inventory |
| `sendgrid` | API key | 5 | Email delivery and contacts |
| `asana` | Token | 6 | Tasks and project management |
| `clickup` | Token | 6 | Tasks and productivity |
| `trello` | API key | 6 | Kanban boards and cards |
| `monday` | Token | 5 | Work management and boards |
| `dropbox` | OAuth | 6 | File storage and sharing |
| `docusign` | OAuth | 5 | Electronic signatures |
| `apollo` | API key | 5 | Sales intelligence and prospecting |
| `attio` | Token | 6 | CRM and relationship management |

### Google (OAuth required)

| Template | Tools | Description |
|----------|-------|-------------|
| `gmail` | 6 | Read, send, and search email |
| `google-sheets` | 6 | Read and write spreadsheets |
| `google-calendar` | 6 | Events and scheduling |
| `google-drive` | 6 | Files and folders |
| `google-docs` | 3 | Create and read documents |

### Microsoft (OAuth required)

| Template | Tools | Description |
|----------|-------|-------------|
| `microsoft-teams` | 5 | Messaging and channels |
| `microsoft-outlook` | 6 | Email and calendar |
| `onedrive` | 5 | File storage |
| `sharepoint` | 5 | Document management |

## What Happens During Install

1. **Auth** — Prompts for your API key or OAuth credentials. Stores them in the encrypted vault. If credentials already exist in the vault, skips this step.

2. **Tool selection** — Choose all tools or pick individually.

3. **Write config** — Writes tool definitions to `.factorly/tools/<name>.yaml`. This is a standard Factorly config file — you can edit it by hand afterward.

4. **OAuth** (if applicable) — Adds an `oauth_providers` entry to `.factorly/factorly.yaml` and tells you to run `factorly auth login <provider>`.

## Governance

Write and delete actions include `shadow: confirm: true` by default — your agent must confirm before creating, updating, or deleting resources. Read and search actions have no restrictions.

You can modify governance after install by editing the YAML:

```yaml
# .factorly/tools/linear.yaml
linear.create_issue:
  type: rest
  ...
  shadow:
    confirm: true        # require confirmation
    rate_limit: 10/hour  # add a rate limit
```

## OAuth Templates

Templates for Google, Microsoft, Salesforce, DocuSign, and Dropbox require OAuth. During install, you'll be prompted for a client ID and secret, which get stored in the vault.

After install:

```bash
factorly auth login gmail
```

This opens your browser for the OAuth consent flow. Tokens are stored in the vault and auto-refresh.

**Setting up Google OAuth:**
1. Go to [Google Cloud Console](https://console.cloud.google.com/apis/credentials)
2. Create a **Desktop app** OAuth client
3. Enable the relevant API (Gmail API, Sheets API, etc.)
4. Use the client ID and secret when installing the template

**Setting up Microsoft OAuth:**
1. Go to [Azure Portal](https://portal.azure.com/#blade/Microsoft_AAD_RegisteredApps)
2. Register an application
3. Add redirect URI: `http://localhost:18019/callback`
4. Use the client ID and secret when installing the template

## Template Files

Templates are plain YAML — the same format as any Factorly tool config. You can:

- **Copy manually** instead of using the command: grab a YAML file from the [templates directory](https://github.com/factorly-dev/factorly-cli/tree/main/src/internal/templates/yaml) and drop it in `.factorly/tools/`
- **Edit after install** — add parameters, change paths, adjust governance
- **Use as examples** — reference the YAML when writing your own tool configs

## Flags

```bash
--dry-run     # Preview YAML without writing
--all         # Install all tools (skip selection prompt)
--api-key     # API key or token (non-interactive, skip prompt)
```

---

[← Back to README](README.md)
