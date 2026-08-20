# Agent Instructions

This project uses **product TOML files as the canonical planning and implementation task system**. Do **not** use Beads/`bd`.

## Product Workflow

```text
VISION.md
  -> VALUES.md
  -> product/missions/*.toml
  -> product/epics/*.toml
  -> product/stories/*.toml
  -> commits / PRs
```

Stories in `product/stories/*.toml` are implementation tasks. Product TOML state is the source of truth for mission, epic, and story progress.

## Quick Reference

```text
/plan-mission <idea>                  # Create a grounded mission TOML
/product-mission-breakdown M001       # Break a mission into epics
/product-epic-breakdown M001-E001     # Break an epic into stories
/execute-mission M001                 # Execute a mission
/mission-status M001                  # Show mission, epic, and story state
/story-list ready                     # List ready stories
/story-set-status <id> in_progress    # Update story state
/product-judgment <id>                # Validate outcomes against evidence
/quality-gates                        # Run formatting, tests, and build
/landing-plane                        # Complete and push the session
```

## Story Statuses

Use these values in `product/stories/*.toml`:

- `draft` — not ready for implementation
- `ready` — ready and unclaimed
- `in_progress` — currently being implemented
- `blocked` — waiting on a decision or prerequisite
- `complete` — implemented, tested, and committed
- `failed` — attempted but incomplete; include a failure reason

Use `triage = "ready" | "plan" | "brainstorm"` to select direct implementation, brief planning, or approach comparison.

## Go Guidelines

- Keep packages focused and interfaces small.
- Return and wrap errors with useful context; do not silently discard them.
- Use `context.Context` for cancellable or request-scoped work.
- Add table-driven tests where they improve coverage and readability.
- Run focused tests while developing, then the full gates before shipping.

## Quality Gates

For Go changes:

```bash
gofmt -w <changed-go-files>
go test ./...
go build ./...
```

For website-only changes, run the relevant npm build from `website/` when dependencies are available.

## Landing the Plane

Work is not complete until intended changes are committed and `git push` succeeds.

1. Create or update product stories for remaining work.
2. Run relevant quality gates.
3. Update story, epic, and mission statuses with evidence.
4. Commit intended changes with a clear message.
5. Push and verify:
   ```bash
   git pull --rebase
   git push
   git status --short --branch
   ```
6. Confirm the branch is up to date with origin and hand off completed work, gates, remaining stories, and blockers.

## Critical Rules

- Product TOML is the task-state source of truth; do not create or update Beads issues.
- Do not mark work complete until acceptance criteria and relevant gates pass.
- Do not overwrite unrelated work already present in the worktree.
- Never stop before pushing completed work unless the user explicitly asks not to push.
