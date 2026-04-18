# Ad-hoc Commands with factorly exec

Run any command through Factorly's safety layer — compression, truncation, vault resolution, and audit logging — without writing a YAML config.

## Usage

```bash
# Simple command — logged and compressed by default
factorly exec -- git status
```

```bash
# Compress JSON output from a REST API
factorly exec --compress json -- curl -s https://api.github.com/repos/golang/go
```

```bash
# Set environment variables for the subprocess
factorly exec --env REGION=us-east-1 -- echo "Deploying to $REGION"
```

```bash
# Resolve vault secrets in arguments — the token never hits shell history
factorly exec -- curl -H "Authorization: Bearer {{vault:GITHUB_TOKEN}}" https://api.github.com/user
```

```bash
# Combine strict isolation with vault secrets
factorly exec --env-isolation strict --env DB_URL={{vault:DATABASE_URL}} -- psql "{{vault:DATABASE_URL}}" -c "SELECT version()"
```

## What happens

1. `factorly exec` resolves `{{vault:KEY}}` and `{{env:VAR}}` references in the arguments before the shell sees them. Secrets stay out of `ps` output and shell history.
2. The command runs through the same code path as config-based CLI tools — compression (`--compress`), truncation (`--max-output`), and environment isolation (`--env-isolation`) all apply.
3. Output is compressed with mode `all` by default. Use `--compress none` to disable.
4. Every execution is logged to the JSONL audit trail with `interface: "exec"`, including tool name, duration, and status.
5. The command's exit code is preserved — `factorly exec` exits with the same code as the wrapped command.

---

[← Back to Examples](README.md)
