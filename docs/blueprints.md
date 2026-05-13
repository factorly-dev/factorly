# Blueprints

A **blueprint** is a single YAML file that bundles tools, workflows, and OAuth provider definitions into one sharable, installable unit. Factorly ships 40+ bundled blueprints for popular services, and you can also install blueprints from a GitHub repo, a raw URL, or a local file.

## Quick Start

### Install a bundled blueprint

```bash
# Install by name — pulls from the 40+ blueprints baked into the binary
factorly blueprint install linear

# Preview without writing anything
factorly blueprint install linear --dry-run

# Skip the interactive vault-key prompt (you'll set keys later)
factorly blueprint install linear --no-prompt
```

### Install from a GitHub repo or URL

```bash
# GitHub shorthand pointing at a specific file in a community blueprint repo.
# The official community catalog lives at github.com/factorly-dev/factorly-blueprints.
factorly blueprint install github.com/factorly-dev/factorly-blueprints/gmail.yaml

# Pin a tag / branch / commit
factorly blueprint install github.com/factorly-dev/factorly-blueprints@v1.0.0/gmail.yaml

# Whole-repo shorthand — resolves to factorly.yaml or blueprint.yaml at the repo root
factorly blueprint install github.com/your-org/your-blueprint-repo

# Raw URL
factorly blueprint install https://example.com/my-blueprint.yaml

# Local file
factorly blueprint install ./my-blueprint.yaml
```

### Manage installed blueprints

```bash
# List
factorly blueprint list

# Uninstall (removes the .factorly/blueprints/<name>.yaml file)
factorly blueprint uninstall linear
```

## The UI Catalog

Run `factorly ui` and open the **Blueprints** page in the nav, then click **Browse Catalog** in the header to see all bundled blueprints as a grid:

- Click a card → detail page with the tools it adds, the auth guide, vault keys needed, and a YAML preview.
- Fill in your credentials and click **Install Blueprint** — the blueprint is installed and the tools become live in the proxy immediately.
- Already-installed blueprints show an **Installed** badge.

The catalog grid is the same set of blueprints `factorly blueprint install <name>` resolves; they're embedded in the binary so the catalog works offline.

You can also paste a blueprint YAML directly into the **Install Blueprint** modal — no need to save it to a file first.

## Bundled Blueprints

### Engineering

| Blueprint | Auth | Tools | Description |
|-----------|------|-------|-------------|
| `anthropic` | api_key | 2 | Claude API — messages endpoint + simple ask interface |
| `chatgpt` | api_key | 2 | OpenAI API — chat completions + simple ask interface |
| `linear` | api_key | 6 | Issue tracking and project management |
| `github` | bearer | 8 | Code hosting, issues, PRs, repositories |
| `slack` | bearer | 6 | Team messaging, channels, notifications |
| `discord` | bearer | 5 | Server messaging and management |
| `telegram` | bearer | 6 | Bot messaging, mobile notifications |
| `notion` | bearer | 6 | Docs, databases, project management |
| `jira` | api_key | 7 | Issues, projects, and workflows |
| `airtable` | bearer | 6 | Bases, tables, and records |
| `coda` | api_key | 5 | Docs, tables, and rows |
| `bigquery` | oauth | 4 | SQL queries, datasets, tables |
| `git` | none | 6 | Local git operations with oversight |
| `make` | none | 1 | Run `make` targets in the project |
| `npm` | none | 3 | Run npm scripts and queries |

### Business

| Blueprint | Auth | Tools | Description |
|-----------|------|-------|-------------|
| `stripe` | api_key | 8 | Payments, subscriptions, billing |
| `hubspot` | bearer | 7 | CRM, contacts, deals |
| `salesforce` | oauth | 6 | Enterprise CRM, SOQL queries |
| `calendly` | bearer | 5 | Scheduling and calendar management |
| `intercom` | bearer | 6 | Customer messaging and support |
| `zendesk` | bearer | 6 | Support tickets and help desk |
| `freshdesk` | api_key | 6 | Ticket management and support |
| `shopify` | bearer | 6 | Orders, products, inventory |
| `sendgrid` | api_key | 5 | Email delivery and contacts |
| `asana` | bearer | 6 | Tasks and project management |
| `clickup` | bearer | 6 | Tasks and productivity |
| `trello` | api_key | 6 | Kanban boards and cards |
| `monday` | bearer | 5 | Work management and boards |
| `dropbox` | oauth | 6 | File storage and sharing |
| `docusign` | oauth | 5 | Electronic signatures |
| `apollo` | api_key | 5 | Sales intelligence and prospecting |
| `attio` | bearer | 6 | CRM and relationship management |

### Google (OAuth)

| Blueprint | Tools | Description |
|-----------|-------|-------------|
| `gmail` | 6 | Read, send, and search email |
| `google-sheets` | 6 | Read and write spreadsheets |
| `google-calendar` | 6 | Events and scheduling |
| `google-drive` | 6 | Files and folders |
| `google-docs` | 3 | Create and read documents |

### Microsoft (OAuth)

