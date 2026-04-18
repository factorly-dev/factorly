# REST API with Bearer Token

Call the GitHub API using a personal access token stored in the encrypted vault.

## Config

```yaml
# .factorly/factorly.yaml
tools:
  github.repos:
    type: rest
    description: "List repositories for a GitHub user"
    base_url: https://api.github.com
    method: GET
    path: /users/{{username}}/repos
    headers:
      Accept: application/vnd.github+json
    auth:
      type: bearer
      token: "{{vault:GITHUB_TOKEN}}"
    parameters:
      - name: username
        in: path
        required: true
      - name: per_page
        in: query
      - name: sort
        in: query
```

## Usage

```bash
# Store your token in the vault
factorly vault set GITHUB_TOKEN ghp_xxxxxxxxxxxxxxxxxxxx

# Call the tool
factorly call github.repos --username octocat --per_page 5 --sort updated
```

## What happens

1. Factorly decrypts `GITHUB_TOKEN` from the vault.
2. It builds the request: `GET https://api.github.com/users/octocat/repos?per_page=5&sort=updated` with an `Authorization: Bearer ghp_xxx...` header.
3. The response JSON is returned to your agent or terminal.
4. The token never appears in output, logs, or agent context.

---

[← Back to Examples](README.md)
