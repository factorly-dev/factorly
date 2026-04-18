# Environment Isolation

Restrict the environment variables a tool can see using `env_isolation: strict` — the child process gets only the bare essentials.

## Config

```yaml
# .factorly/factorly.yaml
tools:
  deploy:
    type: cli
    command: ./deploy.sh
    args: ["--env", "{{environment}}"]
    env_isolation: strict
    env:
      AWS_PROFILE: "{{env:AWS_PROFILE}}"
      DEPLOY_TOKEN: "{{vault:DEPLOY_TOKEN}}"
    parameters:
      - name: environment
        required: true
```

## Usage

```bash
# See what a strict-isolated command actually receives
factorly exec --env-isolation strict -- env
```

```
PATH=/usr/local/bin:/usr/bin:/bin
HOME=/home/user
USER=user
LANG=en_US.UTF-8
TERM=xterm-256color
```

```bash
# Forward specific variables into the restricted environment
factorly exec --env-isolation strict --env AWS_PROFILE={{env:AWS_PROFILE}} -- env
```

```
PATH=/usr/local/bin:/usr/bin:/bin
HOME=/home/user
USER=user
LANG=en_US.UTF-8
TERM=xterm-256color
AWS_PROFILE=staging
```

```bash
# Call the configured tool — only PATH, HOME, USER, LANG, TERM + explicit env
factorly call deploy --environment production
```

## What happens

1. With `env_isolation: strict`, the child process starts with only `PATH`, `HOME`, `USER`, `LANG`, and `TERM` from the parent.
2. Variables listed in `env:` are added on top of that minimal set. Here `AWS_PROFILE` is forwarded from the host and `DEPLOY_TOKEN` is decrypted from the vault.
3. Everything else — `GITHUB_TOKEN`, `AWS_SECRET_ACCESS_KEY`, shell history, editor configs — is invisible to the subprocess.
4. Without `env_isolation` (the default), the child inherits the full parent environment plus any `env:` additions.

---

[← Back to Examples](README.md)
