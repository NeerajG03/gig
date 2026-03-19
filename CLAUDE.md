# gig

Lightweight task management system — Go CLI + SDK backed by SQLite.

## Layout

```
gig/
├── doc.go              # Package documentation with file layout guide
├── gig.go              # Core types: Task, Comment, Checkpoint, Event, Attribute, Status, Priority, enums
├── store.go            # Store: Open/Close, ID generation, event emitter, WAL+FK pragmas
├── task.go             # Task mutations: Create, Get, Update, Close, Cancel, Reopen, Claim, autoUnblock
├── query.go            # Task queries: List, Search, Ready(parentID), Blocked, Children, GetTree
├── checkpoint.go       # AddCheckpoint, ListCheckpoints, LatestCheckpoint (structured progress snapshots)
├── doctor.go           # Doctor(): health checks (integrity, orphans, cycles, config)
├── attribute.go        # Custom attributes: DefineAttr, SetAttr, GetAttr, Attrs, DeleteAttr
├── comment.go          # AddComment, ListComments
├── dependency.go       # AddDependency, RemoveDependency, ListDeps, DepTree, DetectCycles
├── event.go            # Events, EventsSince (query the audit log)
├── hook.go             # Shell hook execution from gig.yaml config + EnableHooks()
├── hooks.go            # //go:embed hooks + MaterializeHooks (install agent/git hooks)
├── config.go           # LoadConfig, SaveConfig, DefaultGigHome, gig.yaml parsing
├── export.go           # ExportJSONL, ImportJSONL, ExportEvents
├── util.go             # Time helpers, JSON label marshaling
├── ui/                 # Embedded web UI (kanban board)
│   ├── server.go       # HTTP server, handlers, template funcs — ui.New(store)
│   └── templates/      # HTML templates + CSS (embedded via embed.FS)
├── hooks/              # Embedded hook scripts (materialized by `gig install hooks`)
│   ├── agent/          # Claude Code hooks: session-start, pre-compact, pickup
│   └── git/            # Git hooks: post-commit
├── cmd/gig/            # CLI (cobra) — thin wrapper over SDK
│   ├── main.go         # Root command, store lifecycle, --json/--quiet/--actor flags
│   ├── task_cmds.go    # create, show, update, close, cancel, reopen + rendering helpers
│   ├── list_cmd.go     # list (tree/flat modes, filtering, ExcludeStatuses)
│   ├── query_cmds.go   # search, ready (--id scope), blocked, children
│   ├── checkpoint_cmds.go # checkpoint, checkpoints
│   ├── config_cmd.go   # config, config set
│   ├── install_cmd.go  # install hooks (--claude, --git, --root)
│   ├── color.go        # ANSI color output, terminal detection, NO_COLOR support
│   ├── completion.go   # Shell completions (bash/zsh/fish) + dynamic task ID/flag completion
│   ├── attr_cmds.go    # attr define/undefine/types/set/get/list/delete
│   ├── dep_cmds.go     # dep add/remove/list/tree/cycles
│   ├── comment_cmds.go # comment, comments
│   ├── sync_cmds.go    # export, import, sync
│   ├── ui_cmd.go       # ui — starts embedded web kanban board
│   ├── util_cmds.go    # init, events, stats, doctor
│   └── cli_test.go     # E2E CLI tests (build tag: e2e)
├── examples/gig-controller/ # SDK usage example (standalone web app with demo data)
├── internal/migrate/
│   └── migrations.go   # Versioned SQLite schema migrations (v1: core, v2: custom attrs, v3: checkpoints)
├── *_test.go           # SDK unit tests (task_test, config_test, comment_test, checkpoint_test, etc.)
├── .github/workflows/
│   ├── test.yml        # CI: unit tests + E2E CLI tests on PRs
│   └── release.yml     # Release: tag → tests → GitHub release → homebrew tap update
└── docs/               # Technical documentation
```

## Build & Test

