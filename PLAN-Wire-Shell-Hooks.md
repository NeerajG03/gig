# PLAN: Wire Up Shell Hooks (the feature is dead code from the CLI)

**Rank: 5 of 5 by total leverage — but the best effort-to-value ratio of the five (roughly an afternoon). Fully independent of the other plans; a good warm-up.**

## Problem (verified on this codebase, 2026-07-07, commit `08c7a13`)

The `gig.yaml` `hooks:` feature is documented in three places — README ("Event system — ... shell hooks"), docs/architecture.md (data-flow diagram: `→ RunHooks() ← fires shell hooks`, plus a config example), docs/sdk-reference.md:150 — and `gig doctor` even validates hook config (doctor.go:176-190). But **`Store.EnableHooks()` is never called anywhere in `cmd/gig/`** (verified: the only references are hook.go and hook_test.go). Every user who configures `hooks:` in gig.yaml gets silent nothing.

Two latent bugs will surface the moment it's wired up, so they're in scope:

1. **Hooks race against process exit.** `RunHooks` fires `go executeHook(cmd)` (hook.go:39) fire-and-forget. A CLI process ends right after the mutation; whether the goroutine reaches `exec` before `main` returns is a coin flip → hooks fire *sometimes*, the worst failure mode.
2. **The `assignee` hook filter matches the wrong field.** `hookMatchesFilter` compares `filter.assignee` against `e.Actor` (hook.go:55-58) — the person *making* the change — not the new assignee (`e.NewValue` on `EventAssigned`).

Also in scope: hook template values are spliced raw into `sh -c` (hook.go:65-74). The command string is user-authored config (trusted), but `{new}`/`{old}` can carry arbitrary task data (e.g. a title with `"; rm -rf ~"` — task titles land in `new_value` of `created` events). Passing values via environment variables kills that class of injection without breaking existing configs.

## Goal

Configured hooks actually fire on CLI mutations and are guaranteed to have *started and finished* (or hit a 10s cap) before the process exits; values are also available as env vars; filters match documented semantics; e2e proof exists.

## Files to touch

| File | What changes |
|---|---|
| `hook.go` | WaitGroup tracking + `WaitHooks`; env-var passing; `assignee` filter fix |
| `cmd/gig/main.go` | call `EnableHooks()` after open; `WaitHooks` in `PersistentPostRun` |
| `hook_test.go` | new unit tests |
| `cmd/gig/cli_test.go` | e2e hook-fires test |
| `docs/architecture.md`, `README.md` | document env vars, cancel-vs-close hook semantics, long-running-hook guidance |

## Implementation order

### Step 1 — waitable hooks (hook.go)

Add to `Store` (store.go): `hookWG sync.WaitGroup`.

In `RunHooks` (hook.go:34-41):

```go
for _, h := range hooks {
    if !hookMatchesFilter(h, event) {
        continue
    }
    cmd := expandHookVars(h.Command, event)
    s.hookWG.Add(1)
    go func(cmd string, e Event) {
        defer s.hookWG.Done()
        executeHook(cmd, e)
    }(cmd, event)
}
```

Add:

```go
// WaitHooks blocks until all in-flight shell hooks finish, or the timeout
// elapses. Returns true if everything finished. Long-running CLI commands
// call this before exit so fire-and-forget hooks aren't killed mid-spawn.
func (s *Store) WaitHooks(timeout time.Duration) bool {
    done := make(chan struct{})
    go func() { s.hookWG.Wait(); close(done) }()
    select {
    case <-done:
        return true
    case <-time.After(timeout):
        return false
    }
}
```

### Step 2 — env-var value passing (hook.go `executeHook`)

Change signature to `executeHook(cmd string, e Event)` and set:

```go
c := exec.Command("sh", "-c", cmd)
c.Env = append(os.Environ(),
    "GIG_TASK_ID="+e.TaskID,
    "GIG_EVENT="+string(e.Type),
    "GIG_ACTOR="+e.Actor,
    "GIG_FIELD="+e.Field,
    "GIG_OLD="+e.OldValue,
    "GIG_NEW="+e.NewValue,
)
```

Keep `expandHookVars` (`{id}`, `{old}`, `{new}`, `{actor}`, `{field}`) for backward compatibility, but document that env vars are the injection-safe form (`"$GIG_NEW"` is data; `{new}` is code). Do not attempt to shell-quote `{new}` substitutions — that breaks configs that intentionally splice, e.g. `notify.sh {id}`.

