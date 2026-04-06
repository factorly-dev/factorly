# CLI Reference

## Commands

```bash
factorly serve                      # start MCP server (stdio)
factorly serve --http :3000         # start MCP server (HTTP at /mcp)
factorly serve --http-token <tok>   # HTTP with Bearer token auth
factorly init                       # create .factorly/factorly.yaml (interactive)
factorly init --out factorly.yaml   # create at custom path
factorly tools                      # list all configured tools
factorly tools list                 # same as above
factorly tools add                  # add a tool (interactive)
factorly tools add --name x --type cli  # add a tool (non-interactive)
factorly tools remove <tool>        # remove a tool from config
factorly tools import openapi <spec>  # generate tools from OpenAPI spec
factorly call <tool> [--param val]  # call a tool
factorly status                     # check all tools are reachable
factorly auth login <provider>      # OAuth login (opens browser)
factorly auth status [provider]     # show OAuth token status
factorly auth logout <provider>     # remove stored OAuth tokens
factorly vault set <key> [value]    # store a secret (prompts if no value)
factorly vault get <key>            # retrieve a secret (raw value to stdout)
factorly vault list                 # list secret names
factorly vault delete <key>         # remove a secret
factorly version                    # print version
```

## Global flags

```bash
-v, --verbose          # print debug info to stderr
-c, --config <path>    # path to factorly.yaml
    --config-dir       # load tools from a directory (no config file needed)
```

## Vault flags

```bash
    --vault-path       # path to vault file (default: ~/.config/factorly/vault.enc)
```

## Environment variables

```bash
FACTORLY_VAULT_PASSWORD   # vault master password (for CI/automation)
FACTORLY_VAULT_PATH       # vault file path override
FACTORLY_NO_LOG           # disable call logging when set
```

---

[← Back to README](../README.md)
