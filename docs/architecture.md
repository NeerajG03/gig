# Architecture

## Overview

gig has two interfaces sharing one core:

```
┌─────────────┐     ┌─────────────┐
│   CLI       │     │  Go SDK     │
│  (cobra)    │     │  (import)   │
└──────┬──────┘     └──────┬──────┘
       │                   │
       └───────┬───────────┘
               │
        ┌──────▼──────┐
        │    Store     │  ← single entry point
        │  (store.go)  │
        └──────┬──────┘
               │
    ┌──────────┼──────────┐
    │          │          │
┌───▼──┐  ┌───▼──┐  ┌───▼──────┐  ┌───▼──────┐
│ task │  │ dep  │  │ comment  │  │attribute │
│ .go  │  │ .go  │  │ .go      │  │ .go      │
└───┬──┘  └───┬──┘  └───┬──────┘  └───┬──────┘
    │         │          │             │
    └─────────┼──────────┼─────────────┘
              │
       ┌──────▼──────┐
       │  event.go   │  ← every mutation writes here
       │  hook.go    │  ← then fires callbacks + shell hooks
       └──────┬──────┘
              │
       ┌──────▼──────┐
       │   SQLite    │
       │  (WAL mode) │
       └─────────────┘
```

## Store Lifecycle

```go
store, err := gig.Open(dbPath, opts...)  // creates dir, opens DB, runs migrations, enables WAL+FK
defer store.Close()
```

`Open()` is the only constructor. It handles:
1. Creating parent directories for the DB file
2. Opening SQLite with `modernc.org/sqlite` (pure Go, no CGO)
3. Setting `PRAGMA journal_mode=WAL` (concurrent reads)
4. Setting `PRAGMA foreign_keys=ON`
5. Running versioned migrations from `internal/migrate/`
6. Applying options (prefix, hash length, config)

## Data Flow: Mutation

Every write follows the same path:

```
SDK call (e.g., store.Create())
  → validate inputs
  → execute SQL
  → recordEvent()          ← writes to events table
    → emit()               ← fires SDK callbacks (synchronous)
      → RunHooks()         ← fires shell hooks (async goroutine)
  → return result
```

## Data Flow: Query

Queries go directly to SQLite:

```
SDK call (e.g., store.List())
  → build SQL WHERE clause from ListParams
  → execute query
  → scanTasks() / scanTask()   ← handles string→time parsing
  → return []*Task
```

## Database Schema (v1 + v2)

Six tables + a migrations tracker:

| Table | Version | Purpose |
|-------|---------|---------|
| `tasks` | v1 | Primary entity — title, status, priority, assignee, parent, labels, etc. |
| `comments` | v1 | Text notes attached to tasks (FK cascade delete) |
| `dependencies` | v1 | DAG edges: from_id depends on to_id, with type (blocks/relates_to/duplicates) |
| `events` | v1 | Append-only audit log — every mutation recorded with old/new values |
| `attribute_definitions` | v2 | Schema registry — defines allowed attribute keys + types (string/boolean/object) |
| `custom_attributes` | v2 | Per-task typed key-value pairs (FK to definitions + tasks, cascade delete) |
| `schema_migrations` | — | Tracks which migration versions have been applied |

See `internal/migrate/migrations.go` for the full DDL.

## ID Generation

**Root tasks:**
```
prefix + "-" + sha256(uuid + timestamp)[:hashLen]
```
Example: `gig-a3f8`, `demo-c136`

**Subtasks (ladder notation):**
```
parentID + "." + childNumber
```
Example: `gig-a3f8.1`, `gig-a3f8.2`, `gig-a3f8.1.1` (grandchild)

- Default prefix: `gig` (configurable)
- Default hash length: 4 characters (configurable 3-8)
- Child numbering is sequential based on existing children count at creation time
- The ladder notation makes task hierarchy immediately visible from the ID

## Custom Attributes

Two-layer system for typed key-value metadata on tasks:

**Layer 1 — Definitions (schema registry):**
- `attribute_definitions` table stores allowed keys + types
- Types: `string`, `boolean`, `object` (JSON stored as string)
- Must define a key before setting values on tasks

**Layer 2 — Values (per-task data):**
- `custom_attributes` table stores task_id + key + value
- FK constraint: key must exist in definitions
- FK cascade: deleting a task deletes its attributes
- Type validation on write (boolean must be "true"/"false", object must be valid JSON)

## Embedded Web UI

The `ui/` package provides a drag-and-drop kanban board:
- Embedded via `embed.FS` — no external files at runtime
- Usable as `gig ui` CLI command or as `ui.New(store).ListenAndServe(addr)` in Go apps
- Features: board view with top-level/all filter, task detail with mini kanban for subtasks, drag-and-drop status changes, HTMX-powered interactions

## Event System

### Three layers:

1. **Events table** (always) — append-only audit log in SQLite
2. **SDK callbacks** (opt-in) — `store.On(EventType, func(Event))`, fires synchronously
3. **Shell hooks** (opt-in) — commands in `gig.yaml`, fires as async goroutines

### Event types:
`created`, `updated`, `status_changed`, `commented`, `assigned`, `closed`, `dependency_added`, `dependency_removed`

### Hook configuration:
```yaml
hooks:
  on_status_change:
    - command: "notify.sh {id} {new}"
      filter:
        new_status: "closed"
```

Template variables: `{id}`, `{old}`, `{new}`, `{actor}`, `{field}`

## Sync Model

```
~/.gig/
├── gig.db          # SQLite database (LOCAL ONLY — never committed)
├── gig.yaml        # Config (committed if in a sync repo)
├── tasks.jsonl     # Deterministically sorted, one task per line
└── events.jsonl    # Append-only event export
```

- `gig sync` exports both JSONL files
- JSONL is sorted by task ID for clean git diffs
- Import uses `ON CONFLICT DO UPDATE` (upsert) with FK checks disabled
- On a new machine: clone sync repo → `gig import` → DB rebuilt

## Dependency Graph

- Stored as edges in `dependencies` table (from_id, to_id, type)
- Cycle detection uses BFS from target back to source before every `AddDependency`
- `Ready()` query: tasks where no blocker has status != 'closed'
- `Blocked()` query: inverse of Ready
- `DepTree()` renders ASCII tree via recursive traversal
- `DetectCycles()` does full graph DFS for audit/doctor purposes

## Configuration

Central location: `~/.gig/gig.yaml` (override with `GIG_HOME` env var)

```yaml
prefix: "gig"                    # ID prefix
db_path: "/Users/x/.gig/gig.db" # Database location
hash_length: 4                   # ID hash length (3-8)
sync_repo: ""                    # Optional git repo for sync
hooks:                           # Shell hooks by event type
  on_status_change: [...]
  on_create: [...]
  on_comment: [...]
  on_close: [...]
  on_assign: [...]
```