```bash
go build -o gig ./cmd/gig/                       # Build binary
go test ./...                                      # SDK unit tests
go test -tags=e2e -v -count=1 ./cmd/gig/           # E2E CLI tests
go vet ./...                                       # Static analysis
```

## Key Design Decisions

- **SDK-first**: All logic lives in the root package. CLI (`cmd/gig/`) is a thin cobra wrapper that calls SDK methods. Never put business logic in `cmd/`.
- **Pure Go SQLite**: Uses `modernc.org/sqlite` (no CGO). This keeps the binary portable with zero runtime dependencies.
- **Status enums are prefixed**: `StatusOpen`, `StatusInProgress`, `StatusBlocked`, `StatusDeferred`, `StatusClosed`, `StatusCancelled`. `IsTerminal()` returns true for closed/cancelled.
- **Terminal states**: Both `closed` and `cancelled` are terminal. `IsTerminal()` is used throughout (auto-unblock, Ready queries, Reopen, completions) — always use it instead of checking `== StatusClosed` directly.
- **Auto-unblock**: When a task is closed or cancelled, `autoUnblock()` checks all dependents with `blocked` status. If all their blockers are now terminal, they transition to `open` automatically.
- **Ready = available to pick up**: `Ready(parentID)` returns only `open` tasks with no unresolved blockers. `in_progress` tasks are already claimed — they're "active", not "ready". Optional `parentID` scopes to children of a task.
- **Checkpoints are separate from comments**: Checkpoints are structured progress snapshots (done, decisions, next, blockers, files) in their own table. Comments are freeform notes. `gig show` displays the latest checkpoint.
- **Subtask ID ladder**: Children get IDs like `parent.1`, `parent.2`. Grandchildren: `parent.1.1`. Only root tasks get random hash IDs.
- **Time stored as RFC3339 strings**: SQLite doesn't have a native datetime type. All scan functions read timestamps as strings and parse via `strToTime()`.
- **Events are automatic**: Every mutation in `task.go`, `comment.go`, `checkpoint.go`, `dependency.go`, `attribute.go` calls `s.recordEvent()`. This is the audit log.
- **Event flow**: DB write → Event table insert → SDK callbacks (sync) → Shell hooks (async goroutine).
- **JSONL for sync**: The `.db` file is never committed to git. Only deterministically-sorted JSONL files are synced. Import uses `ON CONFLICT ... DO UPDATE` (upsert) with FK checks temporarily disabled for parent ordering.
- **Custom attributes are two-layer**: Definitions registry (what keys exist + their types) → per-task values. FK constraint enforces you can't set an undefined key.
- **Embedded hooks**: `hooks.go` uses `//go:embed` to bake agent and git hook scripts into the binary. `gig install hooks` materializes them to `$GIG_HOME/hooks/` and wires them into Claude Code `settings.json` and git `.git/hooks/`.
- **Web UI is embedded**: `ui/` package uses Go's `embed.FS` to bake templates into the binary. No external files needed at runtime.
- **Colored output**: Uses ANSI escape codes via `golang.org/x/term` for terminal detection. Respects `NO_COLOR` env var. Colors auto-disable when piped. All color logic lives in `cmd/gig/color.go`.
- **Status icons**: `[ ]` open, `[>]` in_progress (blue), `[!]` blocked (red), `[~]` deferred (yellow), `[x]` closed (green), `[-]` cancelled (magenta).
- **`--actor` flag**: Global CLI flag that sets the actor name in events. Defaults to `"cli"`. Used by JEFF agents to attribute actions (`--actor jeff`).
- **Config-driven CLI defaults**: `default_view` (list/tree) and `show_all` (bool) in `gig.yaml`. Resolution order: CLI flag > config > default. See `listCmd()` in `task_cmds.go`.
- **Tree view in list**: `gig list --tree` renders hierarchical ASCII tree. `--all` includes closed/cancelled tasks. `filterTree()` recursively prunes excluded statuses.
- **Shell completions**: `completion.go` provides dynamic completions — `openTaskIDCompletion` (for close/cancel), `closedTaskIDCompletion` (for reopen — includes cancelled), `attrKeyCompletion`. Every command that takes a task ID has `ValidArgsFunction` wired. Flag values (--status, --type, --priority) use `RegisterFlagCompletionFunc`.