| Blueprint | Tools | Description |
|-----------|-------|-------------|
| `microsoft-teams` | 5 | Messaging and channels |
| `microsoft-outlook` | 6 | Email and calendar |
| `onedrive` | 5 | File storage |
| `sharepoint` | 5 | Document management |

## What Happens During Install

1. **Resolve** — Look up the blueprint by name (bundled), fetch the URL, clone the file from disk, or read the pasted bytes.
2. **Validate** — Parse the YAML, check that any `requires.tools` / `requires.oauth_providers` are satisfied by the merged view of your existing config + the incoming blueprint, and confirm there are no name conflicts with already-installed tools.
3. **Collect vault keys** — If the blueprint declares `requires.vault_keys` (e.g. `LINEAR_API_KEY` or `<NAME>_CLIENT_ID` / `<NAME>_CLIENT_SECRET` for OAuth), you're prompted for the values and they're stored in the encrypted local vault. `--no-prompt` skips this and tells you which keys to set later.
4. **Write** — The blueprint YAML lands at `.factorly/blueprints/<name>.yaml`. It's a plain YAML file — you can edit it by hand afterward.
5. **Reload** — When installed through the UI, the live proxy registry reloads so the new tools are immediately callable. The CLI install just writes the file; the next `factorly` invocation picks it up.

## Authentication

Blueprints declare their auth shape in the header:

```yaml
auth_type: api_key   # or "bearer" or "oauth" or "none"
auth_guide: "Get your API key at https://linear.app/settings/api"
```

The `auth_guide` is what shows up on the UI detail page when you're installing. The `auth_type` drives the catalog chip and tells the install flow which vault keys to expect.

### API key / bearer

Tools reference the vault directly:

```yaml
auth:
  type: bearer
  token: "{{vault:LINEAR_API_KEY}}"
```

You set the key once during install; every tool in the blueprint uses it.

### OAuth

The blueprint ships the OAuth provider definition with vault references for the client credentials:

```yaml
oauth_providers:
  gmail:
    client_id: "{{vault:GMAIL_CLIENT_ID}}"
    client_secret: "{{vault:GMAIL_CLIENT_SECRET}}"
    auth_url: https://accounts.google.com/o/oauth2/v2/auth
    token_url: https://oauth2.googleapis.com/token
    scopes:
      - https://www.googleapis.com/auth/gmail.modify

tools:
  gmail.search:
    auth:
      type: oauth
      provider: gmail   # references the oauth_providers entry above
```

After install:

```bash
factorly auth login gmail
```

This opens your browser for the OAuth consent flow. Tokens are stored in the vault and auto-refresh.

**Google OAuth setup:**
1. Go to [Google Cloud Console](https://console.cloud.google.com/apis/credentials).
2. Create a **Desktop app** OAuth client.
3. Enable the relevant API (Gmail API, Sheets API, etc.).
4. Provide the client ID + secret during `factorly blueprint install <gmail|google-...>`.

**Microsoft OAuth setup:**
1. Go to [Azure Portal](https://portal.azure.com/#blade/Microsoft_AAD_RegisteredApps).
2. Register an application.
3. Add redirect URI: `http://localhost:18019/callback`.
4. Provide the client ID + secret during install.

## Oversight

Write and delete actions ship with `shadow: confirm: true` by default — your agent must confirm before creating, updating, or deleting resources. Read and search actions have no restrictions.

You can modify oversight rules after install by editing the YAML:

```yaml
# .factorly/blueprints/linear.yaml
tools:
  linear.create_issue:
    type: rest
    ...
    shadow:
      confirm: true         # require confirmation
      rate_limit: 10/hour   # add a rate limit
```

## Authoring Your Own Blueprint

A blueprint is just a YAML file with an optional header and a `tools:` map. Minimum viable shape:

```yaml
name: my-thing
version: 1.0.0
description: My custom blueprint
author: me
license: MIT

tools:
  my.tool:
    type: cli
    command: echo
    description: prints a thing
    args: ["{{msg}}"]
```

Add `display_name`, `category`, `auth_type`, `auth_guide`, `homepage` if you want the UI catalog to render it nicely.

For tools that need credentials, declare them up front:

```yaml
requires:
  vault_keys:
    - MY_API_KEY
```

so the install flow knows what to prompt for. Publish the file anywhere — a GitHub gist, a repo, your own server — and your users install it with:

```bash
factorly blueprint install github.com/you/your-blueprint
```

See [`examples/blueprints/gmail.yaml`](https://github.com/factorly-dev/factorly/blob/main/examples/blueprints/gmail.yaml) for the canonical reference shape.

## Reference

### CLI flags

```bash
factorly blueprint install <source> [--dry-run] [--no-prompt]
factorly blueprint uninstall <name>
factorly blueprint list
```

- `--dry-run` — Validate and preview without writing.
- `--no-prompt` — Skip interactive vault-key prompts; print what's missing.

### File location

Installed blueprints live at `.factorly/blueprints/<name>.yaml`. They're plain YAML — edit them by hand, commit them to git, or delete them to uninstall.

### Inline / paste install

The UI Install modal accepts pasted YAML directly. The CLI doesn't (use `--` redirection or a temp file), but the `internal/blueprints` library does — useful if you're scripting against it.

---

[← Back to README](README.md)
