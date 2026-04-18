# Hello World

The simplest Factorly tool: wrap the `echo` command as a callable CLI tool.

## Config

```yaml
# .factorly/factorly.yaml
tools:
  echo:
    type: cli
    description: "Echo a message back"
    command: echo
    args: ["{{text}}"]
```

## Usage

```bash
# Initialize and add the config above
factorly init

# Call the tool
factorly call echo --text "hello"
# Output: hello

# View the audit log entry
factorly logs -n 1
```

## What happens

1. `factorly call` finds the `echo` tool in your config.
2. It substitutes `{{text}}` with the value you passed (`"hello"`).
3. It runs `echo hello` as a subprocess and returns the output.
4. The call is recorded in the audit log — visible with `factorly logs`.

---

[← Back to Examples](README.md)
