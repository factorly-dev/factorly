# Roadmap

## Shipped

- [x] **Encrypted vault** — `{{vault:KEY}}` with per-entry encryption (HKDF + AES-256-GCM)
- [x] **CLI provider** — wrap shell commands as tools
- [x] **REST provider** — wrap HTTP APIs with auth, path/query/body routing
- [x] **MCP provider** — spawn child servers (stdio) or connect to remote (HTTP)
- [x] **`factorly init`** — interactive project setup
- [x] **`factorly exec`** — run any command through Factorly's safety layer (vault/env resolution, compression, logging)
- [x] **`factorly wrap`** — zero-config proxy for any MCP server
- [x] **`factorly serve`** — MCP server mode (stdio + HTTP with token auth)
- [x] **`factorly tools`** — list, add, remove, import (OpenAPI, curl)
- [x] **`factorly blueprint`** — install/uninstall/list sharable tool bundles (bundled catalog + URL/file/GitHub install)
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
- [x] **40+ bundled blueprints** — pre-built tool/workflow bundles for popular services (Gmail, GitHub, Slack, Linear, Stripe, ...)
- [x] **Call logging** — JSONL audit trail with savings, oversight, and agent fields
- [x] **Disabled commands** — `disabled_commands` config to restrict CLI access per project
- [x] **Built-in tools** — governed shell, read, write, fetch, clipboard with default safety guards
- [x] **Per-project vault** — `.factorly/vault.enc` with fallback to global, separate passwords, lazy opening
- [x] **npm distribution** — `npm install -g factorly`
- [x] **External vault backends** — tool-style config for 1Password, AWS SM, GCP SM, any CLI-based secret manager
- [x] **Workspaces** — named variable + vault overlays for switching envs (`--workspace staging`, `--workspace prod`); per-workspace OAuth token isolation
- [x] **`type: workflow`** — sequential tool pipelines with variable passing, persisted state machine, execution trace output
- [x] **`type: code`** — Go scripts run in a yaegi interpreter; agent-authored scripts via `factorly.code` builtin; re-enters the proxy via `factorly.Call` for full shadow/vault/audit coverage
- [x] **Web UI** — `factorly ui` localhost browser interface for tools/workflows/blueprints/vault/auth/history; htmx-driven, mountable MCP endpoint
- [x] **MCP resources** — `factorly://tools/...`, `factorly://workflows/...`, `factorly://blueprints/...` URIs with `notifications/resources/list_changed` on config reload
- [x] **Command-specific filtering** — per-tool output filter engine with strip/keep lines, match_output short-circuit, regex replace, head/tail, max_lines, json_path, pipe + 27 built-in filters

## Future?

### Agent & workflows
- [ ] **`factorly agent`** — lightweight agent loop with tool calling, oversight, and run summary
- [ ] **PreToolUse hooks** — `factorly hooks install` to intercept agent Bash calls, rewriting them to `factorly exec` for compression and logging without MCP ([spec](hooks-spec.md))
- [ ] **Response caching** — return cached results for identical tool calls within a configurable window, reducing API quota usage and latency

### AARM conformance

Working toward [AARM](https://aarm.dev) (Autonomous Action Runtime Management) Core conformance. AARM defines the security spec for AI agent tool-use interception — pre-execution governance, tamper-evident audit, and identity binding. Factorly already satisfies R1 (pre-execution interception). The remaining gaps are listed below, roughly ordered by value and tractability. Several require the agent harness or a hosted version of Factorly to be fully useful.

#### Already satisfied
- [x] **R1: Pre-execution interception** — shadow governance intercepts every call; deny blocks before execution; confirm pauses; rate-limit and loop detection block; all decisions logged with reason

#### Near-term (local CLI)
- [ ] **R5: Hash-chained audit logs** — append SHA-256 hash of (previous_hash + entry) to each JSONL log entry, creating a tamper-evident chain that detects retroactive modification
- [ ] **R5: Signed receipts** — Ed25519 signatures on log entries for offline verification of audit trail integrity
- [x] **R4: MODIFY decision** — parameter coercion before execution: type casting, number clamping to min/max, string truncation to max_length, boolean normalization; original values preserved in audit log
- [x] **R3: Parameter validation** — type checking (integer/number/boolean/json), min/max range, min_length/max_length, regex pattern, enum allowlists on tool parameters

#### Requires agent harness
- [ ] **R4: DEFER decision** — suspend actions when context is insufficient or ambiguous, resume when context is collected or timeout triggers denial
- [ ] **R2: Context accumulation** — feed accumulated session state (prior actions, data classifications, original user request) into policy evaluation
- [ ] **R3: Context-dependent policy** — policies that evaluate against accumulated context, not just static tool names (e.g., "3rd delete in a row", "action diverges from stated intent")
- [ ] **R7: Semantic distance tracking** — cosine similarity between agent actions and stated intent, with drift thresholds triggering alerts or deferral

#### Requires hosted Factorly
- [ ] **R6: Full identity binding** — multi-level identity: human principal, service identity, agent identity, session, role/privilege scope
- [ ] **R9: Least privilege enforcement** — JIT credential issuance with per-operation scoping and minimal validity periods
- [ ] **R8: Telemetry export** — structured event streaming (OCSF/CEF) to SIEM/SOAR platforms in real time

### Observability
- [ ] **Dashboard** — localhost web UI showing live call feed, savings counter, agent activity, blocked call breakdown
- [ ] **Webhook notifications** — alert on blocked calls, rate limits, errors via webhook (Slack, Telegram, HTTP)
- [ ] **`factorly report`** — generate a shareable summary of tool usage, savings, and oversight actions for a time period

### Security & auth
- [ ] **OAuth 2.1 server** — MCP-spec authorization server with PKCE, dynamic client registration, and token endpoints ([spec](oauth-server-spec.md))
- [ ] **Hosted OAuth** — Factorly-managed OAuth app registrations for Google, Microsoft, Salesforce (eliminates provider-side app setup)
- [ ] **Config signing** — verify tool config integrity hasn't been tampered with post-deployment
- [ ] **Secret rotation helpers** — workflows for rotating vault keys with zero downtime

### Scale & collaboration
- [ ] **Team configs** — shared tool definitions and oversight policies across a team
- [ ] **Shared credential vault** — team-scoped secrets with role-based access
- [ ] **Cost attribution** — track estimated API call costs per agent per tool with budget enforcement

### Ecosystem
- [ ] **Community blueprint registry** — `factorly blueprint browse --remote` to discover community-contributed blueprints alongside the bundled catalog
- [ ] **Plugin system** — custom providers beyond CLI/REST/MCP
- [ ] **CI/CD integration** — GitHub Actions, GitLab CI recipes for governed tool access in pipelines
- [ ] **Terraform/Pulumi provider** — manage Factorly configs as infrastructure

---

[← Back to Documentation](README.md)
