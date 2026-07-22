# PLAN: Full-Fidelity Sync (roadmap v0.3.0)

**Rank: 3 of 5.** This is the roadmap's declared next milestone (docs/roadmap.md "v0.3.0 — Sync Repo Integration"), and the current behavior is silently lossy: docs/architecture.md promises "On a new machine: clone sync repo → `gig import` → DB rebuilt", but `ExportJSONL` exports **tasks only**. Dependencies (the product's headline feature), comments, checkpoints, and custom attributes are all dropped on that documented round-trip. The `sync_repo` config key and `gig config set sync_repo` already exist (config.go:16, cmd/gig/config_cmd.go:91) — the feature behind them doesn't.

**Ordering**: independent of Plans 1-2 in logic, but it touches `export.go`, which Plan 1's step 7 also touches. Execute after Plan 1 (sequentially, same branch). If Plan 1 has landed, imports here must use the `defer_foreign_keys`-in-tx pattern (see step 3) — never `PRAGMA foreign_keys=OFF` on the pool, which does not reliably apply (verified: pooled connections ignore it).

## Goal

1. `gig sync` exports **all** entities (tasks, dependencies, comments, checkpoints, attribute definitions, attribute values, events) as deterministic JSONL files.
2. Import restores all of them, with **last-write-wins on `updated_at`** for tasks and attribute values (roadmap: "Conflict resolution: last-write-wins based on updated_at during import").
3. With `sync_repo` set: `gig sync` = export→commit→push; `gig sync --pull` = pull→import; `gig sync --status` = ahead/behind/dirty report.

## Files to touch

| File | What changes |
|---|---|
| `sync.go` (**new**, root package) | `SyncExportAll`, `SyncImportAll`, `ImportStats`, git helpers `SyncPush`, `SyncPull`, `SyncStatus` |
| `export.go` | export helpers for each entity; `upsertTask` gains `q dbq` param + NULL-parent fix; `ImportJSONL` delegates to shared LWW upsert |
| `cmd/gig/sync_cmds.go` | `--pull`, `--status` flags; sync_repo wiring; keep `export`/`import` commands compatible |
| `config.go` | expand `~` in `SyncRepo` when loading |
| `sync_test.go` (**new**), `export_test.go` | unit tests |
| `cmd/gig/cli_test.go` | e2e git round-trip test |
| `docs/architecture.md`, `docs/roadmap.md`, `README.md` | document the format + check the v0.3.0 boxes |

## File format (additive — tasks.jsonl stays byte-compatible)

CLAUDE.md rule: "Don't break JSONL format without a major version bump." So: **do not change tasks.jsonl's shape** (still one `Task` JSON per line, sorted by ID, no attrs embedded). Add sibling files, each deterministic:

| File | Line type | Sort key |
|---|---|---|
| `tasks.jsonl` | `gig.Task` (unchanged) | ID |
| `deps.jsonl` | `gig.Dependency` | from_id, then to_id |
| `comments.jsonl` | `gig.Comment` | ID |
| `checkpoints.jsonl` | `gig.Checkpoint` (Files field inline) | ID |
| `attr_defs.jsonl` | `gig.AttrDefinition` | Key |
| `attrs.jsonl` | `gig.Attribute` | task_id, then key |
| `events.jsonl` | `gig.Event` (already exists via `ExportEvents`) | id |

All entity structs already have complete JSON tags (gig.go) — reuse them; do not invent new DTOs.

## Implementation order

### Step 1 — entity exports (export.go or sync.go)