### Step 3 — `assignee` filter fix (hook.go:55-58)

```go
case "assignee":
    // Match the task's new assignee (EventAssigned carries it in NewValue).
    if e.NewValue != val {
        return false
    }
```

### Step 4 — CLI wiring (cmd/gig/main.go)

In `PersistentPreRunE`, after `store = s` (line 44): `store.EnableHooks()`.

**Do NOT put the wait in `PersistentPostRun`** — cobra skips PostRun hooks when `RunE` returns an error, and a partially-successful command (`gig close a b` failing on `b` after closing `a`) must still flush `a`'s hooks. Put it in `main()` where it runs unconditionally, and remove the existing `PersistentPostRun` block (lines 47-51) so `Close` isn't reachable twice:

```go
err := rootCmd.Execute()
if store != nil {
    store.WaitHooks(10 * time.Second)
    store.Close()
}
if err != nil {
    os.Exit(1)
}
```

`EnableHooks` already no-ops on nil config, and with zero configured hooks `RunHooks` selects an empty list, so `WaitHooks` returns instantly for the overwhelmingly common no-hooks case — no latency regression.

### Step 5 — tests

Unit (hook_test.go — it already builds Stores with inline `Config` literals; follow that pattern):

```
TestHooksWaitable            — config with on_create hook `touch <tmpdir>/ran`; Create a task; WaitHooks(5s) returns true; the file exists (no sleep loops).
TestWaitHooksNoHooks         — no hooks configured; WaitHooks(1s) returns true immediately.
TestAssigneeFilterNewValue   — hook with filter assignee=alice; RunHooks(EventAssigned{Actor:"bob", NewValue:"alice"}) fires (file appears after WaitHooks); NewValue:"carol" does not.
TestHookEnvVars              — hook command `printenv GIG_NEW > <tmpdir>/env`; fire event with NewValue "hello"; file content is "hello\n".
```

E2E (cmd/gig/cli_test.go):

```
TestCLI_HooksFireOnCreate —
  home := setupGig(t)
  overwrite home/gig.yaml adding:
      hooks:
        on_create:
          - command: "touch $GIG_HOME/hook-ran"
  (write the full yaml including prefix so LoadConfig keeps working; or append the hooks block to the existing file)
  run gig create "x"
  assert file home/hook-ran exists IMMEDIATELY after the command returns — WaitHooks in PostRun is exactly what guarantees this; no polling loop allowed in the test, its absence of flakiness is the feature under test.
TestCLI_HooksStatusFilter —
  on_status_change with filter new_status: closed → touch file; `gig update --status in_progress` must NOT create it; `gig close` must.
```

Note for the second test: `gig close` emits `EventClosed` (task.go:275), **not** `EventStatusChanged` — so an `on_status_change` hook with `new_status: closed` fires only when status changes to closed via `UpdateStatus`... verify against the dispatch table in `RunHooks` (hook.go:21-32): `EventClosed → OnClose` list, `EventStatusChanged → OnStatusChange` list. Write the e2e with `on_close` for the close case, and document (step 6) that:
- closing fires `on_close` (not `on_status_change`),
- cancelling fires `on_status_change` with `new_status: cancelled` (CancelTask records `EventStatusChanged`, task.go:303) — there is no `on_cancel`.
(If PLAN-Status-Transition-Integrity lands first, `gig update --status closed` delegates to CloseTask and therefore also fires `on_close` — the doc wording above stays true either way.)

### Step 6 — docs

- docs/architecture.md "Event System → Shell hooks": add the `GIG_*` env vars table, the close/cancel hook-list mapping above, and: "hooks are waited on for up to 10s at CLI exit; daemonize long-running work (`nohup cmd &`) if you need to outlive that".
- README hooks mention: one-line example with env var usage.
- docs/sdk-reference.md: `WaitHooks`.

## Edge cases a weaker model would miss

