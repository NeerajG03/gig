#!/bin/bash
# Claude Code hook: PostToolUse (matcher: Bash)
# Fires after any `gig update <id> --claim` command to bind the task to the
# current Claude session. This replaces the old standalone pickup script —
# claiming now always goes through `gig update --claim` and this hook
# automatically handles session bookkeeping.
#
# settings.json wiring:
#   "PostToolUse": [{
#     "matcher": "Bash",
#     "hooks": [{
#       "type": "command",
#       "command": "<path>/gig-pickup.sh"
#     }]
#   }]

set -euo pipefail

INPUT=$(cat)

TOOL_NAME=$(echo "$INPUT" | jq -r '.tool_name // ""')
COMMAND=$(echo "$INPUT" | jq -r '.tool_input.command // ""')
SESSION_ID=$(echo "$INPUT" | jq -r '.session_id // ""')

# Only act on Bash calls that contain `gig update ... --claim`
if [[ "$TOOL_NAME" != "Bash" ]]; then
  echo '{"continue": true}'
  exit 0
fi

if ! echo "$COMMAND" | grep -qE 'gig\s+update\s+\S+.*--claim'; then
  echo '{"continue": true}'
  exit 0
fi

# Extract task ID: first argument after `gig update`
TASK_ID=$(echo "$COMMAND" | grep -oE 'gig\s+update\s+(\S+)' | awk '{print $3}')
if [[ -z "$TASK_ID" ]]; then
  echo '{"continue": true}'
  exit 0
fi

GIG_HOME="${GIG_HOME:-$HOME/.gig}"
SESSIONS_FILE="$GIG_HOME/sessions.json"

# Need a session ID to map
if [[ -z "$SESSION_ID" ]]; then
  echo '{"continue": true}'
  exit 0
fi

# Initialize sessions.json if missing
mkdir -p "$GIG_HOME"
if [[ ! -f "$SESSIONS_FILE" ]]; then
  echo '{}' > "$SESSIONS_FILE"
fi

# Add task to this session (deduplicate)
TEMP_FILE=$(mktemp)
jq --arg sid "$SESSION_ID" --arg tid "$TASK_ID" '
  if .[$sid] then
    .[$sid].tasks = ((.[$sid].tasks // []) + [$tid] | unique)
  else
    .[$sid] = {
      tasks: [$tid],
      started_at: (now | strftime("%Y-%m-%dT%H:%M:%SZ"))
    }
  end
' "$SESSIONS_FILE" > "$TEMP_FILE" && mv "$TEMP_FILE" "$SESSIONS_FILE"

# Feed context back to Claude
jq -n \
  --arg tid "$TASK_ID" \
  --arg sid "$SESSION_ID" \
  '{
    "continue": true,
    "hookSpecificOutput": {
      "hookEventName": "PostToolUse",
      "additionalContext": ("Picked up task " + $tid + " in session " + $sid)
    }
  }'
