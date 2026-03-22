# Fix PR Feedback

Fix automated PR review feedback. Reads review comments (Graptile, CodeRabbit, etc.), classifies them, fixes actionable issues, and pushes. Max 2 rounds.

## Arguments

- PR number (required): `/fix-pr-feedback 42`

## Instructions

### 1. Setup

Parse the PR number from the argument. Get repo info and PR metadata:

```bash
gh repo view --json nameWithOwner --jq '.nameWithOwner'
gh pr view {pr_number} --json headRefName,baseRefName,title,state,url
```

Check out the PR branch:

```bash
git fetch origin
git checkout {headRefName}
git pull origin {headRefName}
```

If the PR is closed or merged, report that and stop.

### 2. Fetch Review Comments

Collect all review feedback from the PR:

```bash
# Inline code review comments (file-level, with diff context)
gh api repos/{owner}/{repo}/pulls/{pr_number}/comments --paginate

# Review summaries (approve/request changes/comment)
gh api repos/{owner}/{repo}/pulls/{pr_number}/reviews --paginate

# Issue-level comments (general discussion)
gh pr view {pr_number} --json comments --jq '.comments[]'
```

Identify the reviewer source from `user.login`:
- Bot reviewers: `greptile[bot]`, `coderabbitai[bot]`, `github-actions[bot]`
- Human reviewers: any other login

### 3. Classify Each Comment

For every comment, classify into one of three categories:

**ACTIONABLE** — fix now:
- Points to a specific code issue (bug, missing error handling, type error, broken import)
- Suggests a concrete fix with code example
- Identifies a test gap ("no test for X")
- Flags a security concern
- Is an unresolved inline review comment on a specific file/line

**NOISE** — skip:
- Pure style preference with no functional impact
- Asks a question without suggesting a change
- Bot summary/overview comment (not about specific code)
- Already-resolved comment
- Praise or acknowledgment ("looks good", "nice")
- Nitpick explicitly labeled as optional

**DEFERRED** — valid but out of scope:
- Requires architectural change beyond this PR
- Performance concern that needs benchmarking
- Suggests refactoring files outside this PR's changeset
- Feature request disguised as review feedback

Present the classification as a table before proceeding:

```
| # | Source | File | Category | Summary |
|---|--------|------|----------|---------|
| 1 | greptile[bot] | src/foo.ts:42 | ACTIONABLE | Missing null check |
| 2 | greptile[bot] | — | NOISE | Summary comment |
| 3 | greptile[bot] | src/bar.ts:10 | DEFERRED | Suggests refactoring shared util |
```

If there are no ACTIONABLE comments, report "No actionable feedback found" and stop.

### 4. Fix Round 1

For each ACTIONABLE comment, in file order:

1. Read the file and line referenced by the comment
2. Understand the specific issue
3. Apply the fix
4. If the fix requires updating a test, update it

After all fixes:

```bash
npm run test:run
npm run build
```

If tests or build fail, fix the issues before proceeding.

Stage and commit:

```bash
git add {specific files changed}
git commit -m "fix: address PR review feedback (round 1)

Fixes N issues from automated review.

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
git push origin {headRefName}
```

### 5. Round 2 (Conditional)

Wait 30 seconds for automated reviewers to re-process:

```bash
sleep 30
```

Fetch comments created after the push:

```bash
# Get comments, filter by created_at after our push
gh api repos/{owner}/{repo}/pulls/{pr_number}/comments --paginate
```

Compare timestamps — only look at comments created after the round 1 push.

If new ACTIONABLE comments exist:
1. Classify them (same rules as step 3)
2. Fix actionable items
3. Test and build
4. Commit and push as "round 2"

If no new actionable comments, or only NOISE/DEFERRED, stop.

**Maximum 2 rounds total.** Do not attempt a third round.

### 6. Report

Output a summary:

```
## PR Feedback Report: #{pr_number}

### Round 1
**Fixed (N):**
- {file}:{line} — {issue description} — fixed
- {file}:{line} — {issue description} — fixed

**Deferred (N):**
- {description} — out of scope for this PR

**Skipped (N noise comments)**

### Round 2
{Same format, or "No new actionable feedback after round 1."}

### Summary
- Rounds: {1 or 2}
- Issues fixed: {total}
- Deferred: {total}
- PR: {pr_url}
```
