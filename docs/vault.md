# Vault — Encrypted Secret Storage

Factorly includes an encrypted vault so secrets never live in plaintext `.env` files. Two-layer encryption: the vault file is encrypted with AES-256-GCM (Argon2id-derived key), and each secret value is independently encrypted with its own salt via HKDF-SHA256.

This means `vault list` never decrypts secret values, and `vault get` only decrypts the one you ask for. Identical values stored under different keys produce different ciphertext.

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

Use `${vault:KEY}` anywhere you'd use `${ENV_VAR}`:

```yaml
# Env var (plain text in environment)
token: "${GITHUB_TOKEN}"

# Vault (encrypted on disk, decrypted on demand)
token: "${vault:GITHUB_TOKEN}"
```

Both work. Mix and match per tool. Vault references are resolved at call time — the agent never sees either form.

## How it works

```
factorly vault set GITHUB_TOKEN
  → prompts for value (no echo)
  → derives per-entry key (master key + random salt → HKDF-SHA256)
  → encrypts value with AES-256-GCM
  → stores encrypted entry in vault index
  → re-encrypts vault file to disk

factorly call github.repos --username octocat
  → loads config, finds ${vault:GITHUB_TOKEN}
  → decrypts vault index (key names + encrypted blobs)
  → decrypts only the requested entry on demand
  → injects into Authorization header
  → agent sees only the response data
```

## Encryption details

| Layer | Algorithm | Key derivation | Purpose |
|-------|-----------|---------------|---------|
| File | AES-256-GCM | Password → Argon2id (128MB, 2 iterations) → master key | Protects key names + encrypted entry blobs |
| Per-entry | AES-256-GCM | Master key + 16-byte random salt → HKDF-SHA256 → entry key | Protects each secret value independently |

Each entry has its own random salt and nonce, regenerated on every write. Entry keys are zeroized immediately after use. The master key is zeroized on `Close()`.

## Vault password

The vault is locked with a master password. Resolved in order:

1. `FACTORLY_VAULT_PASSWORD` env var (CI/automation)
2. `~/.config/factorly/vault.key` file (headless servers)
3. Interactive prompt (normal dev UX)

## Vault path

Default: `~/.config/factorly/vault.enc`. Override with `--vault-path` flag or `FACTORLY_VAULT_PATH` env var.

## Migration

Existing vaults are automatically migrated to per-entry encryption on first open. No action needed — the upgrade is transparent and preserves all stored values.

## Extensible backends (future)

The vault uses a `Backend` interface. The local encrypted file is the default backend. Future backends:

```yaml
# Coming soon
token: "${1password:Development/GitHub/token}"
token: "${gcp-sm:project-id/GITHUB_TOKEN}"
token: "${aws-sm:prod/stripe-key}"
```

---

[← Back to README](../README.md)
