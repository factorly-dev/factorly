# Import from OpenAPI

Generate tool definitions from any OpenAPI 3.x spec — local file or remote URL:

```bash
# From a URL
factorly tools import openapi https://petstore3.swagger.io/api/v3/openapi.json --out .factorly/tools/petstore.yaml

# From a local file
factorly tools import openapi ./api-spec.yaml --out .factorly/tools/api.yaml

# With a custom prefix
factorly tools import openapi ./spec.yaml --prefix myapi

# Preview to stdout
factorly tools import openapi ./spec.yaml
```

Each operation becomes a REST tool with method, path, parameters (query/path/header/body), auth, and base URL extracted automatically.

## What gets generated

For each OpenAPI operation:
- **Tool name**: `{prefix}.{operationId}` (or `{prefix}.{method}_{path_slug}` if no operationId)
- **Prefix**: defaults to slugified `info.title`, overridable with `--prefix`
- **Base URL**: from `servers[0].url`
- **Auth**: extracted from `securitySchemes` (bearer, apiKey)
- **Parameters**: from operation parameters with `in` field (query, path, header)
- **Request body**: mapped to a `body` parameter

## Output format

The generated YAML is a flat `map[string]ToolConfig` — drop it into a `tools_dir` or merge into your config:

```yaml
petstore.listPets:
  type: rest
  description: List all pets
  base_url: https://petstore.example.com/v1
  method: GET
  path: /pets
  auth:
    type: bearer
    token: ${PET_STORE_TOKEN}
  parameters:
    - name: limit
      description: How many items to return
      in: query
```

---

[← Back to README](../README.md)
