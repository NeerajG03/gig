# PLAN: Concurrency-Safe Storage

**Rank: 1 of 5 (highest leverage — do this first).**
gig's core use case is JEFF driving multiple concurrent agents against one SQLite DB. The storage layer currently fails at exactly that. Every defect below was **reproduced on this codebase** (2026-07-07, commit `08c7a13`):

1. **Foreign keys are not enforced.** `PRAGMA foreign_keys=ON` in `Open()` (store.go:103) runs on *one* pooled connection; `database/sql` opens more connections under concurrency and SQLite defaults FK to OFF on each new one. Measured: **100/100 inserts referencing a nonexistent task succeeded** once the pool had 2 connections.
2. **`busy_timeout` has the same per-connection problem.** Measured: 30 concurrent `Create(ParentID=X)` calls → **25 failed with `SQLITE_BUSY`**, only 5 children persisted.
3. **Root ID collisions.** `newID()` produces `prefix-<4 hex chars>` (65,536 values) and never checks for existence. Measured: first `UNIQUE constraint failed: tasks.id` at task **#433**; 10 failures in 1,500 creates.
4. **Ladder-ID race + delete bug.** Subtask IDs use `COUNT(children)+1` (task.go:35-38) with no transaction: concurrent creators compute the same ID, and deleting a *non-last* child makes the next `COUNT+1` collide with a live sibling (children `.1`,`.2`; delete `.1`; next ID is `.2` → collision).
5. **No transactions; events can lie.** `Update()` writes event rows *before* the UPDATE executes (task.go:132-191 vs 201-206) — a failed UPDATE leaves phantom "updated" events. `CloseTask`+`autoUnblock`, `CancelTask` cascade, `DeleteTask` recursion, and `Claim` are each several statements with no atomicity.
6. **Unstable ordering.** `timeFormat = time.RFC3339` (util.go:8) has **1-second** precision; `Events()` orders by that string (event.go:12), so events in the same second come back in arbitrary order.

## Goal

After this change: FK enforcement and busy_timeout apply to **every** connection; each public mutation is **one atomic transaction**; ID generation never collides; events are recorded iff the mutation committed, and are emitted to listeners only **after** commit; event ordering is stable. Public SDK signatures are unchanged.

## Files to touch

| File | What changes |
|---|---|
| `store.go` | DSN-based pragmas in `Open()`; `withTx` helper; `dbq` interface; `insertEvent`; event buffer |
| `task.go` | Wrap `Create`, `Update`, `UpdateStatus`, `CloseTask`, `CancelTask`, `Reopen`, `Claim`, `DeleteTask`, `autoUnblock`, `autoProgressParent` in transactions; new `generateTaskID` |
| `query.go` | tx-capable variants of `Get`/`Children` scan paths (`getTask(q dbq, id)`, `childrenIn(q dbq, id)`) |
| `dependency.go` | tx-capable `listDependenciesIn`/`listDependentsIn` (used by `autoUnblock`) |
| `event.go` | `Events`/`EventsSince` order by `id`, not `timestamp` |
| `comment.go` | `ListComments` order tiebreak `created_at ASC, rowid ASC` |
| `util.go` | `timeFormat` → fixed-width milliseconds |
| `export.go` | `ImportJSONL`: replace pool-wide `PRAGMA foreign_keys=OFF` with `defer_foreign_keys` in a tx (**required**, see step 7) |
| `store_test.go`, `task_test.go`, `event_test.go` | new regression tests |
| `docs/architecture.md` | update "Store Lifecycle" + "Data Flow: Mutation" sections |

## Implementation order

### Step 1 — per-connection pragmas via DSN (store.go `Open`, lines 78-126)

Replace `sql.Open("sqlite", dbPath)` and delete the three `db.Exec("PRAGMA ...")` blocks (lines 91-106):

```go
dsn := "file:" + filepath.ToSlash(dbPath) +
    "?_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_txlock=immediate"
db, err := sql.Open("sqlite", dsn)
if err != nil {
    return nil, fmt.Errorf("open database: %w", err)
}
if err := db.Ping(); err != nil {
    db.Close()
    return nil, fmt.Errorf("connect database: %w", err)
}
```

This exact DSN was verified against `modernc.org/sqlite v1.46.1` on this repo: with it, the bogus-FK insert test failed 0/100 times (correct) and 30 concurrent inserts had 0 errors. Notes:
- Keep `busy_timeout` **first** in the param list (applied in order; `journal_mode(WAL)` can itself hit a locked DB).
- `_txlock=immediate` makes `db.Begin()` take the SQLite write lock at BEGIN. This is what makes SELECT-then-INSERT inside a transaction race-free across processes.
- `sql.Open` is lazy — a DSN typo surfaces at first use, hence the `Ping()`.
- Keep `migrate.Run(db)` exactly where it is.

