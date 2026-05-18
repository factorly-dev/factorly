# Workspaces — same tools, two environments

This example shows how a single `factorly.yaml` can target a staging API and a production API just by switching `--workspace`. Same idea Postman has for environments.

## Project layout

```
my-project/
└── .factorly/
    ├── factorly.yaml
    └── workspaces/
        ├── default.yaml     # auto-loaded by factorly init
        ├── staging.yaml
        └── prod.yaml
```

`factorly init` creates `default.yaml` for you. It auto-loads when no `--workspace` flag is set, so unflagged calls always have an environment active. Add `staging.yaml` and `prod.yaml` by hand.

## Config

```yaml
# .factorly/factorly.yaml
tools:
  api.fetch:
    type: cli
    description: Hit the active environment's API
    command: curl
    args: ["-s", "{{env:API_BASE}}/health"]
```

```yaml
# .factorly/workspaces/staging.yaml
description: Staging tenant
vars:
  API_BASE: https://api.staging.example.com
```

```yaml
# .factorly/workspaces/prod.yaml
description: Production
vars:
  API_BASE: https://api.example.com
```

## Usage

```bash
factorly workspaces
# NAME       VARS  DESCRIPTION
# default *  0     Default workspace (auto-loaded when no --workspace is set)
# prod       1     Production
# staging    1     Staging tenant

# No flag → default auto-loads.
factorly call api.fetch
# uses default.yaml's vars (empty here, so {{env:API_BASE}} is unresolved)

# Explicit flag → that workspace wins.
factorly call api.fetch --workspace staging
# {"ok":true,"region":"staging-us-west"}

factorly call api.fetch -w prod
# {"ok":true,"region":"prod-us-east"}

# Or set the workspace once for the shell.
export FACTORLY_WORKSPACE=staging
factorly call api.fetch
```

## Adding a workspace-specific secret

Workspaces optionally have their own encrypted vault. Useful when the same `{{vault:KEY}}` reference should resolve to different tokens per environment.

```bash
# Different tokens per workspace, same {{vault:GITHUB_TOKEN}} reference in YAML.
factorly vault set --workspace staging GITHUB_TOKEN ghp_staging_xxx
factorly vault set --workspace prod    GITHUB_TOKEN ghp_prod_xxx

# Shared tokens stay in the project vault — no need to duplicate.
factorly vault set INTERNAL_CA_TOKEN ca_xxx
```

When a workspace is active, `{{vault:GITHUB_TOKEN}}` reads from the workspace vault first, then falls through to the project vault, then to the global vault. `{{vault:INTERNAL_CA_TOKEN}}` (only in the project vault) falls through transparently.

## What happens

1. At config-load, `factorly` reads `.factorly/workspaces/staging.yaml`.
2. The `vars` map populates the `env` backend's overrides.
3. `{{env:API_BASE}}` in `factorly.yaml` resolves to `https://api.staging.example.com`.
4. The tool's `args` get the staging URL; curl runs against the staging API.
5. The audit log entry carries `"workspace":"staging"` so you can later filter by environment.

## See also

- [docs/workspaces.md](../workspaces.md) — full reference: vault chain, precedence, gotchas.
