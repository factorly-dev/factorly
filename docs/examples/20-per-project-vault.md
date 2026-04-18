# Per-Project Vault

Keep project secrets separate from personal ones using project and global vaults with independent passwords.

## Config

```yaml
# .factorly/factorly.yaml
tools:
  deploy:
    type: cli
    command: ./deploy.sh
    env:
      PROJECT_KEY: "{{vault:PROJECT_KEY}}"
      PERSONAL_KEY: "{{vault:PERSONAL_KEY}}"
```

## Usage

```bash
# Store a secret in the project vault (.factorly/vault.enc)
factorly vault set PROJECT_KEY proj-secret-123

# Store a secret in the global vault (~/.config/factorly/vault.enc)
factorly vault --global set PERSONAL_KEY my-global-secret

# Retrieve — project vault is checked first
factorly vault get PROJECT_KEY
# Output: proj-secret-123

# Falls back to global when key isn't in project vault
factorly vault get PERSONAL_KEY
# Output: my-global-secret

# List keys per vault
factorly vault list
# Output: PROJECT_KEY

factorly vault --global list
# Output: PERSONAL_KEY
```

## What happens

1. When `.factorly/` exists, `vault set` writes to `.factorly/vault.enc` (the project vault) by default.
2. `vault --global set` writes to `~/.config/factorly/vault.enc` instead.
3. Each vault has its own password. The project vault checks `FACTORLY_PROJECT_VAULT_PASSWORD`, then `FACTORLY_VAULT_PASSWORD`, then prompts. The global vault checks `FACTORLY_VAULT_PASSWORD`, then prompts.
4. `vault get` resolves project-first, global-fallback. `{{vault:KEY}}` references in configs follow the same order.
5. The global vault is opened lazily — if the key is found in the project vault, the global password is never requested.

---

[← Back to Examples](README.md)