### Step 2 — transaction plumbing (store.go)

```go
// dbq is the query interface satisfied by both *sql.DB and *sql.Tx.
type dbq interface {
    Exec(query string, args ...any) (sql.Result, error)
    Query(query string, args ...any) (*sql.Rows, error)
    QueryRow(query string, args ...any) *sql.Row
}

// eventBuffer collects events during a transaction; they are written in the
// same tx and emitted to listeners only after commit.
type eventBuffer struct{ events []Event }

func (b *eventBuffer) add(taskID string, et EventType, actor, field, oldV, newV string) {
    b.events = append(b.events, Event{TaskID: taskID, Type: et, Actor: actor,
        Field: field, OldValue: oldV, NewValue: newV, Timestamp: timeNowUTC()})
}

func insertEvent(q dbq, e Event) error {
    _, err := q.Exec(
        `INSERT INTO events (task_id, event_type, actor, field, old_value, new_value, timestamp)
         VALUES (?, ?, ?, ?, ?, ?, ?)`,
        e.TaskID, string(e.Type), e.Actor, e.Field, e.OldValue, e.NewValue,
        e.Timestamp.Format(timeFormat))
    return err
}

func (s *Store) withTx(fn func(tx *sql.Tx, ev *eventBuffer) error) error {
    tx, err := s.db.Begin()
    if err != nil {
        return fmt.Errorf("begin transaction: %w", err)
    }
    ev := &eventBuffer{}
    if err := fn(tx, ev); err != nil {
        tx.Rollback()
        return err
    }
    for _, e := range ev.events {
        if err := insertEvent(tx, e); err != nil {
            tx.Rollback()
            return fmt.Errorf("record event: %w", err)
        }
    }
    if err := tx.Commit(); err != nil {
        return fmt.Errorf("commit transaction: %w", err)
    }
    for _, e := range ev.events {
        s.emit(e)
    }
    return nil
}
```

Keep the existing `recordEvent` for now but have it delegate: `insertEvent(s.db, e)` + `s.emit(e)`; log the insert error via the returned error path where callers can (it currently swallows with `_, _ =`). By the end of this plan **no mutating path should call `recordEvent` anymore** — everything goes through `eventBuffer`. `AddComment`, `AddCheckpoint`, `SetAttr`, `DeleteAttr`, `AddDependency`, `RemoveDependency` are single-statement writes; converting them to `withTx` is optional but recommended for uniformity (comment + its event become atomic).

### Step 3 — tx-capable read helpers

- In query.go: extract `Get`'s body into `func (s *Store) getTask(q dbq, id string) (*Task, error)`; `Get` becomes `return s.getTask(s.db, id)`. Same for `Children` → `childrenIn(q dbq, id string)`.
- In dependency.go: `queryDeps` already takes `(query, id)`; add a `q dbq` first parameter and thread it through `ListDependencies`/`ListDependents` (public methods pass `s.db`) plus internal `listDependenciesIn(q, id)` / `listDependentsIn(q, id)`.

### Step 4 — `Create` (task.go:10-79): atomic + collision-proof IDs

Replace the ID-generation block and INSERT with one `withTx`:

```go
func (s *Store) generateTaskID(q dbq, parentID string) (string, error) {
    if parentID == "" {
        for attempt := 0; attempt < 10; attempt++ {
            id := s.newID()
            var n int
            if err := q.QueryRow("SELECT COUNT(*) FROM tasks WHERE id = ?", id).Scan(&n); err != nil {
                return "", fmt.Errorf("check id uniqueness: %w", err)
            }
            if n == 0 {
                return id, nil
            }
        }
        return "", fmt.Errorf("could not generate a unique task ID after 10 attempts; increase hash_length (gig config set hash_length %d)", s.hashLen+1)
    }
    // Ladder ID: MAX(numeric suffix)+1 — NOT COUNT+1 (deleted children, races).
    rows, err := q.Query("SELECT id FROM tasks WHERE parent_id = ?", parentID)
    if err != nil {
        return "", fmt.Errorf("list sibling ids: %w", err)
    }
    maxN := 0
    var ids []string
    for rows.Next() {
        var id string
        if err := rows.Scan(&id); err != nil {
            rows.Close()
            return "", fmt.Errorf("scan sibling id: %w", err)
        }
        ids = append(ids, id)
    }
    rows.Close()
    if err := rows.Err(); err != nil {
        return "", err
    }
    for _, id := range ids {
        suffix := strings.TrimPrefix(id, parentID+".")
        if n, err := strconv.Atoi(suffix); err == nil && n > maxN {
            maxN = n
        }
    }
    return fmt.Sprintf("%s.%d", parentID, maxN+1), nil
}
```

