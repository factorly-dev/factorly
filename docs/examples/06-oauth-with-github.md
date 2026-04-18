# OAuth with GitHub

Set up OAuth 2.0 authentication for GitHub, so Factorly handles the browser login flow, token storage, and automatic refresh.

## Config

```yaml
# .factorly/factorly.yaml
oauth_providers:
  github:
    client_id: "{{vault:GITHUB_CLIENT_ID}}"
    client_secret: "{{vault:GITHUB_CLIENT_SECRET}}"
    auth_url: https://github.com/login/oauth/authorize
    token_url: https://github.com/login/oauth/access_token
    scopes: ["repo", "read:user"]

tools:
  github.repos:
    type: rest
    description: "List your repositories"
    base_url: https://api.github.com
    method: GET
    path: /user/repos
    auth:
      type: oauth
      provider: github
    parameters:
      - name: per_page
        in: query
      - name: sort
        in: query

  github.profile:
    type: rest
    description: "Get your GitHub profile"
    base_url: https://api.github.com
    method: GET
    path: /user
    auth:
      type: oauth
      provider: github
```

## Usage

```bash
# 1. Create a GitHub OAuth App at https://github.com/settings/developers
#    Set redirect URI to http://localhost:18019/callback

# 2. Store your OAuth credentials
factorly vault set GITHUB_CLIENT_ID "Iv1.xxxxxxxxxxxx"
factorly vault set GITHUB_CLIENT_SECRET "xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx"

# 3. Log in — opens your browser
factorly auth login github
# Opening browser for authorization...
# Authenticated with github (expires in 59m)

# 4. Call a tool
factorly call github.repos --per_page 5 --sort updated

# 5. Check token status
factorly auth status
# github          ✓ valid (expires in 47m)
```

## What happens

1. `factorly auth login github` starts a local callback server and opens your browser to GitHub's authorization page (with PKCE).
2. You authorize the app in the browser. GitHub redirects back to Factorly's local server.
3. Factorly exchanges the authorization code for access and refresh tokens, then stores them in the encrypted vault.
4. When you call a tool that uses the `github` provider, Factorly injects the access token automatically.
5. When the token expires, Factorly uses the refresh token to get a new one — no manual intervention.

---

[← Back to Examples](README.md)
