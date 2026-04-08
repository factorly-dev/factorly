# OAuth Authentication

For tools that use OAuth 2.0 (Google, GitHub, Microsoft, Slack), Factorly handles the browser-based login flow, stores tokens in the vault, and auto-refreshes expired access tokens before each request.

## Configure an OAuth provider

```yaml
# Shared provider definition (reused across tools)
oauth_providers:
  google:
    client_id: {{vault:GOOGLE_CLIENT_ID}}
    client_secret: {{vault:GOOGLE_CLIENT_SECRET}}
    auth_url: https://accounts.google.com/o/oauth2/v2/auth
    token_url: https://oauth2.googleapis.com/token
    scopes: ["https://www.googleapis.com/auth/drive.readonly"]

tools:
  google.files:
    type: rest
    base_url: https://www.googleapis.com
    method: GET
    path: /drive/v3/files
    auth:
      type: oauth
      provider: google
```

Or inline for a single tool:

```yaml
tools:
  github.repos:
    type: rest
    base_url: https://api.github.com
    method: GET
    path: /user/repos
    auth:
      type: oauth
      client_id: {{vault:GITHUB_CLIENT_ID}}
      client_secret: {{vault:GITHUB_CLIENT_SECRET}}
      auth_url: https://github.com/login/oauth/authorize
      token_url: https://github.com/login/oauth/access_token
      scopes: ["repo"]
      token_key: github_oauth
```

## Example: GitHub OAuth

See [`examples/github-oauth.yaml`](../src/examples/github-oauth.yaml) for a complete working example.

```bash
# 1. Create a GitHub OAuth App at https://github.com/settings/developers
#    Set callback URL to http://127.0.0.1

# 2. Store credentials
factorly vault set GITHUB_CLIENT_ID "Iv1.xxx"
factorly vault set GITHUB_CLIENT_SECRET "xxx"

# 3. Login — opens browser
factorly auth login github
# Opening browser for authorization...
# Authenticated with github (expires in 59m)

# 4. Use it
factorly call github.repos --per_page 5
factorly call github.profile

# Check token status
factorly auth status
# github_oauth          ✓ valid (expires in 47m)

# Tokens refresh automatically — no manual intervention
```

## How it works

1. `factorly auth login` opens your browser to the provider's authorization page (with PKCE)
2. You authorize the app, the browser redirects to a local callback server
3. Factorly exchanges the authorization code for access + refresh tokens
4. Tokens are stored as a JSON bundle in the encrypted vault
5. On each `factorly call`, the REST provider checks token expiry (with 30s skew)
6. If expired, it automatically refreshes using the refresh token and updates the vault
7. The fresh access token is applied as a Bearer header — the agent never sees it

## Logout

```bash
factorly auth logout google
```

---

[← Back to README](../README.md)
