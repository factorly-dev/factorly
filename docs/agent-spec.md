# Spec: `factorly agent` — the explainable, auditable agent

> Status: **Draft** — future ideation, not yet implemented.

## Context

Every agent framework lets an LLM call tools. None of them make it easy to explain what happened afterward. `factorly agent` is the agent whose behavior you can fully audit — every tool call logged, every governance decision recorded, every secret kept invisible, every token counted.

**Positioning:** The agent you can show to compliance. "What did it do? What did it try to do? What was it blocked from doing? What did it cost?"

## Usage

```bash
# Run a task
factorly agent "check our GitHub repos and post a summary to Slack"

# See what happened
factorly agent --summary "deploy to staging"
# → prints: 5 tool calls, 1 confirmed, 1 denied, 890 tokens, $0.003

# Review in detail
factorly logs --agent last
```

## Core product features

### 1. Run summary (printed after every run)

```
Agent run complete.

  Tools called:     5
  Confirmed:        1  (deploy.push required approval)
  Denied:           1  (deploy.rollback blocked by shadow policy)
  Rate limited:     0
  Tokens:           1,240 in / 380 out ($0.003)
  Duration:         12.4s
  Output savings:   4.2 KB → 1.8 KB (57%)

  Call log:
    1. github.list_repos         success   215ms
    2. github.get_repo           success   187ms
    3. slack.post_message         success   342ms   [confirmed]
    4. deploy.push               success   8.2s    [confirmed]
    5. deploy.rollback           blocked   —       [denied]
```

### 2. Governance is the feature

The LLM can only call tools in the registry. Shadow policy applies:
- **Deny** blocks dangerous operations before they execute
- **Confirm** pauses and asks the user (interactive mode) or auto-denies (headless)
- **Rate limit** prevents runaway tool calling
- **Loop detection** catches the agent repeating itself
- **Max turns** caps the agent loop (default 50)
- **Max duration** times out the entire run (default 5m)

### 3. Secrets never visible

Tool descriptions go to the LLM (names, parameters, descriptions). Credentials are injected by the proxy at execution time. The LLM sees the tool result, never the token.

### 4. Full audit trail

Every tool call recorded in the JSONL log with:
- Timestamp, tool name, parameters, status
- Shadow action (allowed, denied, confirmed, rate_limited)
- Output savings (original vs compressed bytes)
- Agent ID (links all calls in a single run)
- Token usage per LLM call

## Architecture

### Agent loop

```go
func (h *Harness) Run(ctx context.Context, prompt string) (*RunResult, error) {
    messages := []Message{{Role: "user", Content: prompt}}

    for turn := 0; turn < h.maxTurns; turn++ {
        // Call LLM with tools + message history
        resp, err := h.provider.Chat(ctx, &ChatRequest{
            Model:        h.model,
            SystemPrompt: h.systemPrompt,
            Messages:     messages,
            Tools:        h.toolDefs,
        })

        // Track tokens
        result.TokensIn += resp.Usage.InputTokens
        result.TokensOut += resp.Usage.OutputTokens

        // No tool calls → done
        if len(resp.ToolCalls) == 0 {
            result.FinalText = resp.Text
            return result, nil
        }

        // Execute tool calls through proxy
        messages = append(messages, Message{Role: "assistant", ToolCalls: resp.ToolCalls})
        for _, tc := range resp.ToolCalls {
            toolResult, err := h.proxy.Execute(tc.Name, tc.Params, "agent")
            messages = append(messages, Message{
                Role: "tool", ToolCallID: tc.ID, Content: toolResult.Output,
            })
            result.Calls = append(result.Calls, tc)
        }
    }
    return result, fmt.Errorf("max turns reached")
}
```

### LLM providers

Two built-in HTTP adapters (Anthropic Messages API, OpenAI Chat Completions). Each handles:
- Request formatting (messages, tools, model → provider-specific JSON)
- Response parsing (text, tool_use/function_call blocks → ToolCall structs)
- Auth headers from vault

```yaml
agent:
  provider: anthropic
  providers:
    anthropic:
      model: claude-sonnet-4-20250514
      api_key: "{{vault:ANTHROPIC_API_KEY}}"
    openai:
      model: gpt-4o
      api_key: "{{vault:OPENAI_API_KEY}}"
```

### Confirm handling

- **Interactive mode** (`factorly agent` with TTY): prompt user on stderr, wait for y/n
- **Headless mode** (`--headless` or piped input): auto-deny confirmed tools, log the denial
- Uses the same shadow confirm mechanism as `factorly serve` (MCP elicitation) and `factorly call` (stdin prompt)

### RunResult

```go
type RunResult struct {
    FinalText   string
    Calls       []CallRecord
    TokensIn    int
    TokensOut   int
    Duration    time.Duration
    Denied      int
    Confirmed   int
    RateLimited int
}
```

Printed as the summary. Also logged to JSONL as an "agent_run" entry.

## Config

```yaml
agent:
  provider: anthropic           # default
  max_turns: 50
  max_duration: 5m
  system_prompt: |
    You have access to the tools listed below.
    Use them to accomplish the user's request.
    If a tool call is denied, explain why and suggest alternatives.

  providers:
    anthropic:
      model: claude-sonnet-4-20250514
      api_key: "{{vault:ANTHROPIC_API_KEY}}"
    openai:
      model: gpt-4o
      api_key: "{{vault:OPENAI_API_KEY}}"
```

## MVP scope

**Phase 1:** Anthropic only, one-shot, summary
- `factorly agent "prompt"` with Anthropic Claude
- Tool calling loop with proxy execution
- Summary printed after run
- Shadow governance active

**Phase 2:** OpenAI, interactive
- OpenAI adapter
- Interactive REPL mode
- `--headless` flag for CI

**Phase 3:** Advanced
- Token cost tracking
- Agent run logged as single entry in JSONL
- `factorly logs --agent last` to review runs

---

[← Back to Documentation](README.md)
