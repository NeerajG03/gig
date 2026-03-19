# Adding a New CLI Command

## Steps

1. **SDK first**: Add the business logic in the root package (e.g., `task.go`, or a new file). Return `(*Type, error)`.
2. **Wire the command**: Add a `func fooCmd() *cobra.Command` in the appropriate `cmd/gig/*.go` file. Keep it thin — parse flags, call SDK, format output.
3. **Register it**: Add `fooCmd()` to `rootCmd.AddCommand(...)` in `cmd/gig/main.go`.
4. **Skip store init if needed**: If the command doesn't need the database (like `init`, `completion`, or `install` subcommands), add it to the skip list in `PersistentPreRunE`.
5. **Support output modes**: Honor `--json` (full JSON), `--quiet` (IDs only), and default (human-readable table/tree).
6. **Wire autocomplete** (see below).
7. **Use `actorName`**: Replace hardcoded `"cli"` with the package-level `actorName` variable for event attribution.

## Autocomplete

If the command takes a task ID as a positional arg, set `ValidArgsFunction`:
- `taskIDCompletion` — all tasks (general purpose)
- `openTaskIDCompletion` — non-closed tasks only (for commands like `close`, `cancel`)
- `closedTaskIDCompletion` — closed/cancelled tasks only (for commands like `reopen`)

If the command has enum flags (--status, --type, --priority), register completions:
```go
cmd.RegisterFlagCompletionFunc("status", statusCompletion)
cmd.RegisterFlagCompletionFunc("type", taskTypeCompletion)
cmd.RegisterFlagCompletionFunc("priority", priorityCompletion)
```

For custom attribute keys, use `attrKeyCompletion`. All completion functions live in `cmd/gig/completion.go`.

## Code Quality Checklist

- [ ] SDK function in root package (not in `cmd/`)
- [ ] Returns `(*Type, error)` or `error`
- [ ] Error wrapping with `fmt.Errorf("context: %w", err)`
- [ ] Parameterized SQL (`?` placeholders)
- [ ] Time scanned as string, parsed with `strToTime()`
- [ ] `--json`, `--quiet`, default output modes
- [ ] ValidArgsFunction wired for task ID args
- [ ] `actorName` used for event attribution
- [ ] Test in `*_test.go` using `tempDB(t)`
