# Vault — Encrypted Secret Storage

Factorly's vault keeps the agent out of the credential entirely. Your tool's auth block references `{{vault:KEY}}`, Factorly resolves it at call time, and only the resolved request goes to the upstream API. The agent sees the response, never the key.

The vault is one piece of Factorly's workbench — tool definitions, workflows, governance, and audit live alongside it.

Under the hood: secrets never live in plaintext `.env` files. The vault file is encrypted in two layers with AES-256-GCM (Argon2id-derived key) as a whole and each secret value independently encrypted with its own salt via HKDF-SHA256. `vault list` never decrypts secret values, and `vault get` only decrypts the one you ask for. Identical values stored under different keys produce different ciphertext.

## Store and retrieve secrets

```bash
# Interactive — prompts for value with no echo
factorly vault set GITHUB_TOKEN

# Inline
factorly vault set STRIPE_KEY sk_live_xxxxxxxxxxxx

# Retrieve a secret (outputs raw value, pipe-friendly)
factorly vault get GITHUB_TOKEN

# List stored keys (values stay encrypted)
factorly vault list

# Remove a secret
factorly vault delete OLD_KEY
```

## Reference in config

Use `{{vault:KEY}}` anywhere you'd use `{{env:ENV_VAR}}`:

```yaml
# Env var (plain text in environment)
token: "{{env:GITHUB_TOKEN}}"

# Vault (encrypted on disk, decrypted on demand)
token: "{{vault:GITHUB_TOKEN}}"

# With a default fallback
base_url: "{{vault:API_URL|https://api.example.com}}"
```

Both work. Mix and match per tool. Vault references are resolved at call time — the agent never sees either form.

All `{{prefix:content}}` references use the same resolver pattern. Available backends:

