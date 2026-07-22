# PLAN: Status-Transition Integrity

**Rank: 2 of 5. Do after PLAN-Concurrency-Safe-Storage (it introduces the `withTx`/`eventBuffer` plumbing this plan reuses; if executed standalone, use direct `s.db` calls where that plumbing is referenced — the logic here is independent).**

## Problem (all verified on this codebase, 2026-07-07, commit `08c7a13`)

`Store.UpdateStatus` (task.go:212) performs a raw status write with none of the lifecycle side effects, and it's reachable from **two production surfaces**: `gig update <id> --status ...` (cmd/gig/task_cmds.go:213-235) and the web UI's drag-and-drop (ui/server.go:363-379). Verified consequences:

- `UpdateStatus(id, closed)`: **`closed_at` stays NULL, `close_reason` empty, dependents stay `blocked` forever** (auto-unblock never fires), and a parent **with open children closes successfully** — `CloseTask` would refuse.
- `UpdateStatus(id, cancelled)`: no cascade to children (`CancelTask` cascades), no auto-unblock, no `closed_at`.
- `UpdateStatus(closedTask, open)`: `closed_at`/`close_reason` remain set on an open task (`Reopen` clears them).
- `Reopen` (task.go:403) records the event's old value as hardcoded `StatusClosed` even when the task was `cancelled` — audit log lies.
- `Claim` (task.go:458) is get-then-blind-UPDATE: two agents claim the same task and the loser never finds out; a `blocked` or `deferred` task can be silently claimed into `in_progress`. Roadmap v0.4.0 explicitly wants "Claim locking: prevent double-claim with atomic check-and-set".
- `RemoveDependency` (dependency.go:54) never re-evaluates the dependent: remove the last blocker of a `blocked` task and it stays `blocked` forever (auto-unblock only triggers on close/cancel of a blocker).

## Goal

One guarded path for every status change. `UpdateStatus` becomes a dispatcher that delegates terminal transitions to `CloseTask`/`CancelTask`, applies reopen semantics when leaving a terminal state, and rejects terminal→terminal. `Claim` becomes an atomic compare-and-set with a `--force` escape hatch. Removing a dependency can unblock. CLI and web UI inherit all of it with no changes to their call sites.

## Files to touch

| File | What changes |
|---|---|
| `task.go` | `UpdateStatus` dispatcher; `Reopen` old-value fix; `Claim` CAS + new `ClaimForce`; shared `maybeUnblock` helper |
| `dependency.go` | `RemoveDependency` calls `maybeUnblock` |
| `cmd/gig/task_cmds.go` | `--force` flag on `update` (claim path); error text passthrough |
| `task_test.go`, `dependency_test.go` | new unit tests |
| `cmd/gig/cli_test.go` | 2 new e2e tests |
| `CLAUDE.md` | status-model section: note that closed/cancelled via any path get full semantics |
| `docs/sdk-reference.md` | `Claim`/`ClaimForce` contract |

## Implementation order

### Step 1 — extract `maybeUnblock` (task.go)

Pull the inner loop of `autoUnblock` (task.go:333-370) into a reusable helper (post-Plan-1 signature shown; standalone, drop `q`/`ev` and use `s.db`/`s.recordEvent`):

```go
// maybeUnblock transitions a blocked task to open if it has no unresolved 'blocks' dependencies left.
func (s *Store) maybeUnblock(q dbq, ev *eventBuffer, taskID, actor string) error
```

Logic: get task; if status != `blocked`, return nil; list its `blocks` dependencies; if every blocker `IsTerminal()` (an empty list counts as resolved), UPDATE to `open` + event. `autoUnblock(closedID)` becomes: for each dependent with dep type `blocks`, call `maybeUnblock(dep.FromID)`.

### Step 2 — `UpdateStatus` dispatcher (task.go:212-243)

```go
func (s *Store) UpdateStatus(id string, status Status, actor string) error {
    if !status.IsValid() {
        return fmt.Errorf("invalid status: %s", status)
    }
    task, err := s.Get(id)
    if err != nil {
        return err
    }
    if task.Status == status {
        return nil
    }
    // Terminal → terminal is never a legal edge (closed↔cancelled).
    if task.Status.IsTerminal() && status.IsTerminal() {
        return fmt.Errorf("task %s is %s; reopen it first (gig reopen %s)", id, task.Status, id)
    }
    switch status {
    case StatusClosed:
        return s.CloseTask(id, "", actor)     // child guard + closed_at + auto-unblock
    case StatusCancelled:
        return s.CancelTask(id, "", actor)    // cascade + closed_at + auto-unblock
    }
    // Non-terminal target. Leaving a terminal state clears the close fields (reopen semantics).
    ...single UPDATE: status + updated_at, plus closed_at='' and close_reason='' when task.Status.IsTerminal()...
    ...event: status_changed old=task.Status new=status...
    if status == StatusInProgress {
        s.autoProgressParent(task.ParentID, actor)
    }
    return nil
}
```

