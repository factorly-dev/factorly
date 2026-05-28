## [v0.17.3] - 2026-05-28

### Added
- "Used by N tool(s)" indigo and "used by N auth(s)" green badge pills on vault and store list entries
- `toolReferenceCounts` and `oauthProviderReferenceCounts` helpers to count references via YAML rendering and regex matching
- Copy buttons on history Output/Error/Params, try-panel blocked response, and workflow run result containers
- Shared copy button partial and global `copyText` helper in `static/copy.js`

### Changed
- Vault and store HTMX fragment renderers (`renderVaultSections`, `renderStoreKeys`) now include reference count badges to stay in sync after mutations
- Removed 200-character truncation on history Output field so copied content matches full payload

## [v0.17.2] - 2026-05-26

### Added
- Pagination for `/history` endpoint, slicing filtered results into 50-group pages
- `/history/more` endpoint returning appended rows with an out-of-band swap updating the Load-more button
- Full audit log filtering before windowing so filters like "show errors" match across entire log, not just the current page
- Cursor-based pagination using lead audit hash, falling back to `wf:<runID>` for workflow leads whose parent has scrolled out
- Workflow groups preserved intact across page boundaries

## [v0.17.1]

### Fixed

- **Linear blueprint sent malformed bodies.** Every `linear.*` tool POSTed to `/graphql` but the body was the flat `in: body` param map (e.g. `{"id":"WID-5"}`), which Linear's GraphQL server rejected with `GraphQL operations must contain a non-empty 'query' or a 'persistedQuery' extension`. Each tool now uses a `body:` template that wraps params in a proper GraphQL envelope (`{"query":"...","variables":{...}}`) with a per-tool baked query, so the agent supplies flat params and the GraphQL marshaling is invisible to it.
- **Linear authorization header used the wrong format.** `auth.type: bearer` adds a `Bearer ` prefix, but Linear's personal API keys are sent as-is (without `Bearer `). Switched to `auth.type: header` with the raw key as the `Authorization` value. OAuth tokens still use Bearer; this fix matches what the blueprint's own `auth_guide` directs users at (personal API keys).

### Changed

