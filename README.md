# gig

**Lightweight, embeddable task management system for Go — CLI + SDK backed by SQLite.**

**Platforms:** macOS, Linux, Windows

[![License](https://img.shields.io/github/license/NeerajG03/gig)](LICENSE)
[![Go Report Card](https://goreportcard.com/badge/github.com/NeerajG03/gig)](https://goreportcard.com/report/github.com/NeerajG03/gig)
[![Test](https://github.com/NeerajG03/gig/actions/workflows/test.yml/badge.svg)](https://github.com/NeerajG03/gig/actions/workflows/test.yml)

gig gives you task tracking with dependencies, hierarchy, events, and a built-in web UI — all in a single binary with zero runtime dependencies. Use it as a standalone CLI or import it as a Go SDK into your own applications.

## Quick Start

```bash
# Install
brew install neerajg03/tap/gig

# Or via Go
go install github.com/NeerajG03/gig/cmd/gig@latest

# Initialize
gig init --prefix myapp

# Start tracking
gig create "Fix login bug" --type bug --priority 1 --assignee neeraj
gig create "Add OAuth" --type feature --priority 2
gig list
gig show myapp-a3f8
gig close myapp-a3f8 --reason "Fixed in commit abc123"
```

## Features

- **SDK-first** — the CLI is a thin wrapper; import `github.com/NeerajG03/gig` in any Go app.
- **Pure Go SQLite** — single binary, no CGO, no runtime dependencies. Uses [modernc.org/sqlite](https://pkg.go.dev/modernc.org/sqlite).
- **Task hierarchy** — parent/child tasks with ladder IDs (`gig-a3f8.1`, `.1.1`). Full tree view via `gig list --tree`.
- **Dependency DAG** — `gig dep add/remove/tree/cycles` with automatic cycle detection.
- **Custom attributes** — typed key-value metadata (string, boolean, JSON object) with a schema registry.
- **Event system** — every mutation recorded. SDK callbacks, shell hooks, queryable audit log.
- **Web UI** — built-in kanban board via `gig ui` with drag-and-drop status changes.
- **Agent-friendly** — `--json` output on all commands, `--actor` flag for attribution, `--quiet` for scripting.
- **Shell completions** — dynamic tab-completion for task IDs, flags, and attribute keys (bash/zsh/fish).
- **JSONL sync** — export/import for backup or git-based collaboration.

## Essential Commands

| Command | Action |
|---------|--------|
| `gig create "Title" --priority 1` | Create a task. |
| `gig list` | List open tasks (table view). |
| `gig list --tree` | Hierarchical tree view. |
| `gig show <id>` | Task details, comments, deps, subtree. |
| `gig update <id> --claim` | Atomically claim a task (sets assignee + in_progress). |
| `gig close <id> --reason "done"` | Close a task. |
| `gig dep add <task> <blocker>` | Add a dependency. |
| `gig ready` | Tasks with no unresolved blockers. |
| `gig search <query>` | Search titles and descriptions. |
| `gig config set <key> <value>` | Update configuration. |
| `gig ui` | Launch web kanban board. |

## Hierarchy & IDs

gig uses hierarchical IDs for structured task breakdown:

- `myapp-a3f8` — Epic
- `myapp-a3f8.1` — Task
- `myapp-a3f8.1.1` — Subtask

Create subtasks with `--parent`:

```bash
gig create "Design API" --type epic
gig create "Implement endpoints" --parent myapp-a3f8
gig create "Write tests" --parent myapp-a3f8
```

## SDK Usage

```go
import "github.com/NeerajG03/gig"

store, _ := gig.Open("tasks.db", gig.WithPrefix("myapp"))
defer store.Close()

task, _ := store.Create(gig.CreateParams{
    Title:    "Implement feature X",
    Type:     gig.TypeFeature,
    Priority: gig.P1,
})

store.On(gig.EventStatusChanged, func(e gig.Event) {
    fmt.Printf("Task %s: %s -> %s\n", e.TaskID, e.OldValue, e.NewValue)
})

store.UpdateStatus(task.ID, gig.StatusInProgress, "agent-1")
```

## Configuration

All data lives in `~/.gig/` (override with `GIG_HOME` env var):

```
~/.gig/
├── gig.db          # SQLite database
├── gig.yaml        # Configuration
├── tasks.jsonl     # Exported tasks (for sync)
└── events.jsonl    # Exported event history
```

Configure via CLI or YAML:

```bash
gig config set default_view tree    # "list" or "tree"
gig config set show_all true        # include closed tasks
gig config set prefix myapp         # ID prefix
gig config set hash_length 6        # ID hash length (3-8)
```

## Shell Completions

```bash
source <(gig completion bash)     # bash
source <(gig completion zsh)      # zsh
gig completion fish | source      # fish
```

## Installation

```bash
# Homebrew (recommended)
brew install neerajg03/tap/gig

# Or Go install
go install github.com/NeerajG03/gig/cmd/gig@latest

# Or build from source
git clone https://github.com/NeerajG03/gig.git
cd gig && go build -o gig ./cmd/gig/
```

**Requirements:** Go 1.21+

## Documentation

- [Architecture](docs/architecture.md) — system design, data flow, schema
- [SDK Reference](docs/sdk-reference.md) — full API documentation
- [Roadmap](docs/roadmap.md) — what's done and what's next
- [Security](SECURITY.md) — vulnerability reporting and scope

## License

MIT
