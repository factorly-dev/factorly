# Factorly Vault vs HashiCorp Vault

Different products solving different problems at different scales.

| | Factorly Vault | HashiCorp Vault |
|---|---|---|
| **What it is** | Encrypted file on disk | Distributed secrets infrastructure |
| **Target** | Individual developer / small team | Enterprise / platform teams |
| **Deployment** | None — it's a file | Server cluster, unsealing ceremony, audit backends |
| **Access** | CLI + `{{vault:KEY}}` in config | API, CLI, UI, agent sidecar |
| **Auth** | Single password (Argon2id) | 15+ auth methods (LDAP, OIDC, AWS IAM, K8s, etc.) |
| **Encryption** | AES-256-GCM, per-entry HKDF | AES-256-GCM, transit engine, auto-unseal |
| **Dynamic secrets** | No | Yes (DB creds, AWS keys, PKI certs on demand) |
| **Secret rotation** | Manual | Automatic with leases and TTLs |
| **Access control** | Per-project vault, disable commands | Fine-grained policies, namespaces, Sentinel |
| **Audit** | JSONL call log | Multiple audit backends (file, syslog, socket) |
| **HA / Replication** | No | Yes (multi-datacenter replication) |
| **Setup time** | 0 (just `vault set`) | Hours to days |
| **Dependencies** | None (single binary) | Consul/Raft storage, TLS, unsealing |
| **Cost** | Free | Open source + enterprise |

## When to use Factorly Vault

- You're a developer who wants secrets out of `.env` files
- You need per-project secret isolation
- You want zero infrastructure
- Your agent needs governed access to secrets

## When to use HashiCorp Vault

- You need dynamic secret generation
- You have 50+ services needing secrets
- You need enterprise audit/compliance
- You need automatic rotation with TTLs
- You have a platform team to operate it

## They're complementary

Factorly can use HashiCorp Vault as an external backend — no custom client needed:

```yaml
vault_backends:
  hcvault:
    type: cli
    get:
      command: vault
      args: ["kv", "get", "-field=value", "secret/{{key}}"]
    list:
      command: vault
      args: ["kv", "list", "-format", "yaml", "secret/"]
```

Then reference it in your tool configs:

```yaml
tools:
  github.repos:
    type: rest
    auth:
      type: bearer
      token: "{{hcvault:github-token}}"
```

Factorly Vault is `~/.ssh/id_rsa` to HashiCorp Vault's AWS KMS. One is personal, file-based, zero-ops. The other is infrastructure. Use both when it makes sense.

---

[← Back to Vault docs](vault.md) | [← Back to Documentation](README.md)