- Linear blueprint bumped to v1.1.0. Tool surface refined: `linear.list_issues` renamed to `linear.list_my_issues` (more accurate — it lists the API-key owner's issues), `linear.search` renamed to `linear.search_issues` (scoped). New `linear.list_workflow_states` (needed to fill `stateId` on `linear.update_issue`). New `linear.graphql` escape-hatch tool for queries the curated set doesn't cover. `linear.update_issue`'s params expanded to include `description`, `stateId`, `assigneeId`, `priority` since those are the common update targets.

## [v0.17.0] - 2026-05-24

### Added
- `IsSafeBackendName`, `Resolver.ResolveCallerParam`, and `RedactToTemplate` to vault package
- `ParamConfig.HydrateVaultRefs` opt-in flag (default false) to gate secret backend hydration for caller-supplied params
- Recursion test for `factorly.code` covering nested interpreter SDK injection and shared store side-effects
- Dashboard top-tools links to tool pages

### Changed
- Proxy resolver loop now gates secret backends behind `HydrateVaultRefs`; audit log `params` field redacted to `{{vault:K}}` template at all four log sites when a secret backend resolves
- CLI handler drops duplicate eager pre-resolution pass
- `TestCallParamWithVaultRef` rewritten to assert new gated behavior
- Caller-supplied `{{vault:KEY}}` references require explicit opt-in to hydrate into outbound call bodies or audit logs

## [v0.16.0] - 2026-05-24

### Added

- **`factorly.Store` SDK handle in `factorly.code` scripts.** Scripts can now read and write the workspace store directly. Methods: `Get(key)` (returns `factorly.ErrStoreNotFound` on miss so scripts can `errors.Is` to branch on absence), `Set(key, value)` (default TTL), `SetWithTTL(key, value, time.Duration)` (`0` = never expire), `Delete(key)` (idempotent), `List()` (sorted keys, cascaded). Reads cascade workspace → project; writes target the active tier — same rules as the CLI's `factorly store` subcommand. Audit logging routes through the existing `logStoreOp` path so `factorly store history <key>` surfaces script-side writes alongside CLI writes. Foundation work for the upcoming hooks v0 (https://trello.com/c/rLWKvaai).

### Removed

- **`factorly.store.{get,save,search,list,delete}` agent-facing builtin tools.** These were premature — they bloated the agent's tool list with five entries that mirrored what one SDK handle (`factorly.Store`, above) does more ergonomically. Scripts now use `factorly.Store.Get` / `Set` / etc. directly; the CLI's `factorly store ...` subcommand remains for human use. If MCP-direct store access (without going through `factorly.code`) turns out to be a real need, we can re-add a thinner surface. Files / handlers removed from `cmd/factorly/main.go`: `makeStore{Save,Get,Search,List,Delete}Handler` plus the `joinNewline` helper that only those handlers used. Five description constants removed from `internal/builtins/builtins.go`. Three integration tests removed (`TestStoreBuiltinGetReturnsValue`, `TestStoreBuiltinGetMissingKey`, `TestStoreBuiltinsRoundTripViaCode`) — coverage preserved by the new `TestStoreSDKHandle*` set.

### Changed

- `factorly tools` (and `factorly tools list`) now show only the first line of each tool's description. Builtins ship with multi-paragraph agent-facing prose (`factorly.store.save`, `factorly.code`, etc.), which used to splatter across the tabular listing and make it unreadable. The full description is unchanged everywhere else — `factorly tools show <name>` still dumps the raw YAML.

### Fixed

- Vault unlock now re-prompts on wrong password (up to 3 attempts) instead of failing silently and surfacing the error as a confusing "couldn't read vault item" at use time. Each wrong attempt prints `Incorrect password, try again (N attempts left).` to stderr; the third failed attempt exits non-zero with `vault unlock failed after 3 attempts`. Non-interactive sources (`FACTORLY_VAULT_PASSWORD` / key file) still fail on first wrong password — retrying a static value can't change the answer — with a more specific message (`<source> did not unlock <path>`) so users know which source to fix.
- Vault unlock in the **UI** now distinguishes wrong password from other failures: a wrong password renders `Incorrect password — try again.` on the unlock form (htmx swap means the user can just retype and resubmit, no page reload); other errors (corrupt file, I/O, misconfigured manager) surface verbatim as `Failed to open vault: <reason>`. Previously every error was conflated as "incorrect password," which masked the real problem. Routed through a single `unlockErrorMessage` classifier used by `/vault/unlock`, `/vault/unlock-all`, and the workspace unlock dialog so the wording is consistent everywhere the UI prompts for a password.
- `vault.ErrWrongPassword` sentinel introduced so the CLI prompt loop, UI unlock dialog, and any future caller can branch on auth failure vs. other open errors (corrupt file, I/O) — retrying makes sense for one but not the other.

## [v0.15.1] - 2026-05-23

### Added

- **`factorly.store.get` builtin** — agents can now read back values they (or you) have stored. Previously the agent-facing store surface had `save` / `list` / `search` / `delete` but no way to retrieve a single value, leaving the "I researched these URLs already" use case half-implemented. The new builtin mirrors the CLI's `factorly store get`: cascade reads (workspace → project), refresh-on-read to keep frequently-touched keys alive, missing keys return as a non-zero result so agents can branch on absence. Existing docstrings for List/Search already referenced "pair with Get" — this closes the gap.
- Dashboard quick-start tiles expanded:
  - **Fresh install** now also nudges "Stash credentials in the vault" so users discover where API tokens go before they start running tools.
  - **Has-tools-no-calls** branch grew from 2 tiles to 6: Try a built-in, Add credentials to the vault, Connect an OAuth provider, Explore more blueprints, Set up a workspace, Compose a workflow. Pointing at `/vault/new`, `/auth/new`, `/blueprints/browse`, `/workspaces/new` respectively.

### Changed

- Dashboard quick-start tiles reordered for gradual enhancement (so scrolling the list top-to-bottom is itself a "what to do next" path):
  - **Fresh install** (no tools yet): Browse blueprints → Import an OpenAPI spec → Create a tool by hand → Stash credentials in the vault. Easiest path first, support layer last.
  - **Has-tools-no-calls**: Try a built-in → Add credentials to the vault → Connect an OAuth provider → Compose a workflow → Set up a workspace → Explore more blueprints. Fire something immediately, fix the most common failure (missing creds), then compose, then scale, then expand.
- Tests pin the tile order via index walking so a future reshuffle has to acknowledge the contract.

### Fixed

- Dashboard quick-start tiles now actually fall back to the fresh-install set when the user has no hand-rolled tools yet. The check was treating builtins (`factorly.fetch`, `factorly.code`, `factorly.store.*`, etc.) as user tools, which meant every real install landed in the has-tools-no-calls branch and the fresh-install branch was unreachable in production. `quickStartTiles` now filters `Type == "builtin"` (and workflows) out of the inventory before deciding.


## [v0.15.0] - 2026-05-23

New `/dashboard` landing page: always-populated, status strip across the top, live activity feed with workflow-run coalescing on the left, last-24h rollups on the right (top tools + oversight breakdown), quick-start tiles when there's no audit log yet.

### Added

**`/dashboard`** — the new front door.

- Status strip: tool counts by type (`cli`, `rest`, `mcp`, `code`, `builtin`, `workflow`), installed-vs-available blueprints, workspace count, vault tiers opened, store tiers present
- Live feed (SSE) with workflow-run coalescing: events carrying `workflow_run_id` group under one expandable parent row that updates as steps fire. Standalone calls render inline. 30-row cap; older rows fall off the tail
- Live feed is seeded on page load with up to 30 of the most recent calls from the last 24h (newest-first, same coalescing rules as the live JS path) so the panel doesn't look dead on first paint. Seeded workflow rows carry `data-run-id` so the live JS rehydrates its coalescing map and merges incoming step events into the existing parent rather than synthesizing duplicates
- Each feed row (seed + live) expands inline on click to show its full per-row detail (params, status, duration, error, output, plus Replay / Edit & Replay buttons). Reuses `/history`'s `history_entry_body` partial verbatim, lazy-loaded via `GET /history/{hash}/detail` on first expand. Workflow step rows get the same affordance once their audit hash arrives via the matching `call` event
- `proxy.CallEvent` carries `Hash` (the audit-chain identifier of the logged entry); `/activity/stream` SSE payload includes it. Lets live rows attach the hx-get for detail lookup as soon as they're rendered

### Changed

- Dashboard layout: rollups moved to the left column (Oversight on top, Top tools, Top vault items beneath); the Live activity feed takes the full right column with a taller `max-h-[640px]` viewport. Quick-start tiles (fresh install) sit in the left column too.
- Status strip removed from the top of the dashboard — the install-state counts (tool-types, blueprint inventory, workspace count, vault/store tier presence) weren't surfacing decisions, and the same data is more useful on its respective page. `buildStatusStrip`, `countVaultTiers`, `countStoreTiers`, the `statusStrip` / `typeCount` types, and the `Status` field on `dashboardData` are all gone.

### Added (dashboard, continued)

- **Top vault items** panel (under Top tools, left column). Bar list of the 10 most-referenced vault keys over the last 24h, counted from each call's `logger.Entry.VaultKeys` slice (the tool's declared `vault_keys` at log time). Header link drills into `/vault`. Emerald bars so it visually distinguishes from the indigo Top tools bars.
- "manage →" link on the Top tools panel header (mirrors the Top vault items pattern) — drills into `/tools`.
- Oversight tiles deep-link to `/history?status={success|error|blocked}` so you can click straight from the count into the matching rows.

