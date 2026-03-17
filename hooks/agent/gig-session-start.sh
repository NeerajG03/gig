#!/bin/bash
# Hook: SessionStart — inject gig task context into agent prompt
# Fires on: startup, resume, clear, compact (source field varies)

set -euo pipefail

GIG_HOME="${GIG_HOME:-$HOME/.gig}"
SESSIONS_FILE="$GIG_HOME/sessions.json"
CURRENT_SESSION_FILE="$GIG_HOME/current_session"
HOOK_EVENT="SessionStart"

# Read JSON input from stdin
INPUT=$(cat)
SESSION_ID=$(echo "$INPUT" | jq -r '.session_id // ""')

# Ensure gig home exists
mkdir -p "$GIG_HOME"

# Write current session ID for non-hook scripts (gig-pickup.sh)
if [ -n "$SESSION_ID" ]; then
  echo "$SESSION_ID" > "$CURRENT_SESSION_FILE"
fi

# Initialize sessions.json if missing
if [ ! -f "$SESSIONS_FILE" ]; then
  echo '{}' > "$SESSIONS_FILE"
fi

# Look up tasks for this session
TASK_IDS=()
if [ -n "$SESSION_ID" ]; then
  TASK_IDS_RAW=$(jq -r --arg sid "$SESSION_ID" '.[$sid].tasks // [] | .[]' "$SESSIONS_FILE" 2>/dev/null)
  if [ -n "$TASK_IDS_RAW" ]; then
    while IFS= read -r tid; do
      TASK_IDS+=("$tid")
    done <<< "$TASK_IDS_RAW"
  fi
fi

CONTEXT=""

if [ ${#TASK_IDS[@]} -gt 0 ]; then
  # --- RESUMING: Show task details + recent comments ---
  CONTEXT="# Gig Task Context\n\n## You were working on:\n"

  for TASK_ID in "${TASK_IDS[@]}"; do
    # Get task summary
    TASK_JSON=$(gig show "$TASK_ID" --json 2>/dev/null || echo '{}')
    TITLE=$(echo "$TASK_JSON" | jq -r '.title // "unknown"')
    STATUS=$(echo "$TASK_JSON" | jq -r '.status // "unknown"')
    PRIORITY=$(echo "$TASK_JSON" | jq -r '.priority // 0')
    DESC=$(echo "$TASK_JSON" | jq -r '.description // ""' | head -c 200)

    CONTEXT+="- **${TASK_ID}**: ${TITLE} (${STATUS}, P${PRIORITY})\n"
    if [ -n "$DESC" ]; then
      CONTEXT+="  Description: ${DESC}\n"
    fi

    # Get last 5 comments as breadcrumbs
    COMMENTS=$(gig comments "$TASK_ID" --json 2>/dev/null || echo '[]')
    RECENT_COMMENTS=$(echo "$COMMENTS" | jq -r '.[-5:][] | "  - [\(.author)] \(.content)"' 2>/dev/null || true)
    if [ -n "$RECENT_COMMENTS" ]; then
      CONTEXT+="  Recent activity:\n${RECENT_COMMENTS}\n"
    fi
  done

  # Recent git commits
  GIT_LOG=$(git log --oneline -5 2>/dev/null || echo "(not in a git repo)")
  CONTEXT+="\n## Recent git commits:\n${GIT_LOG}\n"

fi
# Fresh session with no task mapping: inject nothing.

# Output hook response
jq -n \
  --arg ctx "$CONTEXT" \
  --arg event "$HOOK_EVENT" \
  '{
    hookSpecificOutput: {
      hookEventName: $event,
      additionalContext: $ctx
    }
  }'