Inside `Create`'s `withTx` body: validate parent exists (via `q`), call `generateTaskID(tx, p.ParentID)`, INSERT via `tx`, `ev.add(id, EventCreated, p.CreatedBy, "", "", p.Title)`. Because `_txlock=immediate` serializes writers from BEGIN, the SELECT-then-INSERT pair cannot interleave with another writer.

### Step 5 — `Update` (task.go:123-207): events only for committed changes

Wrap the whole body in `withTx`. Read the task via `getTask(tx, id)`. Replace every `s.recordEvent(...)` with `ev.add(...)` (same arguments). Keep the sets/args building identical. After the UPDATE exec, re-read via `getTask(tx, id)` and return that. The "nothing to update" early return stays (return before any event is buffered).

### Step 6 — remaining mutations

Convert each to `withTx`, using the `q`-variants inside, and `ev.add` instead of `recordEvent`:

- `UpdateStatus`: read via `getTask(tx,...)`, update, `ev.add`, `autoProgressParent(tx, ev, ...)`.
- `CloseTask`: read, `childrenIn(tx, id)` guard, UPDATE, `ev.add`, `autoUnblock(tx, ev, id, actor)`.
- `CancelTask`: **SQLite has no nested transactions.** Extract `cancelInTx(tx *sql.Tx, ev *eventBuffer, id, reason, actor string) error` holding today's logic (including recursion into children via `cancelInTx`, and `autoUnblock(tx, ...)`); public `CancelTask` is one `withTx` around the root call. The recursive cascade must NOT call the public `CancelTask`.
- `DeleteTask`: same pattern — `deleteInTx(tx, id, actor)` recursion; buffer the final in-memory-only `EventDeleted` emit for after commit (it has no DB row — keep using `s.emit` after `withTx` returns nil, not `ev.add`, because `ev` entries are inserted into the events table and the task row is gone → FK would reject it).
- `Claim`: read + UPDATE + events in one tx. (If PLAN-Status-Transition-Integrity has already landed and made Claim a compare-and-set UPDATE, keep its `WHERE` clause and just wrap it; these two plans touch the same functions — **execute them sequentially, never in parallel worktrees**.)
- `Reopen`: single UPDATE + event in one tx.
- `autoUnblock(q dbq, ev *eventBuffer, closedID, actor string) error`: replace `s.db` with `q`, `recordEvent` with `ev.add`. **Collect dependent IDs into a slice and close the rows before executing UPDATEs** — on a `*sql.Tx`, running `Exec` while a `Rows` from the same tx is still open blocks (database/sql serializes tx usage). Same for `autoProgressParent(q, ev, parentID, actor) bool`.

### Step 7 — keep `ImportJSONL` working (export.go:78-91)

This step is **mandatory**, not optional: today's import "works" only because FK enforcement is broken. Once step 1 enforces FKs on every connection, `PRAGMA foreign_keys=OFF` via the pool no longer covers the insert statements (they may run on other connections), and child-before-parent upserts would fail. Replace lines 78-82 with a transaction:

```go
tx, err := s.db.Begin()
if err != nil { return fmt.Errorf("begin import: %w", err) }
if _, err := tx.Exec("PRAGMA defer_foreign_keys=ON"); err != nil {
    tx.Rollback()
    return fmt.Errorf("defer foreign keys: %w", err)
}
for _, task := range tasks {
    if err := s.upsertTask(tx, &task); err != nil {   // change upsertTask to take q dbq
        tx.Rollback()
        return fmt.Errorf("upsert task %s: %w", task.ID, err)
    }
}
if err := tx.Commit(); err != nil { return fmt.Errorf("commit import: %w", err) }
```

`defer_foreign_keys` is per-transaction, auto-resets at COMMIT, checks constraints at commit time (verified on this repo: child-before-parent inside the tx commits fine; a truly dangling reference fails at commit with "FOREIGN KEY constraint failed"). Delete the loop variable aliasing bug risk: `for _, task := range tasks` + `&task` is fine on Go ≥1.22 (this module is on go 1.26).

### Step 8 — timestamps and ordering

