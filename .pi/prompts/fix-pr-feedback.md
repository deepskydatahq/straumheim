---
description: Fix actionable automated PR review feedback with at most two rounds
argument-hint: "<pr-number>"
---
# Fix PR Feedback

Fix actionable PR review feedback for PR `$1`. Maximum two rounds.

## Setup

Before editing, use `/pr-status $1` or equivalent `gh` commands to inspect checks, mergeability, reviews, issue comments, and inline comments.

1. Get repository and PR metadata:
   - `gh repo view --json nameWithOwner --jq '.nameWithOwner'`
   - `gh pr view $1 --json headRefName,baseRefName,title,state,url`
2. Check out the PR branch:
   - `git fetch origin`
   - `git checkout {headRefName}`
   - `git pull origin {headRefName}`
3. Stop if the PR is closed or merged.

## Fetch comments

Collect:

- inline review comments via `gh api repos/{owner}/{repo}/pulls/$1/comments --paginate`
- review summaries via `gh api repos/{owner}/{repo}/pulls/$1/reviews --paginate`
- issue-level PR comments via `gh pr view $1 --json comments --jq '.comments[]'`

## Classify comments

Classify each as:

- `ACTIONABLE`: concrete code issue, missing test, security concern, unresolved specific review comment
- `NOISE`: summary, praise, optional nit, vague question, already-resolved item
- `DEFERRED`: valid but out of scope or architectural follow-up

Present a table before fixing.

## Fix rounds

1. Fix actionable items in file order.
2. Add or update tests where needed.
3. Run relevant focused tests, then `gofmt -w` on changed Go files, `go test ./...`, and `go build ./...`.
4. Commit and push.
5. Wait for new review feedback and do at most one more round.
6. Do not attempt a third round.

## Report

Return fixed, deferred, skipped, quality gate results, and PR URL.
