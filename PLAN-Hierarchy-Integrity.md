# PLAN: Hierarchy Integrity

**Rank: 4 of 5.** Independent of the other plans (shares `task.go`/`query.go` with Plans 1-2 — execute sequentially on one branch, order among them doesn't matter for logic).

## Problem (verified on this codebase, 2026-07-07, commit `08c7a13`)

1. **Parent cycles are accepted and then crash the CLI.** `Update` (task.go:160-173) validates only `*p.ParentID == id` (direct self-parent). Reparenting A under its own descendant was verified to succeed: `Create A; Create B (parent A); Update(A, ParentID=B)` → no error, DB now holds A→B→A. After that, `GetTree` (query.go:120-140) recurses forever → stack overflow. `gig show <id>` calls `GetTree` (cmd/gig/task_cmds.go:166), so one innocent `gig update --parent` bricks `gig show`, `gig list --tree`, and the web UI detail page for that subtree.
2. **`Ready(parentID)` misses reparented descendants.** Scoping uses `t.parent_id = ? OR t.id LIKE ?||'.%'` (query.go:176-177), which assumes ladder-notation IDs. A task created at root (`gig-xxxx`) and later reparented under P keeps its root-style ID: it matches neither clause at depth ≥ 2, so `gig ready --id P` silently omits real descendants.
3. **`Ready` gates on the parent only, not the ancestor chain** (query.go:158-171). Grandparent blocked/with-unresolved-blockers → grandchild still reported "ready". (Direct children are correctly gated; depth ≥ 2 is the hole. `cancelled` cascades so it self-heals, but `blocked` and dependency-gated ancestors don't.)
4. **`Doctor` can't see any of this** (doctor.go) — no checks for hierarchy cycles, terminal parents with active children, or blocked tasks with no blockers.
5. Small "never swallow errors" violations in the same area: `GetFull` ignores the `Attrs` error (task.go:96-98: `return task, nil`); `LatestCheckpoint` returns `nil, nil` on *any* error, not just no-rows (checkpoint.go:96-98); `DeleteTask`'s doc comment claims "Events are preserved as an audit trail" while the code deletes them (task.go:409 vs 427-431).

## Goal

Parent cycles are impossible to create; tree traversal survives pre-existing bad data; `Ready` is correct for any hierarchy shape regardless of ID format; `gig doctor` detects legacy corruption; the three error-handling lies are fixed.

## Files to touch

| File | What changes |
|---|---|
| `task.go` | cycle guard in `Update`; `GetFull` error propagation; `DeleteTask` doc comment |
| `query.go` | `GetTree` visited-guard; `Ready` rewrite (Go-side ancestor gating) |
| `checkpoint.go` | `LatestCheckpoint` distinguishes `sql.ErrNoRows` from real errors |
| `doctor.go` | 3 new checks |
| `task_test.go`, `query`-related tests in `task_test.go`, `doctor_test.go`, `checkpoint_test.go` | new tests |
| `cmd/gig/cli_test.go` | 1 e2e test |
| `docs/architecture.md` | Ready() semantics note |

## Implementation order

### Step 1 — cycle guard (task.go, inside `Update`'s ParentID branch, after the existing `*p.ParentID == id` check at line 164)

```go
// wouldCreateParentCycle reports whether setting childID's parent to newParentID
// would make childID its own ancestor. The walk is bounded by a visited set so it
// terminates even if the DB already contains a cycle (legacy data).
func (s *Store) wouldCreateParentCycle(childID, newParentID string) (bool, error) {
    seen := make(map[string]bool)
    cur := newParentID
    for cur != "" {
        if cur == childID {
            return true, nil
        }
        if seen[cur] {
            return false, nil // pre-existing cycle above the insertion point; not ours
        }
        seen[cur] = true
        var parent sql.NullString
        err := s.db.QueryRow("SELECT parent_id FROM tasks WHERE id = ?", cur).Scan(&parent)
        if err == sql.ErrNoRows {
            return false, nil
        }
        if err != nil {
            return false, fmt.Errorf("walk ancestors of %s: %w", newParentID, err)
        }
        cur = ""
        if parent.Valid {
            cur = parent.String
        }
    }
    return false, nil
}
```

In `Update`: after `validateTaskExists`, call it; on `true` return `fmt.Errorf("cannot set parent of %s to %s: %s is a descendant of %s", id, *p.ParentID, *p.ParentID, id)`. (Post Plan 1 this runs inside `withTx` — pass the `q dbq` through instead of `s.db`.)

### Step 2 — `GetTree` defense-in-depth (query.go:120-140)

Primary protection is step 1, but old DBs may already contain cycles. Add a visited set:

```go
func (s *Store) GetTree(id string) (*Task, error) {
    return s.getTreeVisited(id, make(map[string]bool))
}

func (s *Store) getTreeVisited(id string, visited map[string]bool) (*Task, error) {
    task, err := s.Get(id)
    if err != nil { return nil, err }
    if visited[id] {
        return task, nil // cycle in stored data: return the node, don't descend again
    }
    visited[id] = true
    children, err := s.Children(id)
    ...unchanged recursion, but call s.getTreeVisited(child.ID, visited)...
}
```

### Step 3 — `Ready` rewrite (query.go:146-189)

Keep the first SQL filter (open + no own unresolved blockers), drop the parent-gating SQL and the `LIKE` scoping. Gate ancestors in Go with memoization:

```go
func (s *Store) Ready(parentID string) ([]*Task, error) {
    // 1) candidates: open tasks with no unresolved own blockers (existing first NOT EXISTS
    //    subquery, ORDER BY priority ASC, updated_at DESC — preserve this ordering).
    // 2) load lookup tables in two queries:
    //    SELECT id, parent_id, status FROM tasks               → map[id]struct{parent string; status Status}
    //    SELECT d.from_id, b.status FROM dependencies d JOIN tasks b ON b.id = d.to_id
    //      WHERE d.dep_type = 'blocks' AND b.status NOT IN ('closed','cancelled')
    //                                                          → set of task IDs with unresolved blockers
    // 3) gated(id) with memo map[string]bool: walk parent chain (visited set for cycle safety);
    //    an ancestor gates its subtree if ancestor.status ∈ {blocked, closed, cancelled}
    //    OR ancestor has unresolved blockers. The candidate itself is NOT an "ancestor".
    // 4) scope: if parentID != "", keep a candidate only if parentID appears in its ancestor chain.
    // 5) return candidates in the order produced by query (1).
}
```

Explicitly: gating statuses for ancestors are `blocked`, `closed`, `cancelled` — matching the current SQL (query.go:162). `deferred` ancestors do **not** gate (unchanged semantics). Root tasks (`parent_id` NULL *or* `''` — both exist in real DBs, see import) terminate the walk.

Two behavior fixes fall out and must be asserted in tests: reparented descendants are now included in `Ready(P)`; grandchildren of blocked grandparents are now excluded.

### Step 4 — error-handling truth (three small fixes)

- task.go:96-98 (`GetFull`): `if err != nil { return nil, fmt.Errorf("load attrs for %s: %w", id, err) }`.
- checkpoint.go:96-98 (`LatestCheckpoint`): `if err == sql.ErrNoRows { return nil, nil }` / `if err != nil { return nil, fmt.Errorf("latest checkpoint: %w", err) }` (add `database/sql` import).
- task.go:407-409 (`DeleteTask` doc): rewrite comment to the truth: "Comments, dependencies, attributes and checkpoints are removed via CASCADE; event rows are deleted explicitly (the events FK has no cascade). Listeners receive an EventDeleted emit only — no event row can exist for a deleted task."

### Step 5 — doctor checks (doctor.go; register in `Doctor()` after `checkDependencyCycles`)

```go
func (s *Store) checkHierarchyCycles(r *DoctorReport)
// Load all (id, parent_id). For each id walk the parent chain with a visited set
// (share the walk logic conceptually with wouldCreateParentCycle but over the in-memory
// map — one query, not N). If a walk revisits the starting id → cycle; collect up to 10
// offender ids. Warn: "N task(s) in a parent cycle: id1, id2, ... (fix: gig update <id> --orphan)"

func (s *Store) checkTerminalParentsWithActiveChildren(r *DoctorReport)
// SELECT COUNT(*) FROM tasks c JOIN tasks p ON c.parent_id = p.id
//  WHERE p.status IN ('closed','cancelled') AND c.status NOT IN ('closed','cancelled')
// Warn with count. (Legacy data from the pre-PLAN-Status-Transition UpdateStatus bypass.)

func (s *Store) checkBlockedWithoutBlockers(r *DoctorReport)
// SELECT COUNT(*) FROM tasks t WHERE t.status = 'blocked' AND NOT EXISTS (
//   SELECT 1 FROM dependencies d JOIN tasks b ON b.id = d.to_id
//   WHERE d.from_id = t.id AND d.dep_type = 'blocks' AND b.status NOT IN ('closed','cancelled'))
// Warn: "N blocked task(s) have no unresolved blockers (fix: gig update <id> --status open)"
```

Each check follows the existing add(DiagOK/DiagWarn/...) pattern with a distinct check name (`hierarchy_cycles`, `terminal_parents`, `stale_blocked`) so `--json` consumers can key on them.

### Step 6 — tests

Unit:

```
TestUpdateParentCycleDirect        — A, B(parent A); Update(A, ParentID=B.ID) errors containing "descendant".  (Passes err==nil today — this is the regression test.)
TestUpdateParentCycleDeep          — A → B → C; Update(A, ParentID=C.ID) errors.
TestUpdateParentValidMove          — A→B and separate C; Update(B, ParentID=C.ID) succeeds (guard must not over-reject).
TestGetTreeSurvivesStoredCycle     — force a cycle via store.DB().Exec("UPDATE tasks SET parent_id=? WHERE id=?", b, a) bypassing the guard; GetTree(a) returns within the test timeout and contains each node at most once per branch.
TestReadyIncludesReparentedDescendant — P; X created at ROOT then Update(X, ParentID=P.ID); Y created under X. Ready(P.ID) contains both X and Y.  (Y is missed today.)
TestReadyExcludesBlockedGrandparent   — G(blocked) → P(open) → C(open). Ready("") must not contain C.  (Contains it today.)
TestReadyRootUnaffected               — sanity: plain root task with no deps still ready (guards the NULL-vs-'' parent handling).
TestDoctorHierarchyCycle              — forced cycle via DB(); Doctor reports warn level for check "hierarchy_cycles".
TestDoctorStaleBlocked                — task set blocked with no deps; warn on "stale_blocked".
TestDoctorTerminalParent              — close a parent via raw SQL with an open child; warn on "terminal_parents".
TestLatestCheckpointNoRows            — fresh task: LatestCheckpoint → (nil, nil).
TestGetFullPropagatesError            — optional; if awkward to force, assert instead that GetFull returns attrs correctly and rely on vet/review for the signature change.
```

E2E:

```
TestCLI_UpdateParentCycleRejected — create parent+child via CLI; `gig update <parent> --parent <child>` exits non-zero with "descendant" in stderr; `gig show <parent>` still works afterwards.
```

Existing tests that must keep passing unchanged: `TestCLI_ReadyExcludesBlockedParentChildren`, `TestCLI_ReadyExcludesCancelledParentChildren`, `TestCLI_ReadyTreeView`, and the SDK `Ready`/`Blocked` tests in task_test.go — the rewrite must be behavior-compatible for depth-1 cases.

## Edge cases a weaker model would miss

1. **The cycle guard itself must terminate on cyclic input.** If the DB already has A→B→A and someone reparents C under A, a naive "walk to root" loops forever. The `seen` set in step 1 is not optional.
2. **Root detection is `parent_id IS NULL` *or* `''`** — `Create` writes NULL but the current importer writes `''` (export.go:149 binds the raw string). Handle both in the Go walk (`sql.NullString` + empty-string check), like `List` already does (query.go:39).
3. **`Ready`'s result order is part of the contract** — `priority ASC, updated_at DESC` (query.go:180). Filtering in Go must preserve the SQL result order; don't re-sort, don't build from maps (map iteration order would scramble it).
4. **The candidate itself being `open` with no blockers is already ensured by query (1)** — the ancestor gate walks strictly *above* the candidate. Applying the gate to the candidate itself would be a no-op for statuses but would wrongly re-check its own blockers... it wouldn't break, but keep the loop starting at `parent(candidate)` so the semantics stay obvious.
5. **`GetTree`'s visited map is shared across the whole recursion**, not per-branch. A diamond isn't possible with single-parent hierarchy, so "already visited ⇒ cycle ⇒ stop descending" is safe; returning the node without children (rather than erroring) keeps `gig show` usable on corrupt DBs, which is the point.
6. **Doctor checks run on potentially huge tables** — `checkHierarchyCycles` must be one `SELECT id, parent_id FROM tasks` + in-memory walk with a global `resolved` memo (O(n)), not a per-task DB walk.
7. **Don't run cycle detection inside `Ready`'s hot path beyond the visited set** — the memoized gate map already guarantees O(n) total.
8. **`Update` with `Orphan: true` bypasses the ParentID branch** (task.go:157-159) and needs no guard — orphaning can never create a cycle. Don't add the check there.
9. **Reparenting under one's own former ID prefix is legal**: after `Update(X, ParentID=P)`, X may be `gig-ab12` under `P=gig-cd34`. Nothing may assume `strings.HasPrefix(child.ID, parent.ID)` — the old `LIKE` clause was exactly that mistake.
10. **e2e stderr**: `run()` in cli_test.go fails the test on non-zero exit — use the existing `runExpectFail` helper (cli_test.go:61) for the rejection test.

## Acceptance criteria

```bash
go build ./... && go vet ./...
go test ./... -count=1                       # incl. ~11 new tests
go test -tags=e2e -v -count=1 ./cmd/gig/     # incl. TestCLI_UpdateParentCycleRejected
```

Manual (fresh GIG_HOME):
1. `gig create A; gig create B --parent <A>; gig update <A> --parent <B>` → non-zero exit, message names the descendant relationship; `gig show <A>` still renders.
2. Force a legacy cycle with `sqlite3` (or skip); `gig show <A>` completes; `gig doctor` warns `hierarchy_cycles`.
3. `gig create P; gig create X; gig update <X> --parent <P>; gig create Y --parent <X>; gig ready --id <P>` lists X and Y.
4. `gig update <T> --status blocked` (no deps) → `gig doctor` warns `stale_blocked`.

## Out of scope

- Status transition semantics (Plan 2) — the doctor checks here only *detect* what Plan 2 *prevents*.
- Renumbering ladder IDs after reparenting (IDs are immutable identifiers, not paths — document only).
- Doctor auto-fix (`gig doctor --fix`) — future idea, list it in roadmap if desired.
