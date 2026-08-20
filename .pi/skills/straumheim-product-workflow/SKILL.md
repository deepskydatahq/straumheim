---
name: straumheim-product-workflow
description: Works with Straumheim product TOML files as the canonical planning and task system. Use when creating, breaking down, updating, or judging missions, epics, and stories. Never use Beads/bd.
---

# Straumheim Product Workflow

Product TOML files are the canonical task system. Do not use Beads/`bd`.

## Source of truth

```text
VISION.md
  -> VALUES.md
  -> product/missions/*.toml
  -> product/epics/*.toml
  -> product/stories/*.toml
  -> commits / PRs
```

Use product TOML state for task status. Stories are implementation tasks.

## Mission breakdown

When turning a mission into epics:

1. Read `AGENTS.md`, `VISION.md`, `VALUES.md`, and `product/missions/{mission_id}-*.toml`.
2. Extract outcome, user progress, testing criteria, scope, relevant paths, and notes.
3. Review relevant paths to understand current code, configuration, and documentation constraints.
4. Identify 3-6 independently shippable epic boundaries.
5. Create `product/epics/{mission_id}-E{NNN}-{slug}.toml`.
6. Verify every mission outcome is covered with no major gaps or overlaps.

Each epic should include:

- `id`, `parent`, `title`, `status`, `created`, `depends_on`
- `[outcome].description`
- `[job_story].description`
- `[testing].approach`, `criteria`, and `validator_context`
- `[context].relevant_paths` and `dependencies`
- `[notes].considerations`
- `estimated_stories`

Set new epics to `status = "ready"` unless they need human refinement; then use `draft` and explain why.

## Epic breakdown

When turning an epic into stories:

1. Read `AGENTS.md`, the epic TOML, and its parent mission.
2. Extract outcome, job story, testing criteria, relevant paths, and dependencies.
3. Review relevant paths for existing patterns.
4. Break the epic into stories that:
   - fit one implementation session
   - have one clear purpose
   - are testable in isolation
   - result in working code or documentation, not only scaffolding
5. Create `product/stories/{epic_id}-S{NNN}-{slug}.toml`.

Each story should include:

- `id`, `parent`, `title`, `status`, `triage`, `created`
- `[outcome].description`
- `[acceptance_criteria].executable`
- one or more `[[acceptance_criteria.criteria]]` entries
- `[context].relevant_paths`, `input_fixtures`, and `depends_on`
- `[handoff].implementation_hints` and `reference_files`
- optional `[execution]` metadata for branch, PR, commit, failure reason

## Triage labels

Use `triage = "ready"` when file paths are specific, changes are mechanical, scope is small, and no design decision is needed.

Use `triage = "plan"` when acceptance criteria are clear but codebase exploration is needed to identify exact files or steps.

Use `triage = "brainstorm"` when multiple approaches, architecture decisions, or unclear scope are involved.

## Product judgment

To validate a story, epic, or mission:

1. Read the artifact and its parents/children.
2. Check each acceptance or testing criterion against actual code/configuration/documentation/tests/build output.
3. Run focused checks when practical.
4. Mark `complete` only when criteria pass with evidence.
5. If failing, leave or set `in_progress`, `blocked`, or `failed` and explain why.

Be strict about criteria but do not fail for unrelated style preferences.
