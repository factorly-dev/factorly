# Custom Header Authentication

Some APIs use a custom header (like `X-API-Key`) instead of the standard `Authorization: Bearer` header. Factorly supports this with the `header` auth type.

## Config

```yaml
# .factorly/factorly.yaml
tools:
  weather.forecast:
    type: rest
    description: "Get weather forecast for a city"
    base_url: https://api.weatherapi.com/v1
    method: GET
    path: /forecast.json
    auth:
      type: header
      header: X-API-Key
      value: "{{vault:WEATHER_API_KEY}}"
    parameters:
      - name: q
        in: query
        required: true
      - name: days
        in: query
```

For comparison, bearer auth sends `Authorization: Bearer <key>`, while custom header auth sends `X-API-Key: <key>`. Use `type: header` when the API expects a non-standard header name.

## Usage

```bash
# Store the API key
factorly vault set WEATHER_API_KEY wapi_xxxxxxxxxxxx

# Call the tool
factorly call weather.forecast --q "San Francisco" --days 3
```

## What happens

1. Factorly decrypts `WEATHER_API_KEY` from the vault.
2. It builds the request with the custom header: `X-API-Key: wapi_xxx...`
3. The request goes to `https://api.weatherapi.com/v1/forecast.json?q=San+Francisco&days=3`.
4. Your agent sees the forecast data. The API key never appears in output or logs.

---

[← Back to Examples](README.md)
