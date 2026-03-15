# gig

A lightweight, embeddable task management system written in Go. Use it as a CLI tool or import it as a Go SDK into your own applications.

## Install

```bash
# From source
go install github.com/neerajg/gig/cmd/gig@latest

# Or build locally
git clone https://github.com/neerajg/gig.git
cd gig && go build -o gig ./cmd/gig/
```

## Quick Start

```bash
gig init --prefix myapp          # Creates ~/.gig/ with config + database
gig create "Fix login bug" --type bug --priority 1 --assignee neeraj
gig create "Add OAuth" --type feature --priority 2
gig list
gig show myapp-a3f8
gig comment myapp-a3f8 "Investigating root cause"
gig update myapp-a3f8 --claim --assignee neeraj
gig close myapp-a3f8 --reason "Fixed in commit abc123"
gig stats
```

## Features

- **Task CRUD** — create, list, show, update, close, reopen, search
- **Tree hierarchy** — parent/child tasks via `--parent` with ladder IDs (`gig-a3f8.1`, `.2`, `.3`)
- **Custom attributes** — typed key-value pairs (string, boolean, object) on tasks with schema registry
- **Dependency DAG** — `gig dep add/remove/tree/cycles` with cycle detection
- **Web UI** — built-in kanban board via `gig ui` (drag-and-drop, status colors, subtask boards)
- **Event log** — every mutation recorded, queryable via `gig events <id>`
- **Hook system** — shell commands triggered on status changes (configurable in `gig.yaml`)
- **JSONL sync** — export/import for backup or git-based sync
- **JSON output** — `--json` flag on all query commands for programmatic use
- **SDK-first** — the CLI is a thin wrapper; import `github.com/neerajg/gig` in any Go app

## SDK Usage

```go
import "github.com/neerajg/gig"

store, _ := gig.Open("~/.gig/gig.db", gig.WithPrefix("myapp"))
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

## CLI Reference

```
gig init [--prefix NAME]
gig create <title> [--desc --type --priority --parent --assignee --labels]
gig list [--status --assignee --priority --type --label --parent --attr key=val --limit --json --quiet]
gig show <id> [--json]
gig update <id> [--title --desc --status --priority --assignee --notes --labels --claim]
gig close <id> [id2...] [--reason]
gig reopen <id>
gig comment <id> <message> [--author]
gig comments <id>
gig dep add <task> <depends-on>
gig dep remove <task> <depends-on>
gig dep list <id>
gig dep tree <id>
gig dep cycles
gig attr define <key> --type <string|boolean|object> [--description "..."]
gig attr undefine <key>
gig attr types
gig attr set <task-id> <key> <value>
gig attr get <task-id> <key>
gig attr list <task-id>
gig attr delete <task-id> <key>
gig ready
gig blocked
gig children <id>
gig export [--file]
gig import [--file]
gig sync
gig events <id>
gig stats
gig config
gig doctor
gig ui [--port 9741]
```

## Storage

All data lives centrally in `~/.gig/` (override with `GIG_HOME` env var):

```
~/.gig/
├── gig.db          # SQLite database (gitignored)
├── gig.yaml        # Configuration
├── tasks.jsonl     # Exported tasks (for sync/backup)
└── events.jsonl    # Exported event history
```

## Part of JumpStreet

gig is the task engine for [JumpStreet](https://github.com/neerajg/jump-street), a CLI-based AI agent orchestration platform. JumpStreet imports gig as a Go dependency.

## License

MIT