One generic helper writes any slice as JSONL (mirror `ExportJSONL`'s encoder setup: `SetEscapeHTML(false)`). Needed listing queries that don't exist yet — add unexported SQL, all `SELECT ... ORDER BY <sort key>`:

- all dependencies: `SELECT from_id, to_id, dep_type, created_at FROM dependencies ORDER BY from_id, to_id` (reuse `queryDeps` scan).
- all comments: `SELECT ... FROM comments ORDER BY id`.
- all checkpoints: `SELECT ... FROM checkpoints ORDER BY id`, then populate `Files` via existing `checkpointFiles`.
- all attr definitions: `ListAttrDefs()` exists already (ordered by key).
- all attribute values: `SELECT ca.task_id, ca.key, ca.value, ad.attr_type, ca.created_at, ca.updated_at FROM custom_attributes ca JOIN attribute_definitions ad ON ad.key = ca.key ORDER BY ca.task_id, ca.key`.

`func (s *Store) SyncExportAll(dir string) error` writes all seven files into `dir` (create with `os.MkdirAll(dir, 0o755)`). Keep `ExportJSONL(path)` as-is for `gig export` back-compat.

### Step 2 — LWW upserts

Verified on this repo's SQLite (modernc v1.46.1): `ON CONFLICT ... DO UPDATE ... WHERE` works and the row is untouched when the WHERE is false.

- **tasks** — change `upsertTask` (export.go:127) to `upsertTask(q dbq, t *Task) error`, append to the DO UPDATE clause: `WHERE excluded.updated_at > tasks.updated_at`. Also fix the NULL-parent inconsistency: bind `sql.NullString{String: t.ParentID, Valid: t.ParentID != ""}` instead of raw `t.ParentID` (today import writes `''` where `Create` writes NULL).
- **attribute values**: `INSERT INTO custom_attributes (task_id, key, value, created_at, updated_at) VALUES (?,?,?,?,?) ON CONFLICT(task_id, key) DO UPDATE SET value=excluded.value, updated_at=excluded.updated_at WHERE excluded.updated_at > custom_attributes.updated_at`.
- **attr definitions**: `INSERT ... ON CONFLICT(key) DO NOTHING` (no updated_at column — local wins; count skips in stats).
- **deps / comments**: `INSERT OR IGNORE` (append-only sets; PKs are (from_id,to_id) and id).
- **checkpoints**: `INSERT OR IGNORE`; insert `checkpoint_files` rows **only when** `RowsAffected() == 1` (otherwise files duplicate on every import — checkpoint_files has no PK).
- **events**: do **not** import. The audit log is local history; merging two machines' logs by AUTOINCREMENT id would interleave nonsense. Export exists purely for backup/inspection. Document this.

### Step 3 — `SyncImportAll(dir string) (*ImportStats, error)`

```go
type ImportStats struct {
    Tasks, Deps, Comments, Checkpoints, AttrDefs, Attrs int  // rows inserted or updated
    SkippedOlder int                                          // LWW kept local
}
```

One transaction for everything:
1. `tx, _ := s.db.Begin()`; `tx.Exec("PRAGMA defer_foreign_keys=ON")` — verified: child-before-parent inside the tx commits fine; genuinely dangling refs fail at COMMIT with "FOREIGN KEY constraint failed". This replaces (and must not reuse) the old pool-wide `PRAGMA foreign_keys=OFF` hack.
2. Import in this order (constraint-friendly even though deferral makes order mostly moot): tasks → attr_defs → attrs → deps → comments → checkpoints.
3. Missing files are fine (`os.IsNotExist` → skip; enables importing an old tasks.jsonl-only sync dir).
4. Malformed line → rollback with `fmt.Errorf("import %s line %d: %w", file, n, err)`. Use the same 1MB `bufio.Scanner` buffer as `ImportJSONL` (export.go:53).
5. Count LWW skips: for tasks/attrs, `RowsAffected() == 0` on a line whose ID/(task_id,key) already exists means local was newer → `SkippedOlder++`. (Detect "exists" by the zero rows-affected; no extra SELECT needed since a fresh insert always affects 1.)

Rewrite `ImportJSONL(path)` (the legacy single-file API) to parse then call the shared task-upsert loop inside the same kind of tx, so `gig import` gets LWW too. **Behavior change to document**: previously an import unconditionally stomped local rows; now an older import line loses to a newer local task.

### Step 4 — git layer (sync.go)

Small exec wrapper; check `exec.LookPath("git")` once and return `fmt.Errorf("git not found in PATH — install git or unset sync_repo")`.

```go
func gitRun(repoDir string, args ...string) (string, error)  // exec.Command("git", append([]string{"-C", repoDir}, args...)...), CombinedOutput
```

- `func (s *Store) SyncPush(repoDir string) (string, error)`:
  1. Verify repo: `gitRun(dir, "rev-parse", "--git-dir")` → on error: `"%s is not a git repository (git init it first)"`.
  2. `SyncExportAll(repoDir)`.
  3. `gitRun(dir, "status", "--porcelain")` → empty output ⇒ return "already up to date", skip commit.
  4. `gitRun(dir, "add", "-A")`, then commit `-m "gig sync <RFC3339 UTC now> from <os.Hostname best-effort>"`.
  5. `gitRun(dir, "remote")` → empty ⇒ return "committed locally (no remote configured)"; else `gitRun(dir, "push")` (first push may need upstream: on error containing "no upstream", retry with `push -u origin HEAD`).
- `func (s *Store) SyncPull(repoDir string) (*ImportStats, string, error)`: verify repo; if `gitRun(dir, "remote")` non-empty → `gitRun(dir, "pull", "--ff-only")`, on error return it wrapped with "resolve manually in <dir>, then rerun"; then `SyncImportAll(repoDir)`.
- `func (s *Store) SyncStatus(repoDir string) (string, error)`: porcelain dirty count + `gitRun(dir, "rev-list", "--left-right", "--count", "@{upstream}...HEAD")` → "behind N / ahead M"; if the rev-list errors (no upstream), report "no upstream configured".

### Step 5 — CLI (cmd/gig/sync_cmds.go)

`syncCmd` gains `--pull` and `--status` bool flags, `cmd.MarkFlagsMutuallyExclusive("pull", "status")`:

- Resolve repo dir: `cfg.SyncRepo` (after `~` expansion, step 6). If empty and `--pull`/`--status` → error `"sync_repo is not configured (gig config set sync_repo <path>)"`.
- No flags, `sync_repo` empty → **preserve today's behavior** (export tasks+events into `DefaultGigHome()`) and print a hint: `"tip: set sync_repo to enable git sync (gig config set sync_repo <path>)"` — TestCLI-compat: check existing e2e expectations on `gig sync` output first (none exist today; `TestCLI_ExportImport` uses `export`/`import`, not `sync`).
- No flags, `sync_repo` set → `SyncPush`, print its message.
- `--pull` → `SyncPull`, print stats (`"imported: N tasks, N deps, ... (skipped M older)"`).
- `--status` → print `SyncStatus` string.

Leave `gig export` / `gig import` commands exactly as they are (single-file, back-compat), but `gig export` should additionally mention: `"note: gig sync exports the full dataset (deps, comments, checkpoints, attrs)"`.

### Step 6 — `~` expansion (config.go `LoadConfig`)

YAML doesn't expand `~`. After parsing, normalize:

```go
if strings.HasPrefix(cfg.SyncRepo, "~/") {
    if home, err := os.UserHomeDir(); err == nil {
        cfg.SyncRepo = filepath.Join(home, cfg.SyncRepo[2:])
    }
}
```

### Step 7 — tests

Unit (`sync_test.go`, using `tempDB(t)`):

```
TestSyncExportImportFullFidelity — store A: 3 tasks (one with parent), 1 blocks-dep, 2 comments, 1 checkpoint WITH Files, 1 attr def + value. SyncExportAll(dir). Fresh store B: SyncImportAll(dir). Assert counts AND: dep exists (ListDependencies), checkpoint Files round-tripped, attr value+type readable (GetAttr), child's ParentID intact.
TestSyncImportLWWTaskNewerLocalWins — export from A; in B import once, then update B's task title (updated_at advances), re-import the old file: title keeps B's value; stats.SkippedOlder >= 1.
TestSyncImportLWWRemoteNewerWins    — reverse: bump the exported line's updated_at (rewrite the JSONL line in the test), import → remote title wins.
TestSyncImportChildBeforeParent     — handcraft tasks.jsonl with the child line FIRST; import succeeds (defer_foreign_keys), parent link intact.
TestSyncImportMissingFilesOK        — dir containing only tasks.jsonl imports without error.
TestSyncImportIdempotent            — SyncImportAll twice; second run: zero inserted, no duplicated checkpoint files (SELECT COUNT(*) FROM checkpoint_files stays constant — use store.DB()).
```

E2E (cmd/gig/cli_test.go — git is available in CI's ubuntu image; guard with `exec.LookPath("git")` → `t.Skip` if absent):

```
TestCLI_SyncGitRoundTrip:
  bare := t.TempDir(); git init --bare bare
  homeA/homeB: two setupGig homes; repoA/repoB: two clones of bare (git clone; set user.email/user.name locally in each clone!)
  gig(A): config set sync_repo repoA; create T1, T2; dep add T1 T2; comment T1 "hello"; sync            → pushes
  gig(B): config set sync_repo repoB; sync --pull                                                        → imports
  assert: gig(B) show T1 output contains "Depends on:" and "hello"
  gig(B): close T2 --reason done; sync
  gig(A): sync --pull; assert gig(A) show T2 contains "Closed"   (LWW: B's newer updated_at wins)
```

### Step 8 — docs

- docs/architecture.md "Sync Model": new file table, LWW rule, "deletions and dependency removals do not propagate" limitation, events-not-imported rationale.
- docs/roadmap.md: tick the four v0.3.0 boxes.
- README.md "JSONL sync" bullet → describe full-fidelity sync + `--pull`/`--status`.

## Edge cases a weaker model would miss

1. **`PRAGMA foreign_keys=OFF` on the pool is a no-op lie** (verified: pooled connections keep their own setting). The only correct import posture is one transaction + `PRAGMA defer_foreign_keys=ON` on that tx.
2. **LWW compares timestamps as strings.** Works because all stored times are UTC + fixed layout. After Plan 1 the layout gains `.000` milliseconds. Mixed-precision caveat: `'Z'` (0x5A) sorts above `'.'` (0x2E), so a legacy second-precision string (`"...:00Z"`) compares as **newer** than any millisecond string within the same second (`"...:00.999Z"`). The skew is bounded to same-second edits written across the format migration — acceptable. Do not "fix" it by parsing timestamps inside SQL; just note it in a code comment.
3. **`checkpoint_files` has no primary key** — blind re-import duplicates file rows. Gate on the checkpoint INSERT's `RowsAffected`.
4. **Deletions don't propagate** (no tombstones in scope). A task deleted on machine A reappears after pulling from B. State this in docs; do not attempt tombstones here.
5. **`gig config set sync_repo ~/gig-sync` stores a literal `~`** — expand at load (step 6), not at save (users may edit YAML by hand).
6. **Fresh clones have no committer identity in CI** — the e2e test must `git config user.email/user.name` inside each test clone or commits fail.
7. **First push to a bare remote from a fresh clone**: `git push` can fail with "no upstream"; retry `push -u origin HEAD` (step 4.5). Also `git init --bare` + clone yields branch `main` or `master` depending on git version — never hardcode the branch name; `HEAD` avoids it.
8. **`Task.Children`/`Task.Attrs` fields are `omitempty` view-model fields** (populated by GetTree/GetFull). Export via `List()` leaves them empty — keep it that way; importing a hand-made file that *does* contain `children` must ignore them (json.Unmarshal into Task will populate the field; simply never read it during upsert).
9. **`sort.SliceStable` root-first ordering in the legacy `ImportJSONL`** (export.go:71-76) is insufficient for grandchildren and becomes irrelevant once deferral is in — you can delete that sort, but only after the deferral tx is in place.
10. **`SyncPush` exports into the repo dir, not `GIG_HOME`** — the two dirs coexist (`gig sync` without sync_repo still writes `~/.gig/tasks.jsonl`). Don't unify them.
11. **Don't shell out through `sh -c`** for git (paths with spaces); always `exec.Command("git", args...)`.

## Acceptance criteria

```bash
go build ./... && go vet ./...
go test ./... -count=1                          # incl. 6 new sync tests
go test -tags=e2e -v -count=1 ./cmd/gig/        # incl. TestCLI_SyncGitRoundTrip
```

Manual (fresh `GIG_HOME`, throwaway dir):
1. `git init --bare /tmp/sync.git && git clone /tmp/sync.git /tmp/sync && gig config set sync_repo /tmp/sync`
2. `gig create A; gig create B; gig dep add <A> <B>; gig sync` → prints a commit/push message; `ls /tmp/sync` shows all 7 jsonl files.
3. Point a second `GIG_HOME` + clone at the same bare repo, `gig sync --pull` → `gig show <A>` lists the dependency.
4. `gig sync --status` → reports clean/ahead-behind without error.
5. Old-format dir (tasks.jsonl only) + `gig import` → still works.

## Out of scope

- Tombstones / deletion propagation; three-way merge (LWW only, per roadmap).
- Importing events.
- Auto-sync hooks (e.g. sync-on-close) — future.
- Remote management (`git remote add`) — user sets up the repo.
