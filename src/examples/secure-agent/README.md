# Secure Agent Example

This example shows how Factorly keeps secrets out of your agent's context.

## The Problem

Without Factorly, your agent needs direct access to every API key:

```python
# Your agent code — secrets everywhere
import os
import requests

github_token = os.environ["GITHUB_TOKEN"]     # agent sees this
slack_token = os.environ["SLACK_BOT_TOKEN"]    # agent sees this
stripe_key = os.environ["STRIPE_SECRET_KEY"]   # agent sees this

# Agent makes requests with raw credentials
requests.get("https://api.github.com/repos/...",
    headers={"Authorization": f"Bearer {github_token}"})
```

The agent has full access to every credential. If the agent is compromised, prompt-injected, or has a bug — your secrets are exposed.

## With Factorly

Secrets live in Factorly's config, resolved from environment variables at boot. The agent only sees tool names and parameters:

```
┌─────────────────────────────────────────────────┐
│  Your Agent                                     │
│                                                 │
│  "List repos for octocat"                       │
│  → factorly call github.repos --username octocat│
│                                                 │
│  The agent NEVER sees:                          │
│  - GITHUB_TOKEN                                 │
│  - SLACK_BOT_TOKEN                              │
│  - STRIPE_SECRET_KEY                            │
│                                                 │
│  It only knows:                                 │
│  - tool names (github.repos, slack.post, ...)   │
│  - parameter names (username, channel, ...)     │
│  - response data                                │
└─────────────┬───────────────────────────────────┘
              │
              │  factorly call github.repos --username octocat
              │  (no secrets in the command)
              │
┌─────────────▼───────────────────────────────────┐
│  Factorly                                       │
│                                                 │
│  1. Looks up tool "github.repos"                │
│  2. Injects Authorization: Bearer ghp_xxxxx     │
│  3. Makes GET api.github.com/users/octocat/repos│
│  4. Returns response data to agent              │
│  5. Logs the call                               │
│                                                 │
│  Secrets: resolved from env at boot,            │
│  never exposed to the agent                     │
└─────────────────────────────────────────────────┘
```

## Setup

```bash
# Set your secrets in the environment (or .env file)
export GITHUB_TOKEN=ghp_xxxxxxxxxxxxxxxxxxxx
export SLACK_BOT_TOKEN=xoxb-xxxxxxxxxxxx
export STRIPE_SECRET_KEY=sk_live_xxxxxxxxxxxx

# See what tools are available
factorly tools -c examples/secure-agent/factorly.yaml
```

```
NAME                  TYPE  DESCRIPTION                       PARAMETERS
github.create_issue   rest  Create an issue in a repository   owner, repo, body
github.issues         rest  List issues for a repository      owner, repo, state
github.repos          rest  List repositories for a user      username, per_page, sort
slack.post            rest  Post a message to a Slack channel body
stripe.charges        rest  List charges for a customer       customer, limit
stripe.customers      rest  List Stripe customers             limit, email
web.fetch             cli   Fetch a webpage                   url
```

## Usage

The agent calls tools by name. No secrets in any command:

```bash
# List repos — agent says "username", never sees the token
factorly call github.repos --username octocat --per_page 3

# List open issues
factorly call github.issues --owner octocat --repo Hello-World --state open

# Post to Slack — agent says "channel" and "text", never sees the bot token
factorly call slack.post --body '{"channel":"#general","text":"Deploy complete"}'

# Look up a customer — agent says "email", never sees the Stripe key
factorly call stripe.customers --email "customer@example.com"
```

## What the Agent Sees vs. What Factorly Sends

```
Agent runs:
  factorly call github.repos --username octocat --per_page 5

Factorly sends:
  GET https://api.github.com/users/octocat/repos?per_page=5
  Authorization: Bearer ghp_xxxxxxxxxxxxxxxxxxxx  ← injected by Factorly
  Accept: application/vnd.github.v3+json          ← from static headers

Agent receives:
  [{"name":"Hello-World","full_name":"octocat/Hello-World",...}, ...]
  (just the data — no tokens, no auth headers, no credentials)
```

## Why This Matters

- **Prompt injection defense** — even if an attacker manipulates the agent's prompt, they can't extract credentials because the agent never has them
- **Least privilege** — the agent can only call the tools you've configured, with the parameters you've defined
- **Key rotation** — rotate a secret in your environment, restart Factorly. No agent code changes.
- **Audit trail** — every call is logged with tool name, parameters, and response. You know exactly what your agent did.
- **Team security** — share tool configs without sharing secrets. Each developer/environment has its own `.env`.

## File Structure

```
secure-agent/
├── factorly.yaml          # main config, points to tools/
├── tools/
│   ├── github.yaml        # GitHub API tools (3 tools)
│   ├── slack.yaml         # Slack API tools (1 tool)
│   └── stripe.yaml        # Stripe API tools (2 tools)
└── README.md
```

Secrets referenced: `{{GITHUB_TOKEN}}`, `{{SLACK_BOT_TOKEN}}`, `{{STRIPE_SECRET_KEY}}` — all resolved from environment, never written to config files, never exposed to the agent.
