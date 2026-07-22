# Changelog

All notable changes to gig are documented here. This project follows
[Semantic Versioning](https://semver.org/).

## [Unreleased]

### Added

- **`EventsAfterID(afterID int64, limit int)`** — event-stream cursor keyed on
  the events table's monotonic integer primary key. Persist the last id you
  processed and pass it back to resume with no missed or duplicated events.
  Prefer this over `EventsSince` for reliable polling: RFC3339 timestamps have
  only second precision, so a time cursor can miss or duplicate events sharing a
  boundary second.
- `-race` in CI (test and release workflows) plus a `go vet` step.

### Changed

- **`Claim` is now compare-and-swap (BREAKING semantics).** The claim is applied
  with a conditional `UPDATE`, so two concurrent claimers can no longer both
  succeed. Claiming a task already held by a **different** assignee now returns
  the new sentinel **`ErrAlreadyClaimed`** (check with `errors.Is`). Re-claiming
  by the **same** assignee still succeeds (idempotent resume). Terminal tasks
  (closed/cancelled) return `ErrAlreadyClaimed`; `open`/`blocked`/`deferred`
  remain claimable as before.
- **Default generated-ID hash length 4 → 6** (`gig-a3f8` → `gig-a3f8c1`),
  expanding the space from 65,536 to 16.7M values. Existing IDs stay valid —
  length only affects newly generated IDs. Override with `WithHashLength`.
- `Create` now retries on an ID collision (root tasks regenerate the random id;
  subtasks bump the ladder offset) instead of surfacing a raw
  `UNIQUE constraint failed` error; after the bounded retries it returns a clear
  "id space exhausted" error.
- `Events`, `EventsSince`, and `Events(taskID)` order by `(timestamp, id)` so
  same-second batches are deterministic (insertion order).

### Fixed

- **Connection PRAGMAs now apply to every pooled connection.** `busy_timeout`,
  `journal_mode=WAL`, and `foreign_keys` are set via the DSN's `_pragma` query
  params instead of one-off `PRAGMA` execs that bound to a single pooled
  connection — the previous approach left other connections with
  `busy_timeout=0` (instant `SQLITE_BUSY` under contention) and foreign keys
  off. `SetMaxOpenConns(1)` additionally serializes in-process access.
- **Task mutations and their audit events are now transactional.** `Create`,
  `Update`, `UpdateStatus`, `CloseTask`, `CancelTask`, and `Claim` write the row
  change and its event in one transaction (event emission happens after commit),
  so a crash or a failed event insert can no longer leave a mutation without its
  audit record. `recordEvent` no longer silently discards insert errors.
- **Concurrent subtask creation no longer mints duplicate ladder IDs.** The
  child-count read and insert share the `Create` transaction, with a bounded
  retry on collision.
