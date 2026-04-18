# Wrap a CLI Tool

Expose a team script as a governed tool with vault-injected secrets, output compression, timeouts, and confirmation prompts.

## Config

```yaml
# .factorly/tools/deploy.yaml
tools:
  deploy:
    type: cli
    description: "Deploy a service to the specified environment"
    command: ./scripts/deploy.sh
    args: ["--service", "{{service}}", "--env", "{{env}}", "--tag", "{{tag}}"]
    env:
      AWS_ACCESS_KEY_ID: "{{vault:AWS_ACCESS_KEY_ID}}"
      AWS_SECRET_ACCESS_KEY: "{{vault:AWS_SECRET_ACCESS_KEY}}"
      AWS_REGION: us-east-1
      DEPLOY_TOKEN: "{{vault:DEPLOY_TOKEN}}"
    env_isolation: strict
    timeout: 120s
    max_output: 50000
    compress: ["logs"]
    shadow:
      confirm: true
      rate_limit: 5/hour
      log_params: [service, env, tag]
```

## Usage

```bash
# Store secrets
factorly vault set AWS_ACCESS_KEY_ID AKIA_xxxxxxxxxxxx
factorly vault set AWS_SECRET_ACCESS_KEY xxxxxxxxxxxxxxxxxxxx
factorly vault set DEPLOY_TOKEN dt_xxxxxxxxxxxx

# Deploy to staging
factorly call deploy --service payments-api --env staging --tag v1.4.2
# ⚠ Tool "deploy" requires confirmation. Proceed? (y/n)
# y
# Deploying payments-api@v1.4.2 to staging...
# [output compressed, logged]

# Check tool status
factorly tools status
```

## What happens

1. The agent calls `deploy` with service, env, and tag parameters.
2. Factorly prompts for confirmation (shadow `confirm: true`).
3. On approval, Factorly injects AWS credentials and the deploy token from the vault into the subprocess environment.
4. `env_isolation: strict` ensures only essential variables plus the explicit `env` entries reach the script — no leaking of host environment.
5. The script runs with a 120-second timeout. If it hangs, Factorly kills it.
6. Output is compressed (repeated log lines deduplicated) and capped at 50KB before returning to the agent.
7. The call is logged with `service`, `env`, and `tag` highlighted in the audit trail.
8. Rate limit of 5/hour prevents the agent from deploying in a loop.

---

[← Back to Examples](README.md)
