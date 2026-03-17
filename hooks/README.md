# Gig Hooks

Bundled hook scripts for integrating gig with developer tools.

## Layout

```
hooks/
├── agent/                     # Agent harness hooks (Claude Code, OpenCode, etc.)
│   ├── gig-session-start.sh   # Session start — inject task context on resume/start
│   ├── gig-pre-compact.sh     # Pre-compact — breadcrumb comment before context compaction
│   └── gig-pickup.sh          # Post tool use — binds task to session on `gig update --claim`
└── git/                       # Git hooks
    └── gig-post-commit.sh     # post-commit — log commit to gig task if message has gig-<id>
```

## Agent Hooks

These hooks work with any agent harness that supports lifecycle hooks
(Claude Code, OpenCode, etc.). The wiring mechanism differs per harness
but the scripts are the same.

### gig-pickup.sh

Triggers after any `gig update <id> --claim` command. Automatically maps
the claimed task to the current session in `sessions.json`. No separate
pickup script needed — just use `gig update <id> --claim` as normal.

### gig-session-start.sh

Fires on session start/resume/clear/compact. Injects active task context
(title, status, recent comments, git log) into the agent's prompt so it
knows what it was working on.

### gig-pre-compact.sh

Fires before context compaction. Adds a breadcrumb comment to each active
in-progress task so the audit trail survives context loss.

## Git Hooks

### gig-post-commit.sh

Runs after each git commit. If the commit message contains a `gig-<id>`
pattern, it logs the commit as a comment on that task.

## Installation

Currently manual — a future `gig install hook` command will automate this.
