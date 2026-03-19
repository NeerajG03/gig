# Testing

gig has two layers of tests:

1. **SDK unit tests** — test SDK methods directly in the root package using `tempDB(t)`. Fast, isolated, no binary needed.
2. **E2E CLI tests** — build the `gig` binary, run actual CLI commands, assert stdout/stderr output. Gated behind `e2e` build tag.

## Running Tests

```bash
go test ./...                                      # SDK unit tests only
go test -v ./...                                   # Verbose
go test -run TestList ./...                        # Specific test by name
go test -count=1 ./...                             # Disable cache
go test -tags=e2e -v -count=1 ./cmd/gig/           # E2E CLI tests (builds binary)
```

## SDK Unit Tests

- **`tempDB(t)`** (`store_test.go`): Creates a temporary SQLite store with prefix `"test"` and auto-cleanup. Every test that needs a store should use this — never share state between tests.
- Tests are in `*_test.go` files alongside the code they test (same `package gig`).
- No external test dependencies — just `testing` stdlib.

## E2E CLI Tests

- Located in `cmd/gig/cli_test.go` with build tag `//go:build e2e`.
- Uses `setupGig(t)` helper which builds the binary to a temp dir and creates an isolated `GIG_HOME`.
- Tests run the actual `gig` binary via `exec.Command` and assert on output.
- Slower than unit tests — run separately and not cached by CI on every push.

## How to Write Tests

1. **One test file per source file**: `task.go` → `task_test.go`, `checkpoint.go` → `checkpoint_test.go`, etc.
2. **Start with `tempDB(t)`**: Get a clean store for each test function.
3. **Test the SDK, not the CLI**: Tests go in the root package and exercise SDK methods directly. The CLI is a thin wrapper — if the SDK is correct, the CLI is correct.
4. **Write regression tests for bugs**: If a bug was found (especially runtime bugs that compile but fail), add a test that would have caught it. Document the bug in the test name or comments. Example: `TestListCommentsCreatedAtParsed` is a regression test for the `time.Time` scanning bug.
5. **Test validation and edge cases**: Empty inputs, nonexistent IDs, invalid enum values, boundary conditions (e.g., `hash_length: 1` and `hash_length: 20` in config tests).
6. **Use table-driven tests** for parameterized cases when testing many inputs against the same logic.

## Existing Test Files

| File | What it tests |
|------|--------------|
| `store_test.go` | `tempDB()` helper, Open/Close, directory creation, migration idempotency |
| `task_test.go` | CRUD, list filters, search, tree, subtask ID ladder, ExcludeStatuses, root task filter, ready/blocked, auto-unblock, cancel, IsTerminal |
| `checkpoint_test.go` | Add/list/latest checkpoints, files linking, validation, no-files round-trip |
| `comment_test.go` | Add/list comments, time parsing regression, validation |
| `dependency_test.go` | Add/remove deps, cycle detection, dep tree |
| `attribute_test.go` | Define/set/get/list/delete attrs, type validation |
| `export_test.go` | JSONL export/import round-trip |
| `config_test.go` | Config defaults, view settings, hash length bounds, invalid YAML, hooks parsing, save/reload |
| `hook_test.go` | Filter matching, variable expansion, RunHooks dispatch, EnableHooks |
| `event_test.go` | Event recording, status change events, EventsSince, listeners (On/Off), multiple listeners |
| `doctor_test.go` | Healthy DB, no config, valid/invalid config warnings, integrity check, HasIssues |
