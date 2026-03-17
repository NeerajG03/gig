#!/bin/bash
# Hook: PreCompact — log a breadcrumb comment on active tasks before context is compacted
# No git log — commits are already tracked by the post-commit hook

set -euo pipefail

GIG_HOME="${GIG_HOME:-$HOME/.gig}"
SESSIONS_FILE="$GIG_HOME/sessions.json"

# Read JSON input from stdin
INPUT=$(cat)
SESSION_ID=$(echo "$INPUT" | jq -r '.session_id // ""')

# No session mapping file? Nothing to do.
if [ ! -f "$SESSIONS_FILE" ] || [ -z "$SESSION_ID" ]; then
  exit 0
fi

# Look up tasks for this session
TASK_IDS_RAW=$(jq -r --arg sid "$SESSION_ID" '.[$sid].tasks // [] | .[]' "$SESSIONS_FILE" 2>/dev/null)
if [ -z "$TASK_IDS_RAW" ]; then
  exit 0
fi

NOW=$(date -u +%Y-%m-%dT%H:%M:%SZ)

# Log compact marker on each active task
while IFS= read -r TASK_ID; do
  STATUS=$(gig show "$TASK_ID" --json 2>/dev/null | jq -r '.status // ""')
  if [ "$STATUS" = "in_progress" ]; then
    gig comment "$TASK_ID" "Session compacted at $NOW" --author "hook-compact" 2>/dev/null || true
  fi
done <<< "$TASK_IDS_RAW"

# Update last_compact_at in sessions.json
TEMP_FILE=$(mktemp)
jq --arg sid "$SESSION_ID" --arg now "$NOW" '
  if .[$sid] then .[$sid].last_compact_at = $now else . end
' "$SESSIONS_FILE" > "$TEMP_FILE" && mv "$TEMP_FILE" "$SESSIONS_FILE"

exit 0