Keep it inside one `withTx` if Plan 1 landed (the delegated `CloseTask`/`CancelTask` open their own tx — return **before** starting yours; the dispatch happens pre-transaction, exactly as written above).

### Step 3 — `Reopen` old-value fix (task.go:403)

`s.recordEvent(id, EventStatusChanged, actor, "status", string(StatusClosed), string(StatusOpen))` → use `string(task.Status)` for the old value (`task` is already loaded at line 386).

### Step 4 — `Claim` compare-and-set (task.go:458-492)

Replace the blind UPDATE with:

```go
res, err := ...Exec(
    `UPDATE tasks SET assignee = ?, status = ?, updated_at = ?
     WHERE id = ? AND status IN ('open', 'deferred')`,
    assignee, string(StatusInProgress), now.Format(timeFormat), id)
```

If `RowsAffected() == 0`, load the task and diagnose:
- not found → return the Get error ("task not found");
- terminal → `fmt.Errorf("cannot claim %s task %s", task.Status, id)` (existing message, keep for e2e compat);
- `blocked` → `fmt.Errorf("cannot claim blocked task %s: it has unresolved blockers (see gig show %s)", id, id)`;
- `in_progress` and `task.Assignee == assignee` → return `&ClaimResult{}, nil` (idempotent re-claim);
- `in_progress` and different assignee → `fmt.Errorf("task %s is already claimed by %q (use --force to take over)", id, task.Assignee)`.

Events (only when the CAS row updated): keep the two existing conditional `EventAssigned`/`EventStatusChanged` records, using the pre-read task for old values — **read the task before the UPDATE** for event old-values, but let the `WHERE` clause be the source of truth for success. Keep `autoProgressParent` on success.

Add:

```go
// ClaimForce claims regardless of current status or assignee (terminal tasks still rejected).
func (s *Store) ClaimForce(id string, assignee string) (*ClaimResult, error)
```

Same body, `WHERE id = ? AND status NOT IN ('closed','cancelled')`, and the rows==0 diagnosis reduces to not-found/terminal.

### Step 5 — `RemoveDependency` unblock (dependency.go:54-68)

After a successful delete with `rows > 0` and the removed dep's type was `blocks` (the current code doesn't know the type — SELECT `dep_type` before deleting, or `DELETE ... RETURNING dep_type`; the plain pre-SELECT is simpler), call `maybeUnblock(fromID)`. Record the existing `EventDependencyRemoved` first so the event order reads remove→unblock.

### Step 6 — CLI (cmd/gig/task_cmds.go)

- Add `var force bool` + `cmd.Flags().BoolVar(&force, "force", false, "With --claim: take over a task claimed by someone else")`.
- In the claim branch (line 198): `if force { store.ClaimForce(...) } else { store.Claim(...) }`.
- No changes to the `--status` branch — it already calls `UpdateStatus` and now inherits correct semantics. The auto-progress reporting block (lines 214-233) still works because parent auto-progress only happens on `in_progress`.

### Step 7 — tests

Unit (task_test.go / dependency_test.go):

```
TestUpdateStatusClosedDelegates        — blocker blocked-dependent setup; UpdateStatus(blocker, closed): blocker.ClosedAt != nil AND dependent.Status == open.  (Both assertions fail on current code.)
TestUpdateStatusClosedChildGuard       — parent with open child; UpdateStatus(parent, closed) returns error containing "close or cancel all children first".
TestUpdateStatusCancelledCascades      — parent+child; UpdateStatus(parent, cancelled): child.Status == cancelled.
TestUpdateStatusTerminalToTerminal     — closed task; UpdateStatus(id, cancelled) errors with "reopen it first".
TestUpdateStatusLeavingTerminalClears  — closed task; UpdateStatus(id, open): status open, ClosedAt nil, CloseReason "".
TestReopenEventOldValueCancelled       — cancel task, Reopen, last status_changed event OldValue == "cancelled".
TestClaimConflict                      — Claim(id, "a") ok; Claim(id, "b") errors containing "already claimed by \"a\"".
TestClaimIdempotentSameAssignee        — Claim(id, "a") twice; second returns nil error.
TestClaimBlockedRejected               — blocked task; Claim errors containing "unresolved blockers".
TestClaimForceTakesOver                — claimed by "a"; ClaimForce(id, "b") ok; task.Assignee == "b".
TestRemoveDependencyUnblocks           — A blocks-on B, A set blocked; RemoveDependency(A, B): A.Status == open.
```

