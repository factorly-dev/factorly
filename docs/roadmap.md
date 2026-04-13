# Roadmap

## Shipped

- [x] **Encrypted vault** — `{{vault:KEY}}` with per-entry encryption (HKDF + AES-256-GCM)
- [x] **CLI provider** — wrap shell commands as tools
- [x] **REST provider** — wrap HTTP APIs with auth, path/query/body routing
- [x] **MCP provider** — spawn child servers (stdio) or connect to remote (HTTP)
- [x] **`factorly init`** — interactive project setup
- [x] **`factorly wrap`** — zero-config proxy for any MCP server
- [x] **`factorly serve`** — MCP server mode (stdio + HTTP with token auth)
- [x] **`factorly tools`** — list, add, remove, import (OpenAPI, templates, curl)
- [x] **`factorly tools status`** — health check all tools and connections
- [x] **`factorly logs`** — view, filter, follow, and summarize the audit log
- [x] **`factorly sync`** — push MCP config into AI clients (Claude Code, Cursor, Codex)
- [x] **OAuth 2.0** — `factorly auth login/status/logout` with PKCE + auto-refresh
- [x] **Tool Shadowing** — deny, confirm (CLI + MCP elicitation), rate limit
- [x] **Loop detection** — fingerprint-based detection of repeated identical calls
- [x] **Token bucket rate limiting** — smooth throttling, per-agent scoping
- [x] **Output compression** — ANSI strip, JSON compact, log dedup, whitespace normalize
- [x] **Output truncation** — 60/40 head/tail with savings tracking
- [x] **Agent identity** — per-session tracking, per-agent rate limits, audit trail
- [x] **Environment isolation** — opt-in strict mode for child processes
- [x] **36 tool templates** — pre-built YAML configs for popular services
- [x] **Call logging** — JSONL audit trail with savings, governance, and agent fields
- [x] **`factorly exec`** — run any command through Factorly's safety layer (vault/env resolution, compression, logging)
- [x] **Disabled commands** — `disabled_commands` config to restrict CLI access per project
- [x] **npm distribution** — `npm install -g @factorly/cli`

## Future?

### Agent integration
- [ ] **PreToolUse hooks** — `factorly hooks install` to intercept agent Bash calls, rewriting them to `factorly exec` for compression and logging without MCP ([spec](hooks-spec.md))
- [ ] **Command-specific compression** — pattern-based output reduction for git, npm, cargo, test runners (70-90% savings vs generic compression)
- [ ] **Response caching** — return cached results for identical tool calls within a configurable window, reducing API quota usage and latency

### Observability
- [ ] **Dashboard** — localhost web UI showing live call feed, savings counter, agent activity, blocked call breakdown
- [ ] **`factorly report`** — generate a shareable summary of tool usage, savings, and governance actions for a time period
- [ ] **Webhook notifications** — alert on blocked calls, rate limits, errors via webhook (Slack, Telegram, HTTP)

### Security & auth
- [ ] **OAuth 2.1 server** — MCP-spec authorization server with PKCE, dynamic client registration, and token endpoints ([spec](oauth-server-spec.md))
- [ ] **Hosted OAuth** — Factorly-managed OAuth app registrations for Google, Microsoft, Salesforce (eliminates provider-side app setup)
- [ ] **External vault backends** — 1Password, GCP Secret Manager, AWS Secrets Manager, HashiCorp Vault
- [ ] **Config signing** — verify tool config integrity hasn't been tampered with post-deployment
- [ ] **Secret rotation helpers** — workflows for rotating vault keys with zero downtime

### Scale & collaboration
- [ ] **Factorly Cloud** — hosted version with team management, shared vaults, and centralized governance
- [ ] **Team configs** — shared tool definitions and governance policies across a team
- [ ] **Shared credential vault** — team-scoped secrets with role-based access
- [ ] **Cost attribution** — track estimated API call costs per agent per tool with budget enforcement

### Ecosystem
- [ ] **Community template registry** — `factorly tools import templates --update` to fetch community-contributed templates
- [ ] **Plugin system** — custom providers beyond CLI/REST/MCP
- [ ] **CI/CD integration** — GitHub Actions, GitLab CI recipes for governed tool access in pipelines
- [ ] **Terraform/Pulumi provider** — manage Factorly configs as infrastructure

---

[← Back to Documentation](README.md)
