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
