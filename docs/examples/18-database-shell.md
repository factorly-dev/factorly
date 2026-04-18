# Interactive Database Shell

Launch an interactive database session with vault-injected credentials. The agent or operator gets a live shell — Factorly handles the secrets.

## Config — PostgreSQL

```yaml
# .factorly/tools/db.yaml
tools:
  db.psql:
    type: cli
    description: "Open an interactive PostgreSQL shell"
    command: psql
    args: ["-h", "{{host}}", "-U", "{{user}}", "-d", "{{database}}"]
    interactive: true
    env:
      PGPASSWORD: "{{vault:PG_PASSWORD}}"
```

## Config — MySQL

```yaml
# .factorly/tools/db.yaml (continued or separate file)
tools:
  db.mysql:
    type: cli
    description: "Open an interactive MySQL shell"
    command: mysql
    args: ["-h", "{{host}}", "-u", "{{user}}", "-D", "{{database}}", "--password={{vault:MYSQL_PASSWORD}}"]
    interactive: true
```

## Usage

```bash
# Store credentials
factorly vault set PG_PASSWORD s3cret_pg_pass
factorly vault set MYSQL_PASSWORD s3cret_mysql_pass

# Open a PostgreSQL shell
factorly call db.psql --host db.internal.acme.com --user analyst --database analytics
# Drops into an interactive psql session

# Open a MySQL shell
factorly call db.mysql --host mysql.internal.acme.com --user readonly --database orders
# Drops into an interactive mysql session
```

## What happens

1. Factorly resolves the vault reference and injects the password into the subprocess environment (PostgreSQL) or command args (MySQL).
2. `interactive: true` connects stdin, stdout, and stderr directly to your terminal (TTY passthrough).
3. You get a fully interactive database shell — arrow keys, tab completion, `\d` commands all work.
4. Output is **not captured** — the result returned to Factorly is empty. This is by design: interactive sessions stream directly to the terminal.
5. The call is still logged in the audit trail with tool name, parameters (host, user, database), duration, and exit code.
6. Interactive mode only works via `factorly call`. Through `factorly serve` (MCP), there is no terminal to attach to, so interactive tools are unavailable.

---

[← Back to Examples](README.md)
