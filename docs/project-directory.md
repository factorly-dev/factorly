# Project Directory

Factorly supports a `.factorly/` project directory for per-repo tool configs — similar to `.github/`, `.vscode/`, or `.cursor/`:

```
my-project/
├── .factorly/
│   ├── factorly.yaml        # project config (optional)
│   ├── tools/               # modular tool files (optional)
│   │   ├── slack.yaml
│   │   └── github.yaml
│   └── petstore.yaml        # loose tool files work too
├── factorly.yaml            # top-level config (optional, merges with .factorly/)
└── ...
```

## How it loads

1. Top-level `factorly.yaml` in cwd (if present, merges `.factorly/`)
2. `.factorly/factorly.yaml` (if no top-level config)
3. Loose YAML files in `.factorly/` directory
4. `.factorly/tools/` directory — always scanned if it exists (this is where templates write to)
5. `tools_dir` if specified in config
6. `~/.config/factorly/factorly.yaml` (user-level fallback)

## Tool files

Each YAML file in a tools directory is a flat map of tool definitions — no wrapper key needed:

```yaml
# .factorly/tools/slack.yaml
slack.post:
  type: rest
  base_url: https://slack.com/api
  method: POST
  path: /chat.postMessage
  auth:
    type: bearer
    token: "{{vault:SLACK_TOKEN}}"
  parameters:
    - name: channel
      required: true
    - name: text
      required: true
```

## tools_dir

Use `tools_dir` in your config to point to a tools directory:

```yaml
# .factorly/factorly.yaml
tools_dir: ./tools
tools:
  web.fetch:
    type: cli
    command: curl
    args: ["-s", "{{url}}"]
```

Tools from `tools_dir` merge with inline tools. Duplicate names across files produce an error.

---

[← Back to README](../README.md)
