## [v0.8.1] - 2026-05-04

### Fixed
- Flaky OAuth integration tests by randomizing the server port

## [v0.8.0] - 2026-05-04

### Added
- Integration test for version command with offline CI mode
- Make and npm tool templates with output filters
- Workflows feature - linear sequence of tool calls
  - Workflow tool type with state machine and persisted execution
  - Variable passing between workflow steps
  - Switch case validation for condition and tool fields
  - Expression debug output showing actual conditions evaluated
  - Workflow conditionals with if/switch branching
    - `require:` step condition for halting workflows early
  - Expression evaluator with tokenizer and parser
  - Built-in functions: contains() and jsonpath()
  - Workflow integration tests for variable passing and shadow policies
  - User-facing workflows documentation
  - Workflow specification documentation
  - Conditional workflow example documentation

### Changed
- README updated for problem/solution/install structure
- Result truncation now preserves full output for callers while keeping state files small
- Converted workflow-spec.md to user-facing workflows.md documentation
- Workflow status from "Draft" to "Implemented"
- Documentation structure to include oversight content

### Fixed
- Workflow result truncation affecting both state file and caller output
- Verbose output now shows specific expressions when steps are skipped

## [v0.7.0] - 2026-04-30

### Added
- Parameter validation and coercion engine with type checking and constraint enforcement
- Min/max clamping for integers and numbers with boundary coercion
- String length validation with truncation and rejection rules
- Regex pattern matching and enum allowlist validation
- Boolean normalization for multiple input formats
- JSON validity checking for JSON parameters
- Validation result tracking with modification audit trail
- Cached vault access pattern to eliminate double password prompts
- Global vault fallback messaging for transparency

### Changed
- Parameter validation now runs after defaults and before shadow policy
- ParamConfig and Parameter structs now include validation constraint fields
- Vault operations unified to single process-wide cached instance
- Invalid parameters block execution with "blocked" status logging
- Coerced parameters log "modified" action with original values preserved
- Float-to-int coercion now supported for compatible numeric strings

### Fixed
- Double password prompts eliminated through vault caching
- Zero values now distinguishable from unset constraints using pointer types
- Empty vault creation now uses proper path resolution

### Removed
- Individual vault.Close() calls replaced with shared cached instances

## [v0.6.4] - 2026-04-24

### Added
- Parameter routing type `in: file` for binary file uploads in REST tools
- File streaming directly as request body without loading into memory
- Support for custom Content-Type headers with file uploads
- REST verbose logging with -v flag showing request/response details
- REST timeout configuration support from tool config
- Git tool template
- License to changelog script

### Fixed
- Race condition between CLI and MCP writing to JSONL
- Clear error messages for missing files with path and tool name
- Verify flag documentation and error messages

## [v0.6.3] - 2026-04-22

### Added
- JSONPath filter stage with full JSONPath specification support
- Built-in JSON field extraction without external dependencies
- Support for dot notation, bracket notation, array indexing, wildcards, and recursive descent
- JSONPath expression compilation at config load time
- 13 tests covering various JSONPath scenarios

### Changed
- Release workflows now post changelog links
- Updated filter documentation with JSONPath section

### Fixed
- Parameter substitution when using CLI defaults
- Changelog script functionality

## [v0.6.2] - 2026-04-21

### Added
- Script to generate changelog using factorly anthropic tooling
- Stdin and file reading for CLI parameter values
- Support for reading from stdin using "-" syntax
- Support for reading from files using "@filename" syntax
- Escape syntax "@@" for literal "@" characters
- 10 tests in cmd/factorly/args_test.go for parameter parsing
- CLI reference documentation for "factorly call" command

### Fixed
- Control characters now properly escaped for JSON in parameter submission
- Password bytes handling in vault operations

### Changed
- Optimized vault lookups to search both project and global vaults efficiently

## [v0.6.1] - 2026-04-21

### Added
- Parameter types for REST body serialization: string (default), integer, number, boolean, json
- Body templates with {{param}} placeholders for complex request bodies
- Parameter defaults field - applied before execution for all provider types
- Default values syntax (|default) for template references in vault keys, env vars, and params
- Early parameter validation before vault access
- Tool.ValidateParams() helper method
- anthropic.ask and chatgpt.ask simplified tools with body templates
- --check flag to version command for cache refresh

### Changed
- Multiple body parameters now merge into single JSON object instead of last one winning
- Parameters with defaults are no longer required
- Template references use defaults when keys/vars are missing instead of erroring
- Updated anthropic.yaml and chatgpt.yaml with explicit parameter types
- Tools list now sorted by name
- CI tooling no longer caches test results
- Rewrote hooks-spec.md with implementation-ready specification

### Fixed
- Required parameter validation now occurs before vault opening
- Factorly tools no longer require vault access when listing sub-tools
- Unresolved placeholder check allows defaults to pass through
- Added backslash escape hatch for {{var}} template variables
