# SDK Reference

## Opening a Store

```go
import "github.com/neerajg/gig"

store, err := gig.Open(dbPath, opts...)
defer store.Close()
```

### Options

| Option | Description |
|--------|-------------|
| `gig.WithPrefix("myapp")` | Set ID prefix (default: `gig`) |
| `gig.WithHashLength(6)` | Set hash length 3-8 (default: 4) |
| `gig.WithConfig(&cfg)` | Attach parsed config (enables shell hooks) |

## Tasks

### Create

```go
task, err := store.Create(gig.CreateParams{
    Title:       "Required",
    Description: "Optional",
    Type:        gig.TypeTask,       // task|bug|feature|epic|chore
    Priority:    gig.P2,             // P0(critical) - P4(backlog)
    ParentID:    "gig-a3f8",         // optional parent
    Assignee:    "neeraj",
    Labels:      []string{"backend"},
    Notes:       "freeform text",
    Estimate:    60,                 // minutes
    DueAt:       &dueTime,
    CreatedBy:   "agent-1",
    Metadata:    `{"custom":"json"}`,
})
```

### Read

```go
task, err := store.Get("gig-a3f8")
tasks, err := store.List(gig.ListParams{
    Status:     &status,     // *gig.Status — nil means "any"
    Assignee:   "neeraj",
    Priority:   &priority,   // *gig.Priority
    ParentID:   &parentID,   // *string — "" for root tasks, nil for any
    Type:       &taskType,   // *gig.TaskType
    Label:      "backend",   // substring match
    AttrFilter: map[string]string{"phase": "research"}, // custom attribute filter
    Limit:      20,
    Offset:     0,
})
results, err := store.Search("login bug")      // title + description LIKE search
children, err := store.Children("gig-a3f8")     // direct children
tree, err := store.GetTree("gig-a3f8")          // recursive tree (populates .Children)
```

### Update

```go
// Partial update — nil fields are unchanged
newTitle := "Updated title"
task, err := store.Update("gig-a3f8", gig.UpdateParams{
    Title: &newTitle,
}, "actor-name")

// Status change
err := store.UpdateStatus("gig-a3f8", gig.StatusInProgress, "actor")

// Atomic claim (sets assignee + status=in_progress)
err := store.Claim("gig-a3f8", "agent-1")
```

### Close / Reopen

```go
err := store.CloseTask("gig-a3f8", "reason", "actor")
err := store.CloseMany([]string{"id1", "id2"}, "batch done", "actor")
err := store.Reopen("gig-a3f8", "actor")
```

### Queries

```go
ready, err := store.Ready()     // open tasks with no unresolved blockers
blocked, err := store.Blocked() // tasks with at least one unresolved blocker
```

## Comments

```go
comment, err := store.AddComment("gig-a3f8", "author", "comment text")
comments, err := store.ListComments("gig-a3f8")
```

## Dependencies

```go
// "task-b depends on task-a" (task-a blocks task-b)
err := store.AddDependency("task-b", "task-a", gig.Blocks)
err := store.RemoveDependency("task-b", "task-a")

deps, err := store.ListDependencies("task-b")  // what task-b depends on
dependents, err := store.ListDependents("task-a")  // what depends on task-a

asciiTree, err := store.DepTree("task-b")       // ASCII visualization
cycles, err := store.DetectCycles()              // find all cycles in graph
```

**Dependency types:** `gig.Blocks`, `gig.RelatesTo`, `gig.Duplicates`

Cycle detection runs automatically before every `AddDependency` call.

## Events

### Querying

```go
events, err := store.Events("gig-a3f8")                // all events for a task
events, err := store.EventsSince(time.Now().Add(-24*time.Hour)) // recent events
```

### SDK Callbacks

```go
store.On(gig.EventStatusChanged, func(e gig.Event) {
    fmt.Printf("%s: %s -> %s\n", e.TaskID, e.OldValue, e.NewValue)
})

store.On(gig.EventClosed, func(e gig.Event) {
    triggerNextStep(e.TaskID)
})

store.Off(gig.EventStatusChanged) // remove all callbacks for this type
```

