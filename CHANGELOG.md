## [v0.10.1] - 2026-05-13

### Fixed
- Workflow provider is now lazy-created on first workflow registration, matching CLI and REST provider behavior
- Proxy persists the `OnWorkflowStep` callback and re-applies it when a workflow provider is registered, ensuring step events reach the activity feed regardless of provider creation order

## [v0.10.0] - 2026-05-13

This release includes sharable templates, called blueprints:

### Added
- Bundled blueprint catalog UI with browse, detail, and one-click install routes (`/blueprints/browse`, `/blueprints/browse/{name}`, `/blueprints/browse/{name}/install`)
- `factorly blueprint install <name>` resolves bare names against the bundled catalog
- Blueprint header fields: `display_name`, `category`, `auth_type`, `auth_guide`
- Paste-YAML install path in the import modal and API (`content` field on preview/install endpoints)
- `InstallResult.AlreadyInstalled` flag for structured already-installed UX in dry-run/preview mode
- `internal/blueprints` package with full install pipeline: source resolution, fetch, parse, validate, write, dry-run, conflict detection, requires validation, uninstall, list
- `factorly blueprint install/uninstall/list` CLI commands
- In-place edit and delete of tools and OAuth providers that live inside blueprint files
- `ValidateReferences` for cross-reference checks on workflow steps and requires entries
- Blueprint header fields: `name`, `version`, `description`, `author`, `homepage`, `license`, `requires`
- Loose YAML loader accepts full structured `Config` documents alongside legacy flat tool-map files; installed packs scanned from `.factorly/blueprints/`
- `reloadConfig()` helper extracted and shared across reload, install, and uninstall handlers
- Duplicate button for workflows
- Bundled blueprints test suite
- Example Gmail blueprint (`examples/blueprints/gmail.yaml`)
- `storeInVault` helper moved to `vault_cmd.go` for shared use
- External-link Lucide icon helper in `server.go`

### Changed
- Renamed package and all references: `packs` → `blueprints` (Go package, CLI verbs, HTTP routes, on-disk layout `.factorly/blueprints/`, UI labels, templates, JS)
- `SaveTool` and `DeleteTool` now edit blueprint files in place instead of writing shadow copies or leaving stale entries
- `SaveOAuthProvider` and `DeleteOAuthProvider` now edit blueprint files in place
- Already-installed check moved ahead of conflict walk to produce a clear "already installed" message instead of a generic conflict error
- `factorly init` prompt updated to reference `factorly blueprint install` instead of templates
- Empty-state copy on tools and blueprints pages updated to reference the blueprint catalog
- Blueprint list page header styled to match the Auth page
- Icons made more consistent between views

### Removed
- `internal/templates/` package and all per-template Go wrappers and YAML files
- `cmd/factorly/templates_cmd.go` and the `factorly tools import templates` subcommand
- `internal/ui/handlers_templates.go` and associated `/templates*` routes
- Templates catalog HTML pages (`templates.html`, `template_detail.html`)
- Dead `formatJSONHTML` function in `handlers_tools.go`
- Bundled example blueprints from the main repo in favor of a separate community repo

## [v0.9.1] - 2026-05-11

### Added
- `StepEvent` type and `OnStep` callback on `WorkflowProvider`, fired at every step transition
- `Proxy.SetOnWorkflowStep` wires workflow provider callbacks to the activity broadcaster
- `ActivityBroadcaster` tagged event wrapper supporting both `call` and `workflow_step` SSE event types
- Live workflow step streaming in the "Try It" panel with status icon, index/total, tool, duration, and errors
- `ConfirmBroker.Request` notifies SSE subscribers on both pending creation and resolution via extracted `notifyLocked`
- Toast notification in `confirm.js` when a POST returns 404 ("Already resolved by another tab")

### Changed
- `activity.js` switched from `onmessage` to `addEventListener` for `call` and `workflow_step` event types
- Step events now render nested inside their parent workflow row in the activity drawer
- `confirm.js` IIFE gated behind `window._confirmInit`; `sse`, `pollTimer`, and `reconnectTimer` hoisted onto `window`
- Polling in `confirm.js` stops when SSE is open and only restarts while SSE is in `CLOSED` state, eliminating constant heartbeat polling
- `workflow_edit.html` reads active workflow name from `window._currentWorkflow` instead of a captured closure
- `htmx:beforeRequest`, `htmx:afterSwap`, and `workflow_step` SSE listeners in `workflow_edit.html` installed exactly once, guarded by `window._workflowRunInit`
- `#run-steps` and `#run-steps-list` resolved from live DOM on each event instead of script-load references