1. **The whole feature hinges on wait-before-exit.** Just calling `EnableHooks()` makes hooks fire *nondeterministically* (goroutine vs process exit race) — arguably worse than not firing. `WaitHooks` in `PersistentPostRun` is the actual fix; the e2e test asserting the marker file with **no retry loop** is what pins it.
2. **`PersistentPreRunE` skips store init for `init`, `completion`, and `install` subcommands** (main.go:29-34) — `store` is nil there; the existing nil-check in PostRun already covers it. Don't move `EnableHooks` anywhere that assumes a store always exists.
3. **`gig ui` blocks forever in RunE** — PostRun's WaitHooks only runs after Ctrl-C kills the process... actually it never runs (process dies). That's fine: the UI server process is long-lived, hooks have all the time they need; goroutines complete naturally. No special handling — but don't "fix" it by waiting inside RunHooks (that would serialize mutations behind hooks).
4. **Hook stderr goes to `~/.gig/hooks.log` via `DefaultGigHome()`** (hook.go:81) which respects `GIG_HOME` — e2e tests are already isolated. Don't redirect hook stdout (`c.Stdout = os.Stdout`, hook.go:79) — existing behavior, users may rely on it.
5. **A hook that spawns a daemon** (`some-watcher &` without `nohup`/disown still holds the pipe): `c.Run()` waits on the process, and `sh` exits when the child backgrounds — but the WaitGroup caps at 10s regardless, so a stuck hook delays CLI exit by 10s max, never hangs it. Keep the timeout; don't make it configurable in this pass.
6. **WaitGroup `Add` must happen in `RunHooks` (caller goroutine), not inside the spawned goroutine** — otherwise `WaitHooks` can observe a zero counter before the goroutine starts (classic WaitGroup race).
7. **`expandHookVars` on values containing `{`**: `strings.NewReplacer` is single-pass; a NewValue containing `{actor}` will NOT be re-expanded into the actor — already safe; leave the replacer order alone.
8. **The e2e yaml write**: `setupGig` runs `gig init` which writes gig.yaml. Appending a `hooks:` block with plain `os.WriteFile(append)` must respect YAML — read the file, append the block with correct top-level indentation, write back. Simplest robust approach: unmarshal into `gig.Config`, set `cfg.Hooks.OnCreate = []gig.HookDef{{Command: ...}}`, `gig.SaveConfig(path, cfg)` — but that's SDK-in-e2e (the e2e package imports the binary only via exec today; importing the gig package in cli_test.go is already done? check imports — if not, hand-write the whole yaml file content in the test, which is fine and explicit).
9. **`--json`/`--quiet` output**: hooks write to stdout (`c.Stdout = os.Stdout`) and can corrupt `--json` output consumed by agents. Note it in docs ("hooks writing to stdout will interleave with command output; write to files or stderr in agent contexts") — do not change the plumbing in this pass, it's an existing documented-behavior decision.
10. **Cobra skips `PersistentPostRun` when `RunE` errors.** That's why step 4 moves wait+close into `main()` after `Execute()` returns: `gig close a b` can close `a`, then fail on `b` — RunE returns an error, and hooks for `a` must still be flushed. If you leave the wait in PostRun, the flakiest case (hook loss on partly-failed batch commands) survives.

## Acceptance criteria

```bash
go build ./... && go vet ./...
go test ./... -count=1                      # incl. 4 new hook unit tests
go test -tags=e2e -v -count=1 ./cmd/gig/    # incl. 2 new hook e2e tests, zero retries/sleeps in them
grep -n "EnableHooks" cmd/gig/main.go       # exactly one match
```

Manual (fresh GIG_HOME):
1. Add to `$GIG_HOME/gig.yaml`:
   ```yaml
   hooks:
     on_create:
       - command: "echo created {id} >> $GIG_HOME/audit.txt"
     on_close:
       - command: "echo closed $GIG_TASK_ID >> $GIG_HOME/audit.txt"
   ```
2. `gig create hello; gig close <id>; cat $GIG_HOME/audit.txt` → two lines, correct IDs, present immediately (no sleep).
3. `gig doctor` still reports config valid.
4. Run 20× in a loop: `audit.txt` gains exactly 40 lines (determinism check — this fails without WaitHooks).

## Out of scope

- Webhook (HTTP) hooks — roadmap "Future Ideas".
- Hook timeout configurability, retry policy, structured hook output.
- Firing hooks from the `ui/` server beyond what EnableHooks already provides when the CLI launches it (uiCmd uses the same store — hooks fire there automatically once wired; long-lived process needs no wait).