### Fixed

- Dashboard live feed no longer renders a duplicate standalone row for the outer workflow call when a workflow finishes. The outer call's audit entry doesn't carry `workflow_run_id` (the context value is only set inside the workflow provider, after the proxy has begun logging the outer call), so the live JS used to fall through to the standalone branch and insert a duplicate next to the coalesced parent. The JS now matches the incoming tool name against active coalesced buckets and treats the event as the parent finalizer when found — locking the parent's status + duration onto the existing row instead. (The seed render path already suppressed this duplicate via `groupHistoryEntries`'s real-parent detection; only the live JS was affected.)
- Top tools panel: bar list of the 10 most-fired tools over the last 24h. Leader pegs at 100%; others scale to it
- Oversight panel: success / error / blocked stat row over the last 24h. Template guards against div-by-zero — pathological all-zero data renders "0 total" rather than "NaN%"
- Quick-start tiles when the audit log is empty: branches on install state (no tools → browse / import / create; has-tools-no-calls → "try a builtin" / "compose a workflow")
- `activity` lucide icon added to the template icon map for the Live activity panel header

**Data plumbing for live coalescing**

- `proxy.CallEvent` carries `WorkflowRunID` and `WorkflowName`; copied through `emitCallEvent` from the matching `logger.Entry` fields
- `/activity/stream` SSE payload includes `workflow_run_id` and `workflow_name` on `call` events. Dashboard JS keys its coalescing `Map` by `run_id`

