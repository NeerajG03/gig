#!/bin/bash
# Git post-commit hook — logs commit to a gig task if commit title contains gig-<id>
# Format: "committed to <repo>: <hash> - <title>"
# Install: symlink this into .git/hooks/post-commit in repos JEFF works on

set -euo pipefail

# Get commit info
TITLE=$(git log -1 --format='%s' 2>/dev/null || exit 0)
HASH=$(git log -1 --format='%h' 2>/dev/null || exit 0)

# Extract gig task ID(s) from commit title — matches gig-XXXX or gig-XXXX.N patterns
TASK_IDS=$(echo "$TITLE" | grep -oE 'gig-[a-z0-9]+(\.[0-9]+)*' || true)
if [ -z "$TASK_IDS" ]; then
  exit 0
fi

# Derive repo name from directory name (e.g., backend, frontend, infra-configurations)
REPO_NAME=$(basename "$(git rev-parse --show-toplevel 2>/dev/null)" 2>/dev/null || echo "unknown")

# Log commit to each referenced task
while IFS= read -r TASK_ID; do
  [ -z "$TASK_ID" ] && continue
  gig comment "$TASK_ID" "committed to ${REPO_NAME}: ${HASH} - ${TITLE}" --author "git-hook" 2>/dev/null || true
done <<< "$TASK_IDS"

exit 0
