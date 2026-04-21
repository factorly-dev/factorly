# [v0.6.1] - 2026-04-21

## Major New Features

### 1. Parameter Types & REST Body Serialization
- **New `type` field** on parameters: `string` (default), `integer`, `number`, `boolean`, `json`
- Controls serialization when multiple body params merge into JSON
- **Multiple body parameters** now merge into single JSON object instead of last-one-wins
- Proper type handling: integers/numbers unquoted, JSON passed raw, strings quoted

### 2. REST Body Templates
- **New `body` field** on REST tool definitions
- JSON template strings with `{{param}}` placeholder substitution
- Enables complex request bodies without raw JSON construction
- Used in new simplified tools: `anthropic.ask` and `chatgpt.ask`

### 3. Parameter Defaults
- **New `default` field** on parameters
- Applied in proxy before execution (works for CLI, REST, MCP providers)
- Parameters with defaults become optional
- Template reference defaults with `|` syntax: `{{param|fallback}}`

### 4. Template Variable Defaults
Enhanced template system with fallback values:
```yaml
# Vault keys, env vars, external backends, and params all support defaults
vault_key: "{{api_key|sk-fallback}}"
temp_dir: "{{TMPDIR|/tmp}}"
limit: "{{max_limit|100}}"
```

### 5. Early Parameter Validation
- Required parameters validated **before** vault access
- Prevents unnecessary password prompts for missing params
- New `Tool.ValidateParams()` helper method

## Template Updates

### New Simplified Tools
- **`anthropic.ask`**: Simple `--prompt` interface with model/max_tokens defaults
- **`chatgpt.ask`**: Simple `--prompt` interface with model default
- Both `.messages` tools now have explicit parameter types

## Developer Experience Improvements

### 1. Enhanced CLI Features
- **Escape hatch**: `\{{var}}` for literal `{{var}}` output
- **Version checking**: `factorly version --check` for cache refresh
- **Better error handling**: Tools list no longer requires vault for bootstrap provider

### 2. Template Management
- Templates sorted by name for better organization
- New anthropic and chatgpt templates added
- Updated template documentation

### 3. Build & CI Improvements
- Updated Makefile for changelog generation
- CI tooling improvements with better caching strategy

## Documentation Updates

### Enhanced Reference Documentation
- **config-reference.md**: Parameter type table, body template examples, YAML schema updates
- **templates.md**: Updated template list with new additions
- **cli-reference.md**: Default value syntax and examples
- **hooks-spec.md**: Complete rewrite with implementation-ready specs for Claude Code, Cursor, Codex, and Gemini

## Technical Implementation Details

### Template Resolution Layers
1. **Vault resolver** (`resolver.go`): Handles defaults for missing backend keys
2. **CLI param substitution** (`cli.go`): Manages parameter defaults and substitution
3. **Validation flow**: Early validation → vault access → execution

### Error Handling Improvements
- Graceful fallback with default values
- Better error messages for missing required parameters
- Reduced unnecessary vault access attempts
