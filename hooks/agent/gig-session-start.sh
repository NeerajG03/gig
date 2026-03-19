#!/bin/bash
# Hook: SessionStart — inject gig usage instructions into agent prompt
# Fires on: startup, resume, clear, compact

set -euo pipefail

GIG_HOME="${GIG_HOME:-$HOME/.gig}"
CURRENT_SESSION_FILE="$GIG_HOME/current_session"
SESSIONS_FILE="$GIG_HOME/sessions.json"

# Read JSON input from stdin
INPUT=$(cat)
SESSION_ID=$(echo "$INPUT" | jq -r '.session_id // ""')

# Ensure gig home exists and write current session ID
mkdir -p "$GIG_HOME"
if [ -n "$SESSION_ID" ]; then
  echo "$SESSION_ID" > "$CURRENT_SESSION_FILE"
fi
if [ ! -f "$SESSIONS_FILE" ]; then
  echo '{}' > "$SESSIONS_FILE"
fi

CONTEXT="# Gig Task Management

You have access to \`gig\` — a CLI task tracker. Use it to manage work.

## Quick reference
- \`gig list\`                                  — list open tasks
- \`gig list --tree\`                           — hierarchical view
- \`gig show <id>\`                             — task details
- \`gig create \"<title>\"\`                      — create a task
- \`gig update <id> --claim --actor <name>\`    — claim a task (sets assignee + in_progress)
- \`gig update <id> --status <s>\`              — change status (open/in_progress/closed)
- \`gig close <id>\`                            — close a task
- \`gig comment <id> \"<text>\"\`                 — add a comment
- \`gig comments <id>\`                         — view comments

When you commit code related to a task, include the task ID in the commit message (e.g. \`fix: resolve bug (gig-ab12)\`) — a git hook will auto-log it.
"

jq -n \
  --arg ctx "$CONTEXT" \
  '{
    hookSpecificOutput: {
      hookEventName: "SessionStart",
      additionalContext: $ctx
    }
  }'
