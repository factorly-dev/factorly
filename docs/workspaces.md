# Workspaces

Workspaces are named variable bundles that overlay your project config at load time. Pick a workspace with `--workspace <name>` (or `FACTORLY_WORKSPACE=<name>`) and `{{env:NAME}}` references in `factorly.yaml` resolve to that workspace's values. Same tools, same project, different runtime context — dev vs staging vs prod, work vs personal account, two different customers.

## Quick start

```yaml
# .factorly/factorly.yaml
tools:
  github.api:
    type: rest
    base_url: "{{env:GITHUB_BASE}}"
    auth:
      type: bearer
      token: "{{vault:GITHUB_TOKEN}}"
```

```yaml
# .factorly/workspaces/staging.yaml
description: Staging GitHub Enterprise tenant
vars:
  GITHUB_BASE: https://ghe-staging.acme.com/api/v3
```

```yaml
# .factorly/workspaces/prod.yaml
description: Production GitHub
vars:
  GITHUB_BASE: https://api.github.com
```

```bash
factorly call github.api.list_repos --workspace staging
factorly call github.api.list_repos --workspace prod
```

The same tool definition runs against two different APIs. No YAML editing between switches.

## Selection

Four ways to pick a workspace, in priority order:

1. `--workspace <name>` (or `-w <name>`) on the command line
2. `FACTORLY_WORKSPACE=<name>` environment variable
3. **`default` workspace** — if `.factorly/workspaces/default.yaml` exists and neither (1) nor (2) is set, it loads automatically
4. None — `{{env:NAME}}` falls through to `os.Getenv`, same as before workspaces existed

`factorly init` creates `default.yaml` with `vars: {}` so every fresh project has a place to start overlaying variables without thinking about workspace selection. To opt out of auto-loading, delete `default.yaml`.

Set `FACTORLY_WORKSPACE` in your shell profile if you want a non-default workspace active across all commands.

## Layout

```
<project>/
├── .factorly/
│   ├── factorly.yaml                 # base config
│   ├── workspaces/
│   │   ├── default.yaml              # auto-loaded when no flag is set
│   │   ├── staging.yaml              # opt-in via --workspace staging
│   │   └── prod.yaml
│   ├── vaults/                       # optional per-workspace vaults
│   │   ├── staging.enc
│   │   └── prod.enc
│   └── vault.enc                     # project vault (shared across workspaces)
```

Workspace files are plain YAML. Edit them in any text editor — there's no `factorly workspaces create` command.

## Vault selection

Each workspace can have its own encrypted vault file. When `--workspace staging` is active, the resolver checks `.factorly/vaults/staging.enc` first, then falls through to the project vault, then to the global vault. Each tier can have its own password.

```bash
# Create a workspace vault by writing to it.
factorly vault set --workspace staging GITHUB_TOKEN ghp_staging_xxx

# Shared secrets stay in the project vault.
factorly vault set GITHUB_TOKEN ghp_default_xxx

# Reads follow the chain.
factorly call github.api.list_repos --workspace staging
# → looks up GITHUB_TOKEN in vaults/staging.enc first; falls through to vault.enc if absent.
```

**Workspace vaults are optional.** A workspace that doesn't need its own secrets simply doesn't have a vault file — calls fall straight through to the project vault.

**Writes target a single tier.** `vault set --workspace staging KEY value` writes to `vaults/staging.enc` only. Without `--workspace`, writes target the project vault. Writes never fall through.

## OAuth tokens

OAuth bundles are stored in the vault under a deterministic key (e.g. `github_oauth`), so per-workspace vaults give per-workspace OAuth tokens for free:

```bash
factorly auth login github --workspace staging   # token → vaults/staging.enc
factorly auth login github --workspace prod      # token → vaults/prod.enc
factorly call github.list_repos --workspace staging  # uses staging token
```

Same OAuth provider, two separate logins, no overlap.

**Refresh promotes a token to the active vault.** If a token lives only in the project vault (`factorly auth login github` with no workspace), then later you call a tool under `--workspace staging` and the token expires, the refreshed bundle is written to `vaults/staging.enc` — the active "Primary" tier. Subsequent refreshes stay local to the workspace.

If you want a single OAuth login shared across workspaces, log in *without* `--workspace` so the bundle lands in the project vault, and don't authenticate per-workspace. The chain will fall through transparently — until a refresh under an active workspace promotes the token.

To avoid the surprise, prefer one of:

  - **Per-workspace logins.** `factorly auth login <provider> --workspace <name>` for each environment. Most common.
  - **`factorly auth logout <provider> --workspace <name>`** removes a workspace's token without touching the project vault.

## Precedence

For `{{env:NAME}}` resolution:

1. Workspace var (if `--workspace` active and `vars.NAME` exists)
2. `os.Getenv("NAME")`
3. The default after the `|`, e.g. `{{env:API_BASE|http://localhost:3000}}`
4. Unresolved — left literal

For `{{vault:KEY}}` resolution (workspace active):

1. `.factorly/vaults/<workspace>.enc`
2. `.factorly/vault.enc` (project vault)
3. `~/.config/factorly/vault.enc` (global vault)
4. `ErrNotFound`

## Commands

```bash
factorly workspaces           # list available workspaces
factorly workspaces list      # same
factorly workspaces show <n>  # print a workspace's vars (secret-looking values masked)

factorly call <tool> --workspace <n>             # one-shot
factorly call <tool> -w <n>                      # short flag

FACTORLY_WORKSPACE=<n> factorly call <tool>      # shell-wide

factorly vault set --workspace <n> KEY value     # write to workspace vault
factorly vault list --workspace <n>              # list (workspace + fallback chain)
```

## Passwords

The workspace vault password is resolved in this order:

1. `FACTORLY_WORKSPACE_VAULT_PASSWORD_<UPPER_NAME>` env var (e.g. `FACTORLY_WORKSPACE_VAULT_PASSWORD_STAGING`)
2. `FACTORLY_VAULT_PASSWORD` env var (convenience — share one password across all vaults)
3. `.factorly/vaults/<name>.key` key file
4. Interactive prompt

CI typically sets `FACTORLY_VAULT_PASSWORD` and gets a single password for everything; local dev usually uses a key file or the prompt.

## Audit log

When a workspace is active, every call entry in `audit.jsonl` carries a `workspace` field:

```json
{"timestamp":"...","tool":"github.api.list_repos","workspace":"staging","status":"success", ...}
```

Calls made without a workspace omit the field. Filter by workspace with `jq`:

```bash
jq 'select(.workspace == "prod")' < .factorly/audit.jsonl
```

## Gotchas

- **Workspace names can't contain `/`, `\\`, or `.`** — they map to a filename.
- **`{{env:NAME}}` is the resolution point.** A workspace var named `API_BASE` doesn't affect `os.Environ()` for spawned CLI tools; it only overlays template references inside `factorly.yaml`. To pass a workspace var to a tool's process, reference it explicitly: `env: {API_BASE: "{{env:API_BASE}}"}`.
- **Global configs don't get workspaces.** A `factorly.yaml` under `~/.config/factorly/` has no `.factorly/workspaces/` to load from — workspaces are a per-project feature.
- **Runtime state (`audit.jsonl`, `ratelimit.json`, `runs/`) is shared across workspaces.** Workspaces overlay config; they don't sandbox state. If you want fully isolated state, use separate project directories instead.

## See also

- [Vault](vault.md) — how secrets are stored and resolved.
- [Project directory](project-directory.md) — how `.factorly/` is laid out.
- [Logging](logging.md) — the audit log that records every call.