## Conventions

- All public SDK functions return `(*Type, error)` or `error`
- Use `UpdateParams` with pointer fields (nil = don't change) for partial updates
- IDs are generated via `GenerateID(prefix, hashLen)` — SHA256 of UUID+timestamp, truncated. Subtask IDs use ladder notation (parent.N)
- `GIG_HOME` env var overrides `~/.gig/` as the central storage location
- Module path is `github.com/NeerajG03/gig` — must match the GitHub repo URL
- Tests use `tempDB(t)` helper which creates an in-memory store with cleanup

## What NOT to Do

- Don't add CGO dependencies — the pure-Go SQLite constraint is intentional
- Don't put business logic in `cmd/gig/` — keep it in root package SDK functions
- Don't scan `time.Time` directly from SQLite — always scan as string, parse with `strToTime()`. The pure-Go SQLite driver returns timestamps as strings, not `time.Time`. Scanning into `time.Time` compiles but fails at runtime with `unsupported Scan, storing driver.Value type string into type *time.Time`. See `comment_test.go:TestListCommentsCreatedAtParsed` for the regression test.
- Don't break JSONL format without a major version bump — it's the sync contract
- Don't remove columns from migrations — only add (forward-compatible)
- Don't check `== StatusClosed` directly — use `IsTerminal()` to handle both closed and cancelled
- Don't re-tag released versions — Go module proxy caches checksums. Always bump to a new version number.

## How to Add a New CLI Command

1. **SDK first**: Add the business logic in the root package (e.g., `task.go`, or a new file). Return `(*Type, error)`.
2. **Wire the command**: Add a `func fooCmd() *cobra.Command` in the appropriate `cmd/gig/*.go` file. Keep it thin — parse flags, call SDK, format output.
3. **Register it**: Add `fooCmd()` to `rootCmd.AddCommand(...)` in `cmd/gig/main.go`.
4. **Skip store init if needed**: If the command doesn't need the database (like `init`, `completion`, or `install` subcommands), add it to the skip list in `PersistentPreRunE`.
5. **Support output modes**: Honor `--json` (full JSON), `--quiet` (IDs only), and default (human-readable table/tree).
6. **Wire autocomplete**:
   - If the command takes a task ID as a positional arg, set `ValidArgsFunction`:
     - `taskIDCompletion` — all tasks (general purpose)
     - `openTaskIDCompletion` — non-closed tasks only (for commands like `close`, `cancel`)
     - `closedTaskIDCompletion` — closed/cancelled tasks only (for commands like `reopen`)
   - If the command has enum flags (--status, --type, --priority), register completions:
     ```go
     cmd.RegisterFlagCompletionFunc("status", statusCompletion)
     cmd.RegisterFlagCompletionFunc("type", taskTypeCompletion)
     cmd.RegisterFlagCompletionFunc("priority", priorityCompletion)
     ```
   - For custom attribute keys, use `attrKeyCompletion`.
   - All completion functions live in `cmd/gig/completion.go`.
7. **Use `actorName`**: Replace hardcoded `"cli"` with the package-level `actorName` variable for event attribution.

## Testing

### Running Tests

```bash
go test ./...           # All tests
go test -v ./...        # Verbose output
go test -run TestList   # Run specific test(s) by name pattern
go test -count=1 ./...  # Disable test caching
```

### Test Infrastructure

- **`tempDB(t)`** (`store_test.go`): Creates a temporary SQLite store with prefix `"test"` and auto-cleanup. Every test that needs a store should use this — never share state between tests.
- Tests are in `*_test.go` files alongside the code they test (same `package gig`).
- No external test dependencies — just `testing` stdlib.

### How to Write Tests

1. **One test file per source file**: `task.go` → `task_test.go`, `config.go` → `config_test.go`, etc.
2. **Start with `tempDB(t)`**: Get a clean store for each test function.
3. **Test the SDK, not the CLI**: Tests go in the root package and exercise SDK methods directly. The CLI is a thin wrapper — if the SDK is correct, the CLI is correct.
4. **Write regression tests for bugs**: If a bug was found (especially runtime bugs that compile but fail), add a test that would have caught it. Document the bug in the test name or comments. Example: `TestListCommentsCreatedAtParsed` is a regression test for the `time.Time` scanning bug.
5. **Test validation and edge cases**: Empty inputs, nonexistent IDs, invalid enum values, boundary conditions (e.g., `hash_length: 1` and `hash_length: 20` in config tests).
6. **Use table-driven tests** for parameterized cases when testing many inputs against the same logic.

### Existing Test Files

| File | What it tests |
|------|--------------|
| `store_test.go` | `tempDB()` helper, Open/Close, directory creation, migration idempotency |
| `task_test.go` | CRUD, list filters, search, tree, subtask ID ladder, ExcludeStatuses, root task filter, ready/blocked, auto-unblock, cancel, IsTerminal |
| `checkpoint_test.go` | Add/list/latest checkpoints, files linking, validation, no-files round-trip |
| `comment_test.go` | Add/list comments, time parsing regression, validation |
| `dependency_test.go` | Add/remove deps, cycle detection, dep tree |
| `attribute_test.go` | Define/set/get/list/delete attrs, type validation |
| `export_test.go` | JSONL export/import round-trip |
| `config_test.go` | Config defaults, view settings, hash length bounds, invalid YAML, hooks parsing, save/reload |
| `hook_test.go` | Filter matching, variable expansion, RunHooks dispatch, EnableHooks |
| `event_test.go` | Event recording, status change events, EventsSince, listeners (On/Off), multiple listeners |
| `doctor_test.go` | Healthy DB, no config, valid/invalid config warnings, integrity check, HasIssues |

## Code Quality

- **`go vet ./...`**: Run before committing. Catches common mistakes (printf format mismatches, unreachable code, etc.).
- **No linter yet**: Consider `golangci-lint` for stricter checks in the future.
- **Keep CLI thin**: Business logic belongs in SDK functions (root package). CLI files (`cmd/gig/`) should only parse flags, call SDK, format output. If you find yourself writing `if/else` chains in CLI code, that logic probably belongs in the SDK.
- **Error wrapping**: Use `fmt.Errorf("context: %w", err)` to wrap errors with context. Never swallow errors silently.
- **Parameterized SQL**: All queries use `?` placeholders. Never use `fmt.Sprintf` to build SQL — this is a hard rule for security.
- **Time handling**: Always use `strToTime()` for parsing timestamps from SQLite. Never scan `time.Time` directly (see "What NOT to Do" above).

## JEFF Integration

gig is the state layer for [JEFF](https://github.com/NeerajG03/JEFF) (agent workspace manager). JEFF imports gig as a Go SDK — it never shells out to the gig CLI.

Key SDK methods JEFF uses:
- `Open`, `LoadConfig`, `DefaultGigHome` — initialization
- `Get`, `Claim`, `CloseTask`, `CancelTask` — task lifecycle
- `Create` (with ParentID), `AddDependency` — subtask decomposition
- `Ready(parentID)` — find next available subtask
- `AddCheckpoint`, `LatestCheckpoint`, `ListCheckpoints` — structured progress
- `DefineAttr`, `SetAttr`, `GetAttr` — custom attributes (repos, worktree_setup)
- `AddComment`, `ListComments` — freeform notes

JEFF defines custom attributes in gig: `repos` (object — JSON array of repo names) and `worktree_setup` (string — post-setup script path).
