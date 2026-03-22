#!/usr/bin/env bash
# Huginn Memory — Stop hook
# Instructs the agent to persist session learnings before exiting.
#
# Install: copy to .claude/hooks/ in your project repo
# Configure: set HUGINN_PROJECT_SLUG below or in .huginn-project file

set -euo pipefail

# --- Config ---
# Override this per project, or create a .huginn-project file in repo root
HUGINN_PROJECT_SLUG="${HUGINN_PROJECT_SLUG:-}"

if [[ -z "$HUGINN_PROJECT_SLUG" ]]; then
  # Try reading from .huginn-project file (walk up to repo root)
  dir="$PWD"
  while [[ "$dir" != "/" ]]; do
    if [[ -f "$dir/.huginn-project" ]]; then
      HUGINN_PROJECT_SLUG="$(cat "$dir/.huginn-project" | tr -d '[:space:]')"
      break
    fi
    dir="$(dirname "$dir")"
  done
fi

if [[ -z "$HUGINN_PROJECT_SLUG" ]]; then
  exit 0 # No project configured, skip silently
fi

cat <<EOF
## Huginn Project Memory — Save Before Exit

Before ending this session, persist your learnings for project **${HUGINN_PROJECT_SLUG}**:

1. \`remember(content="<key decisions, discoveries, and learnings from this session>", project="${HUGINN_PROJECT_SLUG}")\` — save session learnings
2. Update any Huginn tasks you worked on: \`update_task(id=..., status="done"|"in_progress", project="${HUGINN_PROJECT_SLUG}")\`
3. If you hit blockers or open questions, record them: \`remember(content="<blocker or open question>", project="${HUGINN_PROJECT_SLUG}")\`

Always pass \`project="${HUGINN_PROJECT_SLUG}"\` on all scoped MCP tool calls.
EOF
