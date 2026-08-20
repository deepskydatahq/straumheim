---
description: Execute a Straumheim mission using product TOML as the canonical workflow
argument-hint: "<mission-id>"
---
# Execute Mission

Execute mission `$1` using the product TOML workflow. Do not use Beads/`bd`.

## Phase 1: Read context and mode

1. Read `AGENTS.md`, `VISION.md`, and `VALUES.md`.
2. Read the mission file: `product/missions/$1-*.toml`.
3. Extract outcome, scope, testing criteria, relevant paths, dependencies, and `[execution]` if present.
4. Determine execution mode:
   - Use `[execution].mode` when present.
   - Otherwise infer mode from the mission outcome/scope.

## Execution modes

| Mode | Behavior |
|------|----------|
| `implementation` | Break into epics/stories, implement code, configuration, documentation, and tests, validate, PR |
| `audit` / `planning` | Produce audit notes, decisions, and follow-up mission/story TOMLs |
| `docs` | Update documentation and examples, then validate them |
| `release` | Run version/changelog/deploy-prep workflow |

Audit/planning missions should produce the artifacts the mission asks for. Do not create implementation stories unless needed.

## Implementation mode

### Phase 2: Ensure epics and stories exist

1. Check for existing epics: `product/epics/$1-E*.toml`.
2. If missing, use the `straumheim-product-workflow` skill to create epics.
3. For each epic, check for existing stories: `product/stories/{epic_id}-S*.toml`.
4. If missing, use the `straumheim-product-workflow` skill to create stories.
5. Assign each story a `triage = "ready" | "plan" | "brainstorm"` field if missing.

### Phase 3: Build execution order

1. Build the dependency graph from epic `depends_on` and story `[context].depends_on`.
2. Identify ready stories whose dependencies are complete.
3. Prefer completing stories in epic order unless independent stories can safely be handled in parallel by separate sessions/worktrees.

### Phase 4: Implement stories

For each selected story:

1. Use the `straumheim-story-execution` skill.
2. Set story `status = "in_progress"`.
3. Implement production code or documentation and tests where appropriate.
4. Run focused checks, then `gofmt -w` on changed Go files, `go test ./...`, and `go build ./...` before shipping code changes.
5. Commit after each completed story when practical.
6. Set story `status = "complete"` and record useful execution metadata if an `[execution]` table exists.

If blocked, set `status = "blocked"` and document the reason in `[execution].failure_reason`.

## Validate and ship

1. Use product judgment to validate completed stories/epics/missions or mode-specific artifacts.
2. Run `gofmt -w` on changed Go files, `go test ./...`, and `go build ./...` when code changed.
3. Push the branch and create/update a PR if this is feature work.
4. Use `/pr-status` to inspect checks and review comments.
5. Use `/pr-merge-if-ready` only when the user explicitly asks to merge.

## Report

Return:

```text
## Mission Execution Report: $1

### Mode
- ...

### Completed Work
- ...

### Blocked or Failed Work
- ...

### Quality Gates
- gofmt: pass/fail/skipped
- go test ./...: pass/fail/skipped
- go build ./...: pass/fail/skipped

### Commits / PR
- ...

### Remaining Work
- ...
```
