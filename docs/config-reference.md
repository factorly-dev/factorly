# Config Reference

## YAML Schema

```yaml
# factorly.yaml or .factorly/factorly.yaml
tools_dir: ./tools              # optional, scan directory for tool files

tools:
  <tool-name>:
    type: cli | rest | mcp      # required
    description: "..."          # optional, shown to agent

    # For CLI commands:
    command: curl               # executable to run
    args: ["-s", "{url}"]      # {param} placeholders are substituted
    stdin: "{input}"            # optional, pipe to subprocess stdin
    interactive: true            # optional, connect to terminal (TTY)

    # For MCP servers (stdio — spawn subprocess):
    command: npx                # executable to start the server
    args: ["@org/server-name"] # arguments
    env:                        # environment variables
      KEY: ${vault:SECRET}

    # For MCP servers (HTTP — connect to remote):
    url: http://host:3000/mcp  # server URL

    # For REST APIs:
    base_url: https://api.example.com
    method: GET                 # GET, POST, PUT, PATCH, DELETE
    path: /items/{id}           # {param} placeholders in path
    headers:                    # static headers (optional)
      Accept: application/json
    auth:                       # optional
      type: bearer              # bearer, basic, header, or oauth
      token: ${vault:API_KEY}   # vault ref or ${ENV_VAR}
      # header: X-Api-Key       # for header type
      # value: ${vault:KEY}     # for header type
    parameters:
      - name: id
        in: path                # path, query, header, or body
        required: true
      - name: limit
        in: query
```

## Secret references

Use `${ENV_VAR}` for environment variables or `${vault:KEY}` for encrypted vault secrets. Both are resolved at startup before the agent sees anything.

## Parameter routing

Parameters are routed by their `in` field. When `in` is omitted, defaults to `query` for GET/DELETE or `body` for POST/PUT/PATCH.

## Stdin

CLI tools can pipe a parameter to the subprocess's stdin using the `stdin` field with `{param}` placeholders:

```yaml
tools:
  jq.filter:
    type: cli
    command: jq
    args: ["{filter}"]
    stdin: "{input}"

  clipboard.copy:
    type: cli
    description: "Copy text to the system clipboard"
    command: pbcopy        # macOS (use xclip -selection clipboard on Linux)
    stdin: "{text}"
```

```bash
factorly call jq.filter --filter ".name" --input '{"name":"Jordan","role":"VP Eng"}'
factorly call clipboard.copy --text "copied to clipboard"
```

## Interactive mode

CLI tools that need a TTY (database shells, SSH, REPLs) can set `interactive: true` to connect the subprocess directly to your terminal:

```yaml
tools:
  db.shell:
    type: cli
    command: psql
    args: ["-h", "localhost", "-U", "{user}", "{database}"]
    interactive: true
    env:
      PGPASSWORD: ${vault:DB_PASSWORD}

  ssh.connect:
    type: cli
    command: ssh
    args: ["{host}"]
    interactive: true
```

```bash
factorly call db.shell --user admin --database myapp
# Drops into an interactive psql session with vault-injected password

factorly call ssh.connect --host prod-server
# Opens an interactive SSH session
```

Interactive tools connect stdin/stdout/stderr directly to your terminal. Output is not captured (Result is empty), but the call is still logged with tool name, params, duration, and exit code. Interactive mode only works via `factorly call`, not through `factorly serve` (MCP has no terminal).

## Parameter inference

For CLI tools, parameters are automatically inferred from `{placeholder}` patterns in `args` and `stdin`.

---

[← Back to README](../README.md)
