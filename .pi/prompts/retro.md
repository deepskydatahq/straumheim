---
description: Run a retrospective and create follow-up product stories instead of Beads tasks
argument-hint: "[scope or branch]"
---
# Retrospective

Analyze recent implementation work and discover follow-up product work. Do not use Beads/`bd`.

Scope: $ARGUMENTS

## Steps

1. Identify changed files:
   - `git diff --name-only $(git merge-base HEAD main)..HEAD`
   - if empty, inspect recent commits with `git log --oneline -10 --name-only`
2. Run relevant verification:
   - `gofmt -w` on changed Go files
   - `go test ./...`
   - `go build ./...`
3. Analyze changed files and related files:
   - tests for changed modules
   - importers of changed modules
   - similar files using the same pattern
   - protocol, configuration, and operational implications
4. Categorize findings:
   - bug
   - tech-debt
   - refactoring
   - enhancement
   - documentation
   - testing
   - reliability
   - operations
5. Assign implementation status:
   - `ready` for trivial, specific mechanical fixes
   - `plan` for clear multi-step work
   - `brainstorm` for ambiguous or architectural work
6. Present findings and ask before creating new product story files.
7. If confirmed, create follow-up stories in the relevant epic when possible. If no epic fits, create a draft story under the closest mission/epic and clearly mark it for review.

## Story creation guidance

Follow-up stories should include:

- specific outcome
- executable acceptance criteria
- relevant paths
- dependencies
- `triage = "ready" | "plan" | "brainstorm"`
- `status = "ready"` or `status = "draft"`

## Report

Return created stories, skipped findings, and any recommended mission/epic updates.
