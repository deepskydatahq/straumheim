#!/usr/bin/env bash
# Huginn Memory — SessionStart hook
# Injects project context instructions so the agent bootstraps from memory.
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
## Huginn Project Memory Bootstrap

You are working on project **${HUGINN_PROJECT_SLUG}**. Before starting work, load project context:

1. \`recall(query="recent decisions, blockers, and learnings", project="${HUGINN_PROJECT_SLUG}", limit=5)\` — get recent project memory
2. \`list_tasks(project="${HUGINN_PROJECT_SLUG}", status="open")\` — check open tasks
3. \`get_agent_profile(name="<your-agent-name>", project="${HUGINN_PROJECT_SLUG}")\` — load your agent profile if you are an agent

Always pass \`project="${HUGINN_PROJECT_SLUG}"\` on all scoped MCP tool calls (memory, tasks, agents).
EOF
