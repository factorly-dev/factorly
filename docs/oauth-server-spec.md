# OAuth 2.1 Authorization Server — Implementation Spec

> **Status: Planned.** This document captures the design for MCP-spec-compliant OAuth 2.1 authorization on Factorly's HTTP transport. Not yet implemented — Factorly currently uses static Bearer token auth which works with all major MCP clients (Claude Code, Cursor, Codex). Build this when multi-tenant, hosted, or delegated identity use cases arise.

## Context

The [MCP authorization spec](https://modelcontextprotocol.io/specification/2025-03-26/basic/authorization) defines OAuth 2.1 with PKCE as the standard auth mechanism for HTTP transport. Authorization is **OPTIONAL** per the spec. When implemented, it requires:

- OAuth 2.1 with PKCE (authorization code grant)
- Metadata discovery (`GET /.well-known/oauth-authorization-server`)
- Dynamic client registration (`POST /register`)
- Token endpoints (`/authorize`, `/token`)
- Bearer tokens on every request
- HTTP 401 when auth required but not provided
- Token refresh via refresh_token grant

## Design

Factorly becomes its own OAuth 2.1 authorization server when `--http-oauth` is set. OAuth endpoints live alongside `/mcp` on the same HTTP server. For single-user local use, auto-approve mode skips user consent.

## Package: `internal/authserver/`

```
authserver/
  authserver.go       — Server struct, config, constructor, Stop()
  store.go            — In-memory token/client/code store with TTL sweep
  metadata.go         — GET /.well-known/oauth-authorization-server
  register.go         — POST /register (dynamic client registration, RFC 7591)
  authorize.go        — GET /authorize (auto-approve → redirect with code)
  token.go            — POST /token (code exchange, refresh, client_credentials)
  middleware.go        — Bearer token validation middleware
  authserver_test.go  — Full endpoint + flow tests
```

## Config

```go
type Config struct {
    Issuer          string        // e.g., "http://localhost:3000"
    AccessTokenTTL  time.Duration // default 1h
    RefreshTokenTTL time.Duration // default 24h
    AuthCodeTTL     time.Duration // default 10m
    AutoApprove     bool          // true = skip consent (single-user)
}
```

## Store

In-memory maps guarded by `sync.RWMutex`:

```go
type Store struct {
    clients  map[string]*ClientRecord  // client_id → record
    codes    map[string]*AuthCode      // code → single-use auth code with TTL
    tokens   map[string]*IssuedToken   // access_token → token
    refreshs map[string]*IssuedToken   // refresh_token → same token
}
```

Methods:
- `RegisterClient(rec)` — stores client, generates random client_id (16 bytes, base64url)
- `StoreAuthCode(code)` — stores with TTL, single-use
- `ConsumeAuthCode(code)` — validates expiry + single-use, returns code data
- `IssueTokens(clientID, scope, ttl)` — generates opaque 32-byte base64url tokens
- `ValidateAccessToken(token)` — lookup + expiry check
- `ConsumeRefreshToken(token)` — rotates (invalidate old, issue new)
- Background `Sweep()` goroutine removes expired entries

## Endpoints

### Metadata: `GET /.well-known/oauth-authorization-server`

Also handles `/.well-known/oauth-authorization-server/mcp` (mcp-go appends the MCP path component).

```json
{
  "issuer": "http://localhost:3000",
  "authorization_endpoint": "http://localhost:3000/authorize",
  "token_endpoint": "http://localhost:3000/token",
  "registration_endpoint": "http://localhost:3000/register",
  "response_types_supported": ["code"],
  "grant_types_supported": ["authorization_code", "refresh_token", "client_credentials"],
  "token_endpoint_auth_methods_supported": ["none", "client_secret_post"],
  "code_challenge_methods_supported": ["S256"],
  "scopes_supported": ["mcp"]
}
```

### Registration: `POST /register`

RFC 7591. Accepts `client_name`, `redirect_uris` (required, must be localhost HTTP or HTTPS), `grant_types`, `token_endpoint_auth_method`. Returns `201` with `client_id`.

### Authorization: `GET /authorize`

Params: `response_type=code`, `client_id`, `redirect_uri`, `state`, `code_challenge`, `code_challenge_method=S256`. In auto-approve mode, immediately generates auth code and redirects to `redirect_uri?code=<code>&state=<state>`.

### Token: `POST /token`

**`grant_type=authorization_code`**: Validates `code`, `client_id`, `redirect_uri`, `code_verifier`. PKCE verification: `SHA256(code_verifier)` vs stored `code_challenge` (constant-time). Returns access + refresh tokens.

**`grant_type=refresh_token`**: Validates `refresh_token`, `client_id`. Rotates tokens.

**`grant_type=client_credentials`**: Validates `client_id` + `client_secret`. Returns access token only.

## Middleware

```go
func (s *Server) Middleware(next http.Handler) http.Handler
```

Validates `Authorization: Bearer <token>` against store. On failure, returns `401` with `WWW-Authenticate: Bearer realm="factorly"` header (critical — mcp-go client uses this to trigger OAuth flow).

## Integration

```go
// serve_cmd.go
mux := http.NewServeMux()

if httpOAuth {
    oauthSrv := authserver.New(authserver.Config{
        Issuer:      computeIssuer(addr),
        AutoApprove: true,
    })
    mux.HandleFunc("GET /.well-known/oauth-authorization-server", oauthSrv.MetadataHandler())
    mux.HandleFunc("GET /.well-known/oauth-authorization-server/mcp", oauthSrv.MetadataHandler())
    mux.HandleFunc("GET /authorize", oauthSrv.AuthorizeHandler())
    mux.HandleFunc("POST /token", oauthSrv.TokenHandler())
    mux.HandleFunc("POST /register", oauthSrv.RegisterHandler())
    mux.Handle("/mcp", oauthSrv.Middleware(httpServer))
} else if httpToken != "" {
    mux.Handle("/mcp", tokenAuthMiddleware(httpServer, httpToken))
} else {
    mux.Handle("/mcp", httpServer)
}
```

New flag: `--http-oauth` (mutually exclusive with `--http-token`).

## mcp-go Client Compatibility

Discovery sequence:
1. `GET /.well-known/oauth-protected-resource` → 404 (skip)
2. `GET /.well-known/oauth-authorization-server/mcp` → 200 ✓
3. `POST /register` → 201 ✓
4. `GET /authorize?...` with PKCE → redirect with code ✓
5. `POST /token` with code + verifier → tokens ✓
6. `Authorization: Bearer <token>` on `/mcp` → accepted ✓

## Security Properties

- Opaque tokens: 32 random bytes, base64url (no structure, no info leakage)
- PKCE required for all public clients (OAuth 2.1 mandate)
- Single-use auth codes with 10-minute TTL
- Refresh token rotation (old refresh invalidated on each use)
- Constant-time comparison for PKCE and token validation
- Tokens only in Authorization header, never in URL query string
- Redirect URIs validated against registered values

## Future Extensions

- Third-party delegation (proxy to Google/GitHub IdP)
- Persistent token store (vault or SQLite)
- Multi-user consent UI (replace auto-approve)
- Token revocation endpoint (`POST /revoke`)
- Per-tool scope enforcement
- `/.well-known/oauth-protected-resource` (RFC 9728)

---

[← Back to README](../README.md)