**Tests**

- `topTools` ordering / tie-breaking / NaN guard, `oversightBreakdown` counts, `filterAfter` windowing
- Fresh-install handler render shows quick-start tiles and hides rollup panels
- Active-use handler render shows rollups (with audit log via `FACTORLY_LOG_PATH`) and hides quick-start
- Status-strip pill counts for `cli`, `rest`, and `workflow` tool types

### Changed

- `history_entry_body` template moved from `templates/history.html` into `templates/partials/history_entry_body.html` so the dashboard's inline-expand handler can render it via `renderPartial`. `/history`'s existing rendering is unchanged (the partial is still loaded by every page template)
- Default landing page is now `/dashboard`. The `/` redirect target moved from `/tools` to `/dashboard`. The factorly logo and wordmark in the top nav point at `/dashboard` too
- Top nav order: `Dashboard` · Tools · Workflows · Vault · Auth · Store · Blueprints · History
- `/activity` legacy route now redirects to `/dashboard` (was `/tools`)

### Removed

- Right-side activity drawer (markup + CSS + toggle button) — replaced by the dashboard's live feed
- `internal/ui/static/activity.js` — superseded by `static/sse-init.js` (~20 lines) which exists solely to keep `window._activitySSE` alive so `workflow_edit.html`'s "Try It" step-progress poller has something to attach to

## [v0.14.1] - 2026-05-22

UI contrast pass: bump weak grays so secondary text actually reads, and make destructive/action links look like buttons instead of faded suggestions.

### Changed

- `text-gray-300` / `text-gray-400` → `text-gray-600` for informational text on light backgrounds: empty states, helper paragraphs, timestamps, counts, table cells, the masked-secret dots in `/vault`
- Form section labels (`text-[10px] ... uppercase`) bumped from `-400` to `-600` across tool / workflow / auth / store / vault edit pages
- Tiny inline annotations (`(default: false)`, type hints, key-count badges) settled at `text-gray-500`
- Action links bumped to the `-600`/`-700` family so they meet WCAG AA on white: store/vault `view` / `replace` / `delete`, auth `logout` / `edit` / `delete` / `refresh`, blueprint `View YAML` / `Uninstall`, tool edit `Duplicate` / `Delete`, every `+ env var` / `+ param` add-button
- Required-field `*` markers bumped from `text-red-400` to `text-red-600`
- `bg-gray-100 text-gray-400` skipped-step pill kept (decorative status, not informational)
- Dark-panel response viewer in `try_result.html` left untouched (`-300`/`-400` are correct on `bg-gray-800`/`bg-gray-900`)
- `CLAUDE.md`: short rule-of-thumb section so future templates don't drift back

## [v0.14.0] - 2026-05-22

Replay any past call from `/history` or the CLI; workflow runs coalesce into one expandable row.

### Added

**Replay** — re-fire any past call through the same proxy as a fresh call. Vault refs re-resolve, shadow rules re-apply, audit log records the new entry with `replayed_from` linking back to the source.

- `↻ Replay` button on every `/history` row, plus a separate `↻ Replay workflow` action on coalesced workflow runs
- `✎ Edit & Replay` link opens the tool's Try-It form pre-populated with the recorded params (`/tools/{name}?prefill={hash}`)
- `POST /history/{hash}/replay` handler; `applyPrefill` helper for the Edit & Replay path
- `factorly logs replay <hash>` CLI command — full or ≥4-char prefix
- `--last [--last-n N]` to pick the Nth most recent call; `--last-of <tool>` to pick the last call of a specific tool
- `--param key=value` overrides one recorded param (repeatable); `--show` dry-runs without firing
- Selection flags are mutually exclusive (errors loudly)

