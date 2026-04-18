# Multi-File Project Structure

Organize tools across multiple YAML files in a `.factorly/` directory for cleaner, team-friendly configs.

## Config

```yaml
# .factorly/factorly.yaml
tools_dir: ./tools

tools:
  health.check:
    type: cli
    command: curl
    args: ["-sf", "{{url}}/healthz"]
```

```yaml
# .factorly/tools/github.yaml
github.repos:
  type: rest
  base_url: https://api.github.com
  method: GET
  path: /users/{{username}}/repos
  auth:
    type: bearer
    token: "{{vault:GITHUB_TOKEN}}"
  parameters:
    - name: username
      in: path
      required: true

github.issues:
  type: rest
  base_url: https://api.github.com
  method: GET
  path: /repos/{{owner}}/{{repo}}/issues
  auth:
    type: bearer
    token: "{{vault:GITHUB_TOKEN}}"
  parameters:
    - name: owner
      in: path
      required: true
    - name: repo
      in: path
      required: true
```

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

The resulting directory layout:

```
my-project/
├── .factorly/
│   ├── factorly.yaml
│   └── tools/
│       ├── github.yaml
│       └── slack.yaml
└── ...
```

## Usage

```bash
# List all tools from every file
factorly tools
```

```
  health.check      cli   curl -sf {{url}}/healthz
  github.repos      rest  GET /users/{username}/repos
  github.issues     rest  GET /repos/{owner}/{repo}/issues
  slack.post        rest  POST /chat.postMessage
```

## What happens

1. Factorly loads `.factorly/factorly.yaml` and finds `tools_dir: ./tools`.
2. It scans `.factorly/tools/` and loads every `.yaml` file alphabetically (`github.yaml`, then `slack.yaml`).
3. Tools from all files are merged into a single registry. The inline `health.check` from `factorly.yaml` merges with tools from the `tools/` directory.
4. If two files define the same tool name, Factorly exits with a duplicate-detection error naming both files.
5. `factorly tools` lists the combined set — agents see one flat tool list regardless of how files are organized.

---

[← Back to Examples](README.md)