Callbacks fire **synchronously** after the DB write, before the SDK call returns.

### Shell Hooks

Enabled via config:

```go
cfg, _ := gig.LoadConfig("")
store, _ := gig.Open(cfg.DBPath, gig.WithConfig(cfg))
store.EnableHooks() // wires up event system to fire shell hooks from config
```

Shell hooks fire **asynchronously** (goroutine), after SDK callbacks.

## Export / Import

```go
err := store.ExportJSONL("tasks.jsonl")   // deterministic sort by ID
err := store.ImportJSONL("tasks.jsonl")   // upsert semantics
err := store.ExportEvents("events.jsonl") // append-only audit export
```

## Configuration

```go
cfg, err := gig.LoadConfig("")             // loads from DefaultConfigPath()
cfg, err := gig.LoadConfig("/custom/path") // custom path
err := gig.SaveConfig("", &cfg)            // saves to DefaultConfigPath()

gig.DefaultGigHome()    // ~/.gig/ (or GIG_HOME env var)
gig.DefaultDBPath()     // ~/.gig/gig.db
gig.DefaultConfigPath() // ~/.gig/gig.yaml
```

## Custom Attributes

### Define attribute types

```go
// Must define before setting values
err := store.DefineAttr("worktree", gig.AttrString, "Git worktree path")
err := store.DefineAttr("tested", gig.AttrBoolean, "Whether tests passed")
err := store.DefineAttr("config", gig.AttrObject, "Agent runtime config")

defs, err := store.ListAttrDefs()         // list all definitions
def, err := store.GetAttrDef("worktree")  // get one definition
err := store.UndefineAttr("worktree")     // remove definition + all values
```

### Set / get attribute values

```go
err := store.SetAttr("gig-a3f8", "worktree", "/tmp/wt-1")        // string
err := store.SetAttr("gig-a3f8", "tested", "true")                // boolean
err := store.SetAttr("gig-a3f8", "config", `{"model":"opus"}`)    // object (JSON)

attr, err := store.GetAttr("gig-a3f8", "worktree")
fmt.Println(attr.StringValue())  // "/tmp/wt-1"
fmt.Println(attr.Type)           // "string"

attr, _ := store.GetAttr("gig-a3f8", "tested")
fmt.Println(attr.BoolValue())    // true

attr, _ := store.GetAttr("gig-a3f8", "config")
obj, _ := attr.ObjectValue()     // map[string]any{"model": "opus"}

attrs, err := store.Attrs("gig-a3f8")       // all attributes on a task
err := store.DeleteAttr("gig-a3f8", "tested") // remove single attribute
```

### Filter tasks by attributes

```go
tasks, err := store.List(gig.ListParams{
    AttrFilter: map[string]string{
        "phase":  "research",
        "tested": "true",
    },
})
```

**Attribute types:** `gig.AttrString`, `gig.AttrBoolean`, `gig.AttrObject`

## Web UI

```go
import "github.com/neerajg/gig/ui"

server := ui.New(store)
server.ListenAndServe(":9741")

// Or get the http.Handler for custom servers:
handler := server.Handler()
```

## Types Quick Reference

```go
// Status
gig.StatusOpen, gig.StatusInProgress, gig.StatusBlocked, gig.StatusDeferred, gig.StatusClosed

// Priority
gig.P0 (critical), gig.P1 (high), gig.P2 (medium), gig.P3 (low), gig.P4 (backlog)

// TaskType
gig.TypeTask, gig.TypeBug, gig.TypeFeature, gig.TypeEpic, gig.TypeChore

// DepType
gig.Blocks, gig.RelatesTo, gig.Duplicates

// EventType
gig.EventCreated, gig.EventUpdated, gig.EventStatusChanged,
gig.EventCommented, gig.EventAssigned, gig.EventClosed,
gig.EventDependencyAdded, gig.EventDependencyRemoved

// AttrType
gig.AttrString, gig.AttrBoolean, gig.AttrObject
```