| Prefix | Source | Example |
|--------|--------|---------|
| `vault` | Encrypted local vault | `{{vault:GITHUB_TOKEN}}` |
| `env` | Environment variable | `{{env:HOME}}` |
| `op` | 1Password (external backend) | `{{op:ABCDEF}}` |
| `expr` | [Expression function](expressions.md#value-expressions) | `{{expr:now()}}` |

All can be mixed in a single string: `{{vault:TOKEN}} expires {{expr:now('24h')}}`.

## How it works

```
factorly vault set GITHUB_TOKEN
  → prompts for value (no echo)
  → derives per-entry key (master key + random salt → HKDF-SHA256)
  → encrypts value with AES-256-GCM
  → stores encrypted entry in vault index
  → re-encrypts vault file to disk

factorly call github.repos --username octocat
  → loads config, finds {{vault:GITHUB_TOKEN}}
  → decrypts vault index (key names + encrypted blobs)
  → decrypts only the requested entry on demand
  → injects into Authorization header
  → agent sees only the response data
```

## Trust model: where `{{vault:K}}` references resolve

Factorly substitutes `{{backend:key}}` references in two distinct contexts. The trust model is different for each.

**Tool configuration (operator-authored, trusted).** Refs in `factorly.yaml` — `auth.token`, `args:`, `headers:`, parameter `default:` values, anything the operator wrote — resolve at config-load time. The operator wrote them; the vault password (or the equivalent) gates access. Standard behavior.

**Caller-supplied parameter values (potentially untrusted).** Refs that an agent, an MCP client, or a CLI invocation puts INTO a parameter value at call time. These default to a stricter rule:

- Safe backends (`env`, `store`, `expr`) always resolve. They don't expose anything the caller didn't already have.
- Secret backends (`vault`, `op`, external secret backends) **DO NOT resolve** unless the tool's parameter config opts in via `hydrate_vault_refs: true`. The literal template string flows through to the call body and the audit log.

Why: an LLM-generated parameter value can contain a literal `{{vault:KEY}}` string the model produced by accident or by injection. Without this gating, that string would get hydrated into the outbound call and into the audit log — leaking the secret into a place the agent (or downstream API) can read.

```yaml
# A tool with an HMAC-signing param that legitimately accepts a vault ref.
# The caller can pass {{vault:HMAC_KEY}} and it will resolve.
tools:
  api.signed:
    type: rest
    method: POST
    url: https://api.example.com/sign
    parameters:
      - name: signing_key
        required: true
        hydrate_vault_refs: true  # opt in for this param only
      - name: payload
        required: true
        # No flag → default-deny → caller can't sneak a vault ref into the payload.
```

If you're tempted to set `hydrate_vault_refs: true` widely, consider instead setting the parameter's `default:` to `"{{vault:K}}"` (config-authored, resolved at bootstrap from the operator's trusted config; the caller never has to know about the vault key).

When a secret-backend ref does resolve (config default firing, or an opt-in caller param), the audit log records the original `{{vault:K}}` template string in `params`, not the resolved secret. The provider call body itself does receive the resolved value — that's the whole point — so a tool that echoes its inputs back (like a debug API) can still leak through the response; this is documented separately.

## Encryption details

| Layer | Algorithm | Key derivation | Purpose |
|-------|-----------|---------------|---------|
| File | AES-256-GCM | Password → Argon2id (128MB, 2 iterations) → master key | Protects key names + encrypted entry blobs |
| Per-entry | AES-256-GCM | Master key + 16-byte random salt → HKDF-SHA256 → entry key | Protects each secret value independently |

Each entry has its own random salt and nonce, regenerated on every write. Entry keys are zeroized immediately after use. The master key is zeroized on `Close()`. The password is accepted as a byte slice and zeroed immediately after Argon2id key derivation — it does not persist in memory beyond vault initialization.

All vault operations (`get`, `set`, `delete`, `list`) are recorded in the [audit log](logging.md#vault-audit-trail). Secret values are never logged — only the operation and key name.

## Project vs Global Vault

Factorly supports per-project vaults. When a `.factorly/` directory exists, vault commands default to `.factorly/vault.enc` (project vault). The global vault at `~/.config/factorly/vault.enc` is the fallback.

```bash
# Default: writes to .factorly/vault.enc (if .factorly/ exists)
factorly vault set PROJECT_KEY secret

# Explicit global
factorly vault --global set PERSONAL_KEY secret

# vault get checks project first, falls back to global
factorly vault get PROJECT_KEY   # → from project vault
factorly vault get PERSONAL_KEY  # → falls back to global vault
```

### Resolution order

1. `--vault-path` flag (explicit override)
2. `--global` flag (forces global vault)
3. `FACTORLY_VAULT_PATH` env var
4. `.factorly/vault.enc` (project vault, if `.factorly/` directory exists)
5. `~/.config/factorly/vault.enc` (global fallback)

`{{vault:KEY}}` references in tool configs resolve from both vaults — project first, global fallback. No syntax change needed.

### Separate passwords

Each vault can have its own password:

**Project vault:**
1. `FACTORLY_PROJECT_VAULT_PASSWORD` env var
2. `FACTORLY_VAULT_PASSWORD` env var (shared fallback)
3. `.factorly/vault.key` file (0600 permissions)
4. Interactive prompt: `Vault password (project):`

**Global vault:**
1. `FACTORLY_VAULT_PASSWORD` env var
2. `~/.config/factorly/vault.key` file (0600 permissions)
3. Interactive prompt: `Vault password (global):`

The global vault is opened **lazily** — only when a key isn't found in the project vault. If all your secrets are in the project vault, the global password is never requested.

## Vault path

Default: `.factorly/vault.enc` (project) or `~/.config/factorly/vault.enc` (global). Override with `--vault-path` flag or `FACTORLY_VAULT_PATH` env var.

## Migration

Existing vaults are automatically migrated to per-entry encryption on first open. No action needed — the upgrade is transparent and preserves all stored values.

## External Backends

Define vault backends the same way you define tools — CLI commands that implement get and list. No vendor SDKs needed.

```yaml
# .factorly/factorly.yaml
vault_backends:
  op:
    type: cli
    get:
      command: op
      args: ["read", "op://Development/{{key}}"]
    list:
      command: op
      args: ["item", "list", "--format=json"]

  aws:
    type: cli
    get:
      command: aws
      args: ["secretsmanager", "get-secret-value", "--secret-id", "{{key}}", "--query", "SecretString", "--output", "text"]
    list:
      command: aws
      args: ["secretsmanager", "list-secrets", "--query", "SecretList[].Name", "--output", "text"]

  gcp:
    type: cli
    get:
      command: gcloud
      args: ["secrets", "versions", "access", "latest", "--secret={{key}}"]
    list:
      command: gcloud
      args: ["secrets", "list", "--format", "value(name)"]
```

Reference them in tool configs:

```yaml
tools:
  github.repos:
    type: rest
    auth:
      type: bearer
      token: "{{op:GITHUB_TOKEN}}"       # from 1Password
      # token: "{{aws:prod/github-key}}" # from AWS Secrets Manager
      # token: "{{gcp:github-token}}"    # from GCP Secret Manager
```

### How it works

External backends are **read-only** — manage secrets in the external tool directly. `{{key}}` in args is replaced with the requested key. Commands inherit the full parent environment (they need AWS credentials, `op` session tokens, gcloud auth, etc.).

If the local vault isn't configured, external backends still work. You can use `{{op:KEY}}` or `{{hcvault:KEY}}` without any local vault.

## See also

- [Factorly Vault vs HashiCorp Vault](vault-comparison.md) — when to use which

---

[← Back to README](../README.md)
