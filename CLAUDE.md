# gig

Lightweight task management system — Go CLI + SDK backed by SQLite.

## Layout

```
gig/
├── gig.go              # Core types: Task, Comment, Event, Attribute, Status, Priority, enums
├── id.go               # Hash-based ID generation (prefix-xxxx)
├── store.go            # Store: Open/Close, DB setup, event emitter, WAL+FK pragmas
├── task.go             # Task CRUD: Create, Get, Update, Close, List, Search, Ready, Blocked, Claim, Tree
├── attribute.go        # Custom attributes: DefineAttr, SetAttr, GetAttr, Attrs, DeleteAttr
├── comment.go          # AddComment, ListComments
├── dependency.go       # AddDependency, RemoveDependency, ListDeps, DepTree, DetectCycles
├── event.go            # Events, EventsSince (query the audit log)
├── hook.go             # Shell hook execution from gig.yaml config + EnableHooks()
├── config.go           # LoadConfig, SaveConfig, DefaultGigHome, gig.yaml parsing
├── export.go           # ExportJSONL, ImportJSONL, ExportEvents
├── util.go             # Time helpers, JSON label marshaling
├── ui/                 # Embedded web UI (kanban board)
│   ├── server.go       # HTTP server, handlers, template funcs — ui.New(store)
│   └── templates/      # HTML templates + CSS (embedded via embed.FS)
├── cmd/gig/            # CLI (cobra) — thin wrapper over SDK
│   ├── main.go         # Root command, store lifecycle, --json/--quiet/--actor flags
│   ├── task_cmds.go    # create, list, show, update, close, reopen, ready, blocked, children, search
│   ├── color.go        # ANSI color output, terminal detection, NO_COLOR support
│   ├── attr_cmds.go    # attr define/undefine/types/set/get/list/delete
│   ├── dep_cmds.go     # dep add/remove/list/tree/cycles
│   ├── comment_cmds.go # comment, comments
│   ├── sync_cmds.go    # export, import, sync
│   ├── ui_cmd.go       # ui — starts embedded web kanban board
│   └── util_cmds.go    # init, events, stats, config, doctor
├── examples/gig-controller/ # SDK usage example (standalone web app with demo data)
├── internal/migrate/
│   └── migrations.go   # Versioned SQLite schema migrations (v1: core, v2: custom attrs)
├── *_test.go           # Tests for each SDK layer
├── .github/workflows/
│   └── test.yml        # CI: runs tests on PRs
└── docs/               # Technical documentation
```

## Build & Test

```bash
go build -o gig ./cmd/gig/    # Build binary
go test ./...                   # Run all tests
go vet ./...                    # Static analysis
```

## Key Design Decisions

- **SDK-first**: All logic lives in the root package. CLI (`cmd/gig/`) is a thin cobra wrapper that calls SDK methods. Never put business logic in `cmd/`.
- **Pure Go SQLite**: Uses `modernc.org/sqlite` (no CGO). This keeps the binary portable with zero runtime dependencies.
- **Status enums are prefixed**: `StatusOpen`, `StatusClosed`, etc. (not `Open`, `Closed`) to avoid collisions with method names like `Store.Close()`.
- **Subtask ID ladder**: Children get IDs like `parent.1`, `parent.2`. Grandchildren: `parent.1.1`. Only root tasks get random hash IDs.
- **Time stored as RFC3339 strings**: SQLite doesn't have a native datetime type. All scan functions read timestamps as strings and parse via `strToTime()`.
- **Events are automatic**: Every mutation in `task.go`, `comment.go`, `dependency.go`, `attribute.go` calls `s.recordEvent()`. This is the audit log.
- **Event flow**: DB write → Event table insert → SDK callbacks (sync) → Shell hooks (async goroutine).
- **JSONL for sync**: The `.db` file is never committed to git. Only deterministically-sorted JSONL files are synced. Import uses `ON CONFLICT ... DO UPDATE` (upsert) with FK checks temporarily disabled for parent ordering.
- **Custom attributes are two-layer**: Definitions registry (what keys exist + their types) → per-task values. FK constraint enforces you can't set an undefined key.
- **Web UI is embedded**: `ui/` package uses Go's `embed.FS` to bake templates into the binary. No external files needed at runtime.
- **Colored output**: Uses ANSI escape codes via `golang.org/x/term` for terminal detection. Respects `NO_COLOR` env var. Colors auto-disable when piped. All color logic lives in `cmd/gig/color.go`.
- **`--actor` flag**: Global CLI flag that sets the actor name in events. Defaults to `"cli"`. Used by JumpStreet agents to attribute actions (`--actor agent-coder`).

## Conventions

- All public SDK functions return `(*Type, error)` or `error`
- Use `UpdateParams` with pointer fields (nil = don't change) for partial updates
- IDs are generated via `GenerateID(prefix, hashLen)` — SHA256 of UUID+timestamp, truncated. Subtask IDs use ladder notation (parent.N)
- `GIG_HOME` env var overrides `~/.gig/` as the central storage location
- Tests use `tempDB(t)` helper which creates an in-memory store with cleanup

## What NOT to Do

- Don't add CGO dependencies — the pure-Go SQLite constraint is intentional
- Don't put business logic in `cmd/gig/` — keep it in root package SDK functions
- Don't scan `time.Time` directly from SQLite — always scan as string, parse with `strToTime()`. The pure-Go SQLite driver returns timestamps as strings, not `time.Time`. Scanning into `time.Time` compiles but fails at runtime with `unsupported Scan, storing driver.Value type string into type *time.Time`. See `comment_test.go:TestListCommentsCreatedAtParsed` for the regression test.
- Don't break JSONL format without a major version bump — it's the sync contract
- Don't remove columns from migrations — only add (forward-compatible)
