---
description: Finish the current work session using the product TOML workflow
---
# Landing the Plane

Finish the current work session. Product TOML is canonical. Do not use Beads/`bd`.

## Steps

1. Inspect worktree:
   - `git status --short --branch`
   - `git diff --stat`
2. File remaining work as product stories if follow-up is needed.
3. Run quality gates if code or generated page output changed:
   - `gofmt -w` on changed Go files
   - `go test ./...`
   - `go build ./...`
4. Update product TOML status:
   - completed stories/epics/missions to `complete`
   - blocked/failed work with explicit reasons
5. Commit intended changes with clear, scoped messages.
6. Push to remote:
   - `git pull --rebase`
   - `git push`
   - `git status --short --branch`
7. Verify the branch is up to date with origin.
8. Hand off with completed work, quality gates, commits, remaining stories, and blockers.
