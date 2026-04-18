# External Secrets with 1Password

Use the 1Password CLI as an external vault backend so Factorly pulls secrets directly from 1Password instead of its built-in vault.

## Config

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

tools:
  github.repos:
    type: rest
    description: "List repositories for a GitHub user"
    base_url: https://api.github.com
    method: GET
    path: /users/{{username}}/repos
    auth:
      type: bearer
      token: "{{op:GitHub/token}}"
    parameters:
      - name: username
        in: path
        required: true
```

Note the `{{op:GitHub/token}}` reference. The `op` prefix matches the backend name, and `GitHub/token` is the 1Password item path passed as `{{key}}`.

## Usage

```bash
# Make sure the 1Password CLI is installed and authenticated
op signin

# Verify Factorly can reach 1Password
factorly vault --backend op get GitHub/token

# Call a tool — the token is fetched from 1Password at call time
factorly call github.repos --username octocat

# List secrets available in the 1Password backend
factorly vault --backend op list
```

## What happens

1. Factorly sees `{{op:GitHub/token}}` in the tool config and identifies `op` as an external vault backend.
2. At call time, it runs `op read "op://Development/GitHub/token"` to fetch the secret from 1Password.
3. The secret is injected into the `Authorization: Bearer` header for the API request.
4. The secret is never written to disk by Factorly — it lives only in 1Password.
5. Audit logs record the tool call but never the secret value.

---

[← Back to Examples](README.md)
