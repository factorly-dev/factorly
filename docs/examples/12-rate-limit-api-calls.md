# Rate Limit API Calls

Prevent runaway agents from burning through expensive API quotas. Shadow rate limits use a token bucket algorithm for smooth throttling.

## Config

```yaml
# .factorly/tools/openai.yaml
tools:
  openai.completion:
    type: rest
    base_url: https://api.openai.com/v1
    method: POST
    path: /chat/completions
    auth:
      type: bearer
      token: "{{vault:OPENAI_API_KEY}}"
    parameters:
      - name: model
        in: body
        required: true
      - name: messages
        in: body
        required: true
    shadow:
      rate_limit: 100/hour

  maps.geocode:
    type: rest
    base_url: https://maps.googleapis.com/maps/api
    method: GET
    path: /geocode/json
    parameters:
      - name: address
        in: query
        required: true
      - name: key
        in: query
    shadow:
      rate_limit: 10/min
```

## Usage

```bash
# First 100 calls in the hour work normally
factorly call openai.completion --model gpt-4 --messages '[{"role":"user","content":"hello"}]'

# Call 101 is rejected
factorly call openai.completion --model gpt-4 --messages '[{"role":"user","content":"hello again"}]'
# Error: rate limit exceeded for "openai.completion" (100/hour) — retry in 42s
```

## What happens

1. Each call consumes one token from the bucket.
2. Tokens refill at a steady rate (100/hour = ~1 token every 36 seconds).
3. The bucket starts full, so initial bursts are allowed up to the limit.
4. Unlike fixed-window counters, there are no boundary bursts — you can't get 200 calls by timing requests at the window edge.
5. When the bucket is empty, the call is rejected with a clear error showing how long until the next token is available.
6. Rate limit state persists across invocations. Project configs store buckets at `<project>/.factorly/ratelimit.json`; the global config falls back to `~/.config/factorly/ratelimit.json`. Delete that file to reset.

---

[← Back to Examples](README.md)