### Fixed
- `hx-boost` navigation no longer leaks poll intervals, `EventSource` instances, or duplicate `document.body` listeners across navigations
- Resolving a confirm prompt on one tab now dismisses the modal on all connected tabs
- Workflows header title corrected when workflows exist

## [v0.9.0] - 2026-05-11

### Added

- **Web UI** (`factorly ui`)
  - Localhost web interface for browsing, testing, and managing tools
  - Tool CRUD with full config editing: parameters, oversight, output filters, advanced
  - Postman-style response inspector with collapsible JSON tree, Raw/Meta tabs, copy button, expand/collapse all
  - Persistent sidebar with type badges, live filter, and dot-prefix grouping with localStorage persistence
  - Visual workflow editor with collapsible steps, input parameters, oversight, and output filters
  - Server-rendered workflow step addition and tool-aware param auto-populate via htmx
  - OpenAPI import with preview, filtering, and selective tool import
  - OAuth provider CRUD with auth config editing (Bearer/Custom header/OAuth)
  - Vault management: project and global vault sections, add/replace/delete with scope awareness
  - History page with last 100 audit log entries and filtering
  - Live activity feed via SSE in a slide-out drawer, accessible from any page
  - Shadow confirm prompts via browser modal — races MCP elicitation and browser, first response wins
  - SSE subscriber broadcast pattern with 2s polling fallback and exponential reconnect backoff
  - 60-second auto-deny timeout on unresponded confirm prompts
  - Config reload button with live diff output
  - Runtime template install with immediate tool registration
  - Tool duplication, inline rename/description editing, hidden toggle
  - Lucide SVG icons rendered inline via template function
  - `--no-launch` flag to skip auto-opening browser
  - Non-localhost binding warning for dev-only use

- **Built-in tools**
  - Native builtin provider — in-process execution via Go stdlib (no subprocess for read/write/fetch)
  - File operations scoped to project directory with path traversal prevention
  - Platform-aware shell (`sh -c` on Unix, `cmd /C` on Windows)
  - Per-tool disable via `disabled_builtins` config field

- **Expressions and resolvers**
  - `today()`, `left()`, `right()`, `find()`, `cut()`, `concat()` expression functions
  - Value expressions (`{{expr:...}}`) for workflow step params
  - `expr` registered as a resolver backend alongside `vault:` and `op:`

- **Tools and config**
  - `text` parameter type for multi-line textarea rendering in the UI
  - `body_type` field (json/form/raw) for REST tools
  - `param in:` field for explicit parameter location (query/path/body/header)
  - Audit log `vault_keys` field tracking which vault references each tool accesses

### Changed

- **Web UI**
  - Activity feed promoted from standalone page to persistent slide-out drawer
  - Templates removed from nav, accessible via "Browse Templates" button in empty state
  - Workflows promoted to top-level navigation section
  - Sidebar tool list replaced with grouped `<details>` sections by dot-prefix
  - Tool save form uses htmx inline confirmation instead of full-page redirect
  - Navigation order standardized: Tools, Workflows, Auth, Vault, History
  - Factorly logo added to layout header and favicon
  - Inline JS extracted to static files (`activity.js`, `confirm.js`, `workflow.js`)
  - Inline HTML generation in handlers replaced with template partials

- **Server and CLI**
  - `factorly ui` and `factorly serve` now bind to `127.0.0.1` by default
  - `factorly serve --http` replaced with `--host`/`--port` flags (legacy flag preserved hidden)

- **Internals**
  - `registerProvider` refactored from 116-line switch into separate helper functions
  - Vault ref resolution unified through shared `vault.Resolver` (supports inline refs, multiple backends, defaults)
  - REST verbose logging now includes all headers, body content, and redacts sensitive headers
  - `refPattern` regex widened to support expressions with parens/commas/quotes
  - OpenAPI operationId `/` replaced with `.` and spaces with `_` for valid tool names
  - Tool name sanitization applied on creation
  - Go and dependencies bumped to 1.25.2

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