- util.go: `const timeFormat = "2006-01-02T15:04:05.000Z07:00"` (fixed-width milliseconds — lexicographically sortable, and `strToTime`'s `time.Parse(time.RFC3339, ...)` parses fractional seconds; change the parse layout constant to a separate `const timeParseFormat = time.RFC3339` if you renamed things, but the existing `strToTime` body needs **no** change).
- event.go:12: `ORDER BY timestamp ASC` → `ORDER BY id ASC`. event.go:26 (`EventsSince`): keep the `WHERE timestamp > ?` but `ORDER BY id ASC`.
- comment.go:42: `ORDER BY created_at ASC` → `ORDER BY created_at ASC, rowid ASC`.

### Step 9 — tests (add to task_test.go / store_test.go / event_test.go)

```
TestCreateManyUniqueIDs            — 1,500 sequential Create() calls, assert zero errors (regression: failed at #433 before).
TestConcurrentSubtaskCreates       — 30 goroutines Create(ParentID=p); assert 30 children, all IDs distinct (before: 5/30).
TestSubtaskIDAfterMiddleDelete     — create p, p.1, p.2; DeleteTask(p.1); Create(ParentID=p) succeeds and returns "p.3".
TestForeignKeysEnforcedAcrossPool  — hold 8 concurrent Query results open to force pool growth, then 100 raw inserts via store.DB() with task_id='no-such-task': all 100 must error.
TestEventsInsertionOrdered         — 3 UpdateStatus calls in one second; Events() returns them in call order.
TestListenerFiresAfterCommit       — register On(EventStatusChanged) listener that immediately calls store.Get(taskID) and asserts it sees the NEW status (proves emit-after-commit and no held write lock).
```

### Step 10 — docs

- docs/architecture.md "Store Lifecycle": pragmas now via DSN so they apply to every pooled connection. "Data Flow: Mutation": `validate → BEGIN IMMEDIATE → SQL + event rows → COMMIT → emit callbacks → hooks`.

## Edge cases a weaker model would miss

1. **Pragmas via `db.Exec` are per-connection** — the whole point of step 1. Do not "fix" this by re-running `PRAGMA` before each query.
2. **Emit listeners only after COMMIT.** A listener may call back into the Store; inside the tx that would run on another pooled connection and block against our own write lock until busy_timeout expires, then fail.
3. **No nested transactions in SQLite.** Cascading code (`CancelTask`, `DeleteTask`) must recurse via `...InTx` helpers, never via the public methods.
4. **Don't `Exec` on a tx while its `Rows` are open** (database/sql blocks). Read ID lists fully, close, then write — relevant in `autoUnblock`, `generateTaskID`, cascades.
5. **Ladder IDs: `MAX(suffix)+1`, not `COUNT+1`** — count breaks after deleting any non-last child (pre-existing bug, test in step 9). Ignore non-numeric suffixes (reparented/imported tasks may not match the ladder pattern).
6. **`EventDeleted` cannot be buffered into the events table** — the task row is gone and events.task_id has an FK. Keep it emit-only.
7. **`_txlock=immediate` means `withTx` takes a write lock even for reads** — never use `withTx` for read-only paths; plain `s.db` queries stay lock-free under WAL.
8. **RFC3339 mixed precision breaks lexicographic order** (`"...00Z"` sorts *after* `"...00.500Z"` because `'Z' > '.'`). That's why events order by `id` and the format is fixed-width. Do not use `time.RFC3339Nano` (variable width).
9. **Step 7 is load-bearing**: skipping it leaves `gig import` broken once FKs are actually enforced. The existing test `TestCLI_ExportImport` (cmd/gig/cli_test.go:559) will catch it — run e2e.
10. **Do not change the default `hash_length`** — collision retry makes 4 chars fine; changing defaults would churn user-visible IDs.

## Acceptance criteria

Run all of:

```bash
go build ./... && go vet ./...
go test ./... -count=1                       # all pass, incl. the 6 new tests
go test -tags=e2e -v -count=1 ./cmd/gig/     # all pass (esp. TestCLI_ExportImport)
grep -n "PRAGMA journal_mode\|PRAGMA busy_timeout\|PRAGMA foreign_keys=ON" store.go   # no matches
grep -rn "recordEvent(" task.go              # no matches (all via eventBuffer)
```

Behavioral checks (each is a new test from step 9, verifiable individually with `go test -run <Name> -count=1 .`):
- 1,500 sequential creates: 0 failures.
- 30 concurrent subtask creates: 30 children, unique IDs.
- Bogus-FK inserts: 0/100 succeed.
- `p.1`,`p.2` → delete `p.1` → next child is `p.3`.
- Events for same-second mutations return in insertion order.

## Out of scope

- Status-transition semantics (PLAN-Status-Transition-Integrity).
- Multi-file export fidelity (PLAN-Full-Fidelity-Sync) — only the FK-deferral fix to keep today's import green.
- Parent-cycle validation (PLAN-Hierarchy-Integrity).
