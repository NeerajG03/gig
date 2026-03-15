# Roadmap

## What's Done (v0.1.0)

- [x] Core types & ID generation
- [x] SQLite store with versioned migrations (v1 + v2)
- [x] Task CRUD (create, get, update, close, reopen, claim, list, search)
- [x] Task hierarchy (parent/child, tree, children)
- [x] Subtask ID ladder notation (parent.1, parent.2, parent.1.1)
- [x] Custom attributes — two-layer system (definitions + per-task values)
  - [x] Types: string, boolean, object (JSON)
  - [x] Schema registry (define/undefine/types)
  - [x] Per-task CRUD (set/get/list/delete)
  - [x] List filter by attributes (`--attr key=value`)
- [x] Ready/Blocked queries
- [x] Comments
- [x] Dependency DAG with cycle detection
- [x] Event system (audit log + SDK callbacks + shell hooks)
- [x] Config (`gig.yaml`, `GIG_HOME`)
- [x] JSONL export/import
- [x] Embedded web UI (kanban board with drag-and-drop)
  - [x] Board view with top-level/all task filter
  - [x] Task detail with mini kanban for subtasks
  - [x] Drag-and-drop status changes
  - [x] `gig ui` CLI command
  - [x] `ui.New(store)` SDK for embedding in Go apps
- [x] CLI (30+ commands)
- [x] GitHub Actions CI (test on PR)
- [x] SDK example app (`examples/gig-controller/`)

## Next Up

### v0.2.0 — Polish & Distribution

- [ ] **Homebrew tap**: `brew install neerajg/tap/gig`
- [ ] **Shell completions**: bash/zsh/fish via cobra's built-in generator
- [ ] **`gig search`** CLI command (SDK `Search()` exists, CLI missing)
- [ ] **Colored output**: status icons with ANSI colors in terminal
- [ ] **Table formatting**: aligned columns for `gig list` output
- [ ] **`--actor` flag**: global flag to set actor name for events (instead of hardcoded "cli")
- [ ] **Config validation**: `gig doctor` warns on invalid config values
- [ ] **More test coverage**: config, hooks, edge cases

### v0.3.0 — Sync Repo Integration

- [ ] **`gig sync` with git**: if `sync_repo` is set, auto-copy JSONL + commit + push
- [ ] **`gig sync --pull`**: pull from sync repo and import
- [ ] **Conflict resolution**: last-write-wins based on `updated_at` during import
- [ ] **`gig sync --status`**: show if local is ahead/behind sync repo

### v0.4.0 — JumpStreet Integration

- [ ] **JumpStreet imports gig** as a Go dependency
- [ ] **Agent-aware features**: `--actor` maps to agent persona names
- [ ] **Claim locking**: prevent double-claim with atomic check-and-set
- [x] ~~**Workspace association**: link tasks to git worktrees via metadata~~ (done via custom attributes)

### Future Ideas (Not Scoped)

- Full-text search with SQLite FTS5
- `gig template` — reusable task templates (YAML definitions)
- ~~`gig view` — TUI dashboard (bubbletea)~~ (replaced by `gig ui` web kanban)
- Webhook hooks (HTTP POST instead of shell commands)
- Jira/Linear import (one-time migration)
- `gig archive` — move old closed tasks to separate DB
- Metrics/burndown from events table
- MCP server for gig (expose task management to AI agents via tool use)