**Workflow run coalescing in `/history`** — every step entry of a run now gathers under a single expandable row labeled with the workflow's name.

- Expandable parent row with a `git-compare` icon badge showing step count
- Child steps render in execution order (step 1 → N) when expanded; filters keep the parent visible when only a step matches
- Per-step Replay and Edit & Replay buttons (step replays fire as plain standalone calls)
- Workflow-level Replay re-fires the entire workflow via the suppressed parent entry's audit hash

**Data shape additions** (drive both UI coalescing and CLI replay):

- `logger.Entry`: new `WorkflowRunID`, `WorkflowName`, and `ReplayedFrom` fields
- `provider.WorkflowRunIDKey` + `WorkflowNameKey` context keys (workflow provider stamps both before each child-step dispatch; proxy reads them)
- `proxy.ReplayedFromKey` context key (replay handlers set it; proxy stamps the audit entry)

**Shared plumbing in `internal/logger`** (used by UI and CLI):

- `FindByHash(path, hash)` — exact or prefix match (≥4 chars), errors on ambiguity
- `MostRecentMatch(path, n, predicate)` — Nth-most-recent entry matching a filter; backs the CLI's `--last` / `--last-of`

**Tests**

- Unit: `logger.FindByHash` (exact/prefix/ambiguous/missing/too-short), `logger.MostRecentMatch` (newest/filtered/Nth/overflow/missing-file), workflow context-stamping in both proxy and provider, history coalescing scenarios, replay eligibility
- Integration: workflow run-ID/name audit stamping end-to-end via the real binary; `logs replay` happy path, `--last`, `--param`, `--show`, mutually-exclusive selection

### Changed

- `isReplayable` no longer excludes `Interface=="workflow"` — step-level replays are now allowed (re-fired as standalone tool calls, no parent workflow context)
- UI's `findAuditEntryByHash` is now a thin wrapper around `logger.FindByHash`
- Homebrew templates and brew git push updated

### Fixed

- Trello blueprint JSON body for POST/PUT requests

## [v0.13.0] - 2026-05-21

### Added

