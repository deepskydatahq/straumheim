---
name: straumheim-story-execution
description: Implements Straumheim product stories directly from product/stories TOML files without Beads. Use when selecting, claiming, implementing, testing, and completing a story.
---

# Straumheim Story Execution

Stories are the canonical implementation tasks. Do not use Beads/`bd`.

## Status model

Use story `status` values:

- `draft` — not ready for implementation
- `ready` — ready and unclaimed
- `in_progress` — currently being implemented
- `blocked` — cannot proceed without a decision or prerequisite
- `complete` — implemented, tested, and committed
- `failed` — attempted but not completed; include a failure reason

Use story `triage` values:

- `ready` — implement directly
- `plan` — explore relevant files, make a short plan, then implement
- `brainstorm` — compare 2-3 approaches, pick the simplest, then plan and implement

## Implementing a story

1. Read `AGENTS.md`, the story TOML, parent epic, and parent mission.
2. Verify dependencies in `[context].depends_on` are complete.
3. Set the story to `status = "in_progress"`.
4. Follow the triage path:
   - `ready`: implement directly.
   - `plan`: inspect relevant paths and write a concise plan before editing.
   - `brainstorm`: compare approaches briefly and choose the simplest.
5. Implement production code or documentation and tests for each acceptance criterion.
6. Run focused checks, then relevant quality gates:
   - `gofmt -w` on changed Go files
   - `go test ./...`
   - `go build ./...`
7. Commit the story with a descriptive message when practical.
8. Update the story to `status = "complete"` and record useful execution metadata if fields exist:
   - branch
   - commit
   - PR URL if known
   - completed date

## If blocked or failed

If implementation cannot continue:

1. Set `status = "blocked"` or `status = "failed"`.
2. Add or update `[execution].failure_reason` with a specific explanation.
3. Create or update follow-up product story TOML if more work is needed.
4. Do not mark the story complete.

## Completion rule

A story is complete only when:

- all acceptance criteria are satisfied
- tests were added or updated where appropriate
- quality gates pass, or failures are documented as out of scope
- changes are committed