E2E (cmd/gig/cli_test.go — use the existing helpers: `run` at line 50 fails the test on non-zero exit; `runExpectFail` at line 61 is for commands that must fail):

```
TestCLI_UpdateStatusClosedFullSemantics — create 2 tasks, dep add, update --status blocked, update blocker --status closed → gig show dependent contains "open"; gig show blocker contains "Closed:".
TestCLI_ClaimForce                      — claim as actor a (--actor a --claim), then --actor b --claim fails, then --actor b --claim --force succeeds.
```

### Step 8 — docs

- CLAUDE.md "Status Model": add "All status changes route through `UpdateStatus`, which delegates to `CloseTask`/`CancelTask` for terminal targets; terminal→terminal requires `Reopen` first."
- docs/sdk-reference.md: document `Claim` CAS semantics + `ClaimForce`.

## Edge cases a weaker model would miss

1. **Terminal→terminal must be rejected *before* delegating.** `CloseTask` early-returns nil for any terminal task and `CancelTask` only checks `== StatusCancelled` — naive delegation makes `cancelled→closed` a silent no-op and `closed→cancelled` overwrite `closed_at`. The guard in step 2 prevents both.
2. **`CloseTask`/`CancelTask` open their own transaction (post Plan 1).** The dispatcher must delegate *before* starting a tx of its own — SQLite has no nested transactions.
3. **Setting `blocked` manually stays legal** (agents use it as a flag before deps exist). Don't reject it; the stale-blocked case is handled by `maybeUnblock` on dep removal + a doctor warning (PLAN-Hierarchy-Integrity).
4. **Claim events need the pre-UPDATE task** for old values, but success is decided by `RowsAffected` — read first, CAS second, and only record events when `rows == 1`, otherwise a lost race records events for a claim that didn't happen.
5. **Idempotent re-claim must not error** — JEFF agents retry; `Claim(id, sameAssignee)` on an in_progress task returns success without touching the row (the CAS WHERE excludes `in_progress`, so detect this in the rows==0 diagnosis).
6. **Empty reason on delegation**: `UpdateStatus(id, closed)` passes `""` as reason. That matches `gig close` without `--reason`. Don't invent a default like "closed via status change" — `close_reason` is user-facing data.
7. **Web UI drag to "Closed" can now fail** (parent with open children → HTTP 400 with the child-guard message). That is intended behavior, not a regression; ui/server.go already surfaces the error body. Note it in the commit message.
8. **`gig update <id> --status open` on a blocked task** is the manual-unblock path; it must keep working (dispatcher treats it as plain non-terminal transition).
9. **Existing e2e tests use `update --status in_progress`** (cli_test.go:627,641,659,672) — plain transitions, unaffected. `TestCLI_ClaimTask` (cli_test.go:689) claims with the default actor (`cli`) on an open task and asserts `in_progress` in `gig show` — the CAS path must keep that green (open → claim succeeds).
10. **`ClaimResult.ParentProgressed`** must still be populated on the CAS path — the CLI prints "Parent %s → in_progress" from it.

## Acceptance criteria

```bash
go build ./... && go vet ./...
go test ./... -count=1                     # all pass incl. 11 new unit tests
go test -tags=e2e -v -count=1 ./cmd/gig/   # all pass incl. 2 new e2e tests
```

Manual spot-checks (fresh `GIG_HOME=$(mktemp -d)`, `gig init`):
1. `gig create A; gig create B; gig dep add <A> <B>; gig update <A> --status blocked; gig update <B> --status closed; gig show <A>` → status `open`; `gig show <B>` → shows a `Closed:` timestamp.
2. `gig create P; gig create C --parent <P>; gig update <P> --status closed` → error mentioning the child.
3. `gig update <X> --claim --actor a` then `gig update <X> --claim --actor b` → non-zero exit, message names `a`; add `--force` → succeeds.
4. `gig cancel <X>; gig update <X> --status closed` → error "reopen it first".

## Out of scope

- Transaction plumbing itself (Plan 1).
- Ancestor-chain readiness and stale-blocked doctor checks (PLAN-Hierarchy-Integrity).
- Any new CLI subcommands — only the `--force` flag.