- **Homebrew distribution.** Install with `brew install factorly-dev/tap/factorly`. Formula lives at [factorly-dev/homebrew-tap](https://github.com/factorly-dev/homebrew-tap); released versions push automatically.
- **`linux/arm64` release binary** — previously missing. Anyone on Raspberry Pi, aarch64 cloud VMs, or M-series Macs running Linux in Docker can now install via any channel (brew, npm, pip).
- **`checksums.txt`** attached to every GitHub Release, with sha256s for all 5 release binaries.

### Changed

- `make release` now produces 5 binaries (added linux/arm64) and a `checksums.txt` alongside them.

## [v0.12.1] - 2026-05-21

### Changed

Two tools, `factorly.read_file` and `factorly.write_file` have been renamed for taste to `factorly.file.read` and `factorly.file.write`.

## [v0.12.0] - 2026-05-21

Agent-writable workspace state primitive, plus a UI consistency pass on the secrets/state pages.

### Added

**Store** — fourth workspace data primitive (alongside vault, env, auth)

CLI:
- `factorly store {set,get,list,search,delete,history}` — mirrors `factorly vault`'s shape minus password/encryption machinery
- `--global` persistent flag pins operations to `~/.config/factorly/store.db` even from inside a project (precedence: `--global` > `--workspace` > project default > global fallback)
- `--ttl` on `set` accepts durations (`7d`, `24h`, `30m`, `0` = never-expire); default is 30d with refresh-on-read so frequently-touched entries stay alive

Agent surface:
- `factorly.store.{save,search,list,delete}` built-ins for `factorly.code` scripts and MCP clients
- `{{store:KEY}}` template reference syntax — store-resolved values flow into tool configs the same way `{{vault:KEY}}` and `{{env:VAR}}` do

Storage:
- Per-scope bbolt files at `.factorly/store.db`, `.factorly/workspaces/<name>/store.db`, `~/.config/factorly/store.db` — pure Go (no CGO), mmap-backed, ACID
- Workspace cascade for reads (workspace → project); writes target exactly one tier
- Per-operation open/close lifecycle: every CLI command, builtin handler, UI request, and `{{store:KEY}}` substitution opens fresh and closes immediately. Two factorly processes (e.g. CLI + MCP server) never block each other on bbolt's file lock.
- Audit log entries for save/delete (get/search aren't logged — high-frequency, low-value)
- `go.etcd.io/bbolt` dependency added (pure Go, stdlib-only deps)

UI:
- `/store` list page with per-tier cards (workspace / project / global), info popover, "+ Create New Entry" header button
- `/store/new` dedicated create page
- `/store/entry` detail page — full value view, inline edit, TTL/created/last-read metadata, delete
- `Store` top-nav link

### Changed

UI consistency across `/store`, `/vault`, `/auth`, `/blueprints`:
- "Create" forms moved off list pages onto dedicated `/store/new`, `/vault/new`, `/auth/new` pages (blueprints keeps its modal install flow — different shape). List pages are now uncluttered browse views.
- Headers use a unified shape: title + small text-indigo outlined "+ Create New X" button (matches the existing auth-page style); page descriptions tucked behind a hover info icon instead of always-on paragraphs
- Per-tier sections on `/store` and `/vault` render as separate rounded cards with vertical spacing (was one card with hairline dividers)
- Nav reordered: Tools · Workflows · **Vault · Auth · Store** · Blueprints · History — secrets/state group in dependency order; Blueprints moved to setup-tools cluster on the right

Resolver:
- `vault.HasVaultRefs` now excludes `{{store:KEY}}` and `{{expr:...}}` (not just `{{env:VAR}}`) — calls with only those refs no longer trigger a vault password prompt
- `factorly call` parameter resolution uses the cached resolver (vault opened lazily on demand) instead of building a vault-only resolver — `{{store:KEY}}`, `{{env:VAR}}`, and `{{expr:...}}` refs in CLI args now resolve consistently with tool YAML defaults

Audit log:
- `cmd/factorly/audit.go` extracts shared `logKVOp` helper, used by both `logVaultOp` and `logStoreOp`
- `logVaultOp` direct-CLI fallback now routes through `resolveLogPath` (project audit log) instead of `NewJSONL("")` (global default) — `factorly vault set` from a project directory now lands in `.factorly/audit.jsonl` like every other tool

### Fixed
- `factorly vault set` invoked before any other factorly command no longer writes audit entries to `~/.config/factorly/audit.jsonl` when a project-scoped log exists

## [v0.11.2] - 2026-05-20

### Added
- Trello blueprint now includes list labels and card label updates
- `internal/promote` package with `FromLog` and `FromEntry` for recovering scripts from audit log
- CLI command `factorly tools promote --from-sha <prefix> --name <name>`
- "Save as tool" button on `factorly.code` rows in `/history` UI
- GET/POST `/tools/promote` handler with source preview, inferred parameters, and compile-error surfacing
- `codeprov.Validate` export for compile-checking without registration side effects
- Path assertion added to `TestGeneratePathParams` for multi-param path conversion
- New `TestConvertPathPlaceholders` unit test with 6 cases covering edge cases

### Changed
- `historyEntry` gains `SourceSHA` and `Promotable` fields
- History and promote scanner buffer bumped to 1MB to handle embedded scripts

## [v0.11.1] - 2026-05-20

### Changed
- Updated Trello authorization header to correct form
- Polished colors for bullets and pills
- Updated roadmap

### Fixed
- Fixed regression bug in blueprint templates using OpenAPI style path parameters instead of Factorly template parameters
- Fixed missing text parameter for workflow params

## [v0.11.0] - 2026-05-19

Workspaces, code tools, and a unified vault layer.

### Added

**Workspaces**
- Workspaces — named variable and vault overlays via `.factorly/workspaces/<name>.yaml`, selectable via `--workspace`/`-w` flag or `FACTORLY_WORKSPACE`
- Workspaces UI — top-nav pill selector, `/workspaces` CRUD page, inline vault unlock modal, workspace banners on vault/auth/history pages
- `factorly workspaces create` and `factorly workspaces delete` CLI subcommands
- OAuth workspace isolation — per-workspace token bundles land in per-workspace vault files automatically
- Workspace vars overlay wired into `exec` args and proxy runtime parameter resolver for `{{env:VAR}}` refs
- `EnvBackendWithOverrides` registered on proxy runtime resolver so `{{env:VAR}}` in call params resolves against workspace vars
- `factorly init` unconditionally creates `.factorly/workspaces/default.yaml`

**Vault layer**
- `vault.Manager` — mutex-guarded per-scope backend cache shared by CLI and UI
- `vault.Secret` type wrapping password `[]byte` with explicit lifecycle (`NewSecret`, `Zero`, `Clone`, `Empty`, `Len`)
- `vaultTier` abstraction collapsing per-tier function sprawl into `Open`/`ResolvePassword`/`Exists`
- `errExplicitVaultLocked` sentinel distinguishing `--vault-path` locks from global vault locks
- `FallbackBackend.EnsureSecondary()` public method to eagerly warm lazy opens
- `FallbackBackend` `sync.Once` opener serialization preventing double password prompts
- Shared-password chain for workspace → project → global vault unlock (one prompt unlocks all when passwords match)
- Startup vault unlock modal fetched on page load; per-session skip via cookie
- Process-wide shared `bufio.Scanner` in `promptSecret` fixing multi-prompt CLI flows over piped stdin
- `openWithCandidateOrPrompt` returning the password that actually unlocked a vault for chaining
- `LocalBackend.Path()` accessor for vault file path classification

**Code tools (`type: code`)**
- `type: code` tool provider — Go scripts run in a yaegi interpreter, callable via CLI/MCP/UI, with `factorly.Call` re-entering the proxy for full shadow/vault/audit coverage
- `factorly.code` builtin — agent-authored Go scripts submitted at call time via `code` parameter, reusing the V1 engine
- `factorly.ListTools()` in-script SDK function returning `ToolInfo`/`ParamInfo` snapshots
- `Run(ctx, src, params, maxCalls)` entry point on code provider for agent-supplied scripts
- `SHA(src)` helper exported from code provider for audit stamping of agent-supplied scripts
- `source_sha` field in audit log entries for code tools (SHA-256 of script body)

**MCP & UI surface**
- MCP resource registration for tools, workflows, and blueprints (`factorly://` URIs) with `RefreshResources` on UI reload
- "View YAML" pages for tools, workflows, and installed blueprints in the UI with copy and download support
- `factorly tools show <name>` CLI subcommand emitting tool YAML
- `factorly blueprint show <name>` CLI subcommand with fallback to bundled YAML
- Dirty-state guard for tool and workflow edit pages — Save disabled when clean, Cancel shown when dirty, Run disabled when dirty
- Env var editor (key/value rows + strict isolation checkbox) on CLI tool create and edit forms
- Collapsible long descriptions in UI (first paragraph visible, rest behind `<details>` / "Show more")
- `markdownLead` template function splitting descriptions at first blank line
- `description_block` partial for reusable collapsible markdown descriptions
- Markdown rendering for tool/workflow/blueprint descriptions in the UI via goldmark

**Project-scoping**
- Project-scoped audit log at `<project>/.factorly/audit.jsonl`
- Project-scoped rate-limit state at `<project>/.factorly/ratelimit.json`
- Project-scoped workflow run state at `<project>/.factorly/runs/`
- `internal/projectpath` package with shared `Resolve(cfgPath, basename, globalFallback)` helper
- `FACTORLY_LOG_PATH` env var to override the audit log path
- `factorly init` now offers to append `.factorly/` entries to an existing `.gitignore`

**Built-ins & blueprints**
- Context propagation through built-in handlers (`ExecuteWithContext`) honoring caller deadlines
- Unix process-group cancellation for shell built-in preventing orphaned child processes
- Deepgram bundled blueprint

**Tests**
- Integration tests: workspace overlay, vault chain, OAuth isolation, shared-password unlock, UI inheritance of CLI-typed passwords, code tool list/SHA/end-to-end, exec workspace vars, param env-ref resolution, gitignore flow, project-scoped log path, MCP resources, tool/blueprint YAML show, dirty-state templates
- `envWithoutHome()` test helper stripping `HOME` and related vars for vault isolation

### Changed

**Vault layer**
- `ui.Options` shrunk from 12 fields to 5 after `vault.Manager` unification
- `vaultTokenStore` holds a `getBackend()` closure instead of a static backend, updated on workspace switch
- `activeTier()` is the single source of truth for vault precedence order
- `FallbackBackend` memoizes secondary-open failures and surfaces them via `Get`/`Set`/`Delete`/`List` instead of flattening to `ErrNotFound`
- `EnsureSecondary` returns `(Backend, error)` so callers can log warming failures at the call site
- `envSource` strictness is a field rather than slice position
- `workspace.ValidateName` applied everywhere a workspace vault path is constructed, closing a path-traversal seam
- `ValidateName` tightened to allow single interior dots, rejecting only `..`, leading/trailing `.`, and path separators
- `stdinScanner` shared process-wide; first `promptSecret` call initializes it

**Config & init**
- `factorly init` drops from five prompts to three; no longer prompts about tools directory; OpenAPI imports always write to `.factorly/tools/`
- `config.Load` accepts `LoadOption` variadic; `WithWorkspace(name)` registers workspace var overlay
- `ShadowConfig` gains `MaxCalls` field; top-level `ToolConfig.MaxCalls` removed
- Audit log renamed from `calls.jsonl` to `audit.jsonl` in both project and global locations
- Workflow run state directory changed from cwd-relative `.factorly/workflows/` to project-scoped `.factorly/runs/`
- `WorkflowProvider` gains opt-in `SetRunsDir(dir)` setter; empty value disables persistence

**MCP & UI**
- MCP server constructor takes `(reg, p, cfg, cfgPath, agentReg...)` to support resource registration
- `handleWorkflowSave` returns inline "✓ Saved" fragment for htmx callers, redirects only for plain form posts
- Save form `hx-on::after-request` filter tightened to `event.detail.elt === this` to prevent bubbled child requests triggering form reload
- Run panel output clears on each new run
- `tool_new.html` now uses `{{template "tool_form_cli" .}}` shared partial instead of duplicated inline fields

### Fixed

**Vault layer**
- Long-latent bug in `deriveKey` zeroing the caller's password slice mid-call, breaking candidate-password chains
- Workspace-switched UI no longer shows "(locked)" for tiers already opened by the CLI
- `bufio.Scanner` sharing bug causing second password prompt to see empty input and error silently
- OAuth token refreshes now write to the active workspace's vault, not the bootstrap-time vault
- `vault set --workspace ../escape` no longer silently falls through to global vault

**Code tools**
- Code tool with compile error now returns clear "script failed to compile" message instead of "no provider for tool"
- Broken code tools no longer dropped from the registry; compile error is stashed and surfaced on invocation

**Workspace var resolution**
- `{{env:VAR}}` in `factorly call` parameter values now resolves against workspace vars (proxy runtime resolver was missing env backend)
- `{{env:VAR}}` in `factorly exec` args now resolves against workspace vars

**Tests & flakes**
- Data race in `TestLoginFlow*` tests replaced spin-loop on shared string with buffered channel handoff
- Rate-limit buckets no longer shared across projects (was poisoning test isolation)
- Workflow `TestWorkflowStatePersisted` no longer scribbles into cwd
- `TestShadowRateLimit` flake caused by shared global rate-limit file
- `TestLogFilePermissions` latent assertion mismatch fixed by project-scoped log routing

**UI**
- Delete-step button inside `<details><summary>` no longer races the native toggle (`stopPropagation` added)
- Removing a param row no longer leaves index gaps that cause subsequent params to be silently dropped on save (JS reindexers added)

### Removed
- Seven dead `Open*` vault wrapper functions (`OpenProjectVault`, `OpenGlobalVault`, `OpenWorkspaceChain`, etc.)
- Six pure-delegation `Server` vault methods (`cachedWorkspaceVault`, `openVaultWithPassword`, etc.)
- `zeroBytes` helper (replaced by `vault.Secret.Zero`)
- `tryCandidate` helper (folded inline via `Secret.Empty`)
- `resolveVaultPassword`, `resolveVaultPath`, and dead per-tier wrappers from vault refactor
- `TestInitWithToolsDir` integration test (tested a removed code path)
- Tools-directory prompts from `factorly init`
- "Add example workspaces (dev, prod)?" prompt from `factorly init`

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
