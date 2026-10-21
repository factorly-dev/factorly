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
# Export vault secrets straight into the environment — no {{vault:...}} spelling needed
factorly exec --vault GITHUB_TOKEN -- gh pr list
factorly exec --vault GH_TOKEN=GITHUB_TOKEN -- ./script.sh
factorly exec --vault 'STRIPE_*' -- npm run sync
```

```bash
# Combine strict isolation with vault secrets: the command sees DATABASE_URL and nothing else
factorly exec --env-isolation strict --vault DATABASE_URL -- psql "{{env:DATABASE_URL}}" -c "SELECT version()"
```

```bash
# Long-running commands are not cut off — there is no timeout unless you ask for one
factorly exec -- make release
factorly exec --timeout 10m -- make release   # exit 124 + "timed out after 10m" if it overruns
```

## What happens

1. `factorly exec` resolves `{{vault:KEY}}` and `{{env:VAR}}` references in `--env` and `--vault` values, then in the arguments, before the shell sees them. Secrets stay out of `ps` output and shell history. A secret that cannot be resolved stops the command instead of running it with a placeholder.
2. The command runs through the same code path as config-based CLI tools — compression (`--compress`), truncation (`--max-output`), and environment isolation (`--env-isolation`) all apply.
3. Output is compressed with mode `all` by default. Use `--compress none` to disable.
4. Every execution is logged to the JSONL audit trail with `interface: "exec"`, including tool name, duration, and status.
5. The command's exit code is preserved — `factorly exec` exits with the same code as the wrapped command. A `--timeout` overrun exits 124.

---

[← Back to Examples](README.md)
