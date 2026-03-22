---
description: Post-implementation retrospective - discover issues and create Beads tasks
allowed-tools: Bash(git:*), Bash(bd:*), Bash(npm run:*), Grep, Glob, Read, Edit
---

# Retrospective

Analyze recent implementation work and discover follow-up issues.

## Instructions

### 1. Identify the Scope of Recent Work

```bash
git diff --name-only $(git merge-base HEAD main)..HEAD
```

If no changes from main, check recent commits:
```bash
git log --oneline -10 --name-only
```

### 2. Run Full Verification Suite

Run all checks and capture any failures:

```bash
# Run your project's verification commands
# Examples:
# npm run lint && npm run test
# pytest && ruff check .
# dbt test
```

Capture all warnings and errors from these checks - they become findings.

### 2a. Analyze Recent Failures

**Check for tasks that failed (moved back to plan status with failure notes):**

```bash
bd list --label plan --json
```

Look for tasks with "## Implementation Failed" in their body.

**For each failure, analyze:**

1. **Read failure notes** - Look for "## Implementation Failed" section in task body
2. **Identify category** - plan-quality, test-failures, blockers, scope-creep, abandoned
3. **Extract lesson** - What should be done differently next time?

**Promote patterns to PROGRESS.md:**

If PROGRESS.md exists, add to "Failure Patterns" section (create section if missing):

```markdown
## Failure Patterns

### <category>: <brief description>
**From:** task <id> (YYYY-MM-DD)
**Why it failed:** <specific cause>
**Lesson:** <actionable guidance for future implementations>
```

Example:
```markdown
### plan-quality: Missing file paths in tasks
**From:** task 01KFJ4YM... (2026-01-18)
**Why it failed:** Tasks said "update config" without specifying which file
**Lesson:** Every task must have explicit file path like `src/config/settings.ts`
```

**After analysis:**
- Update task to remove failure notes if resolved
- Commit PROGRESS.md if patterns were added

```bash
git add PROGRESS.md && git commit -m "docs: add failure pattern from task <id>"
```

### 3. For Each Changed File, Analyze

**Related files to check:**
- Test files for changed modules
- Files that import the changed module (use Grep)
- Similar files in same directory (same pattern)

**Patterns to grep for:**
- If you fixed `any` types, search for similar usage
- If you fixed missing error handling, search for similar patterns
- If you added validation, check for missing validation elsewhere

### 4. Categorize Findings

| Category | Label | Look For |
|----------|-------|----------|
| Bug | `bug` | Failing tests, wrong values, broken assertions |
| Tech debt | `tech-debt` | `any` types, linting warnings, TODOs, deprecated patterns |
| Refactoring | `refactoring` | Code duplication, inconsistent patterns, unclear naming |
| Enhancement | `enhancement` | Missing features, incomplete APIs, useful extensions |
| Documentation | `documentation` | Outdated docs, missing examples, stale comments |
| Testing | `testing` | Missing test coverage, test patterns to improve |

### 5. Assess Status for Each Finding

| Status | Criteria |
|--------|----------|
| `brainstorm` | Problem unclear, multiple approaches possible, needs design decisions |
| `plan` | Problem clear, solution known, but involves multiple files/steps |
| `ready` | Trivial fix - specific file/line known, mechanical change (RARE) |

**Default to `plan`** - only use `ready` for truly trivial fixes.

### 6. Assess Priority

| Priority | Criteria |
|----------|----------|
| High | Blocking other work, causing failures, security concern |
| Medium | Should fix soon, affects code quality |
| Low | Nice to have, cleanup, minor improvement |

### 7. Present Findings

```
## Retro Findings

| # | Category | File | Issue | Priority | Status | Effort |
|---|----------|------|-------|----------|--------|--------|
| 1 | tech-debt | src/X.ts | Uses `any` type for props | Low | ready | Quick win |
| 2 | bug | src/Y.ts | Missing validation for edge case | Medium | plan | Medium |
| 3 | testing | src/Z.ts | No test coverage | Medium | plan | Medium |

Create all N tasks? (y/n)
```

### 8. If Confirmed, Create Tasks

For each finding, create a Beads task:

```bash
bd create "<Task title>" --labels <status> -d "## Problem\n<description of issue>\n\n## Context\nDiscovered during retro after <recent work description>.\n\n## Solution\n<suggested fix if known>\n\n## Files\n- \`<file path>\`\n\n---\n*Priority: <priority> | Effort: <effort>*\n*Created via /retro*"
```

### 9. Summarize

```
Created N tasks:
- <id>: <title> (status: <status>)
- <id>: <title> (status: <status>)
```

### 10. Review PROGRESS.md for Pattern Promotion

After creating issues, analyze recent session entries for promotable patterns:

1. **Read recent entries** from PROGRESS.md Session Log

2. **Identify candidates** - patterns that are:
   - Transferable to other issues/contexts
   - Not obvious from documentation
   - You can articulate WHY in one sentence

3. **For each promotable pattern**, add to Reusable Patterns section:

```markdown
### [Pattern Name]
**From:** #<issue> (YYYY-MM-DD)
**Why:** <one sentence explaining transferability>
**Pattern:** <the actual pattern/approach>
```

Example:
```markdown
### Electron IPC channel registration
**From:** #73 (2026-01-18)
**Why:** Any Electron IPC feature needs preload registration - not obvious from docs
**Pattern:** Register channels in preload.ts contextBridge before main process handler
```

4. **Commit if patterns promoted:**

```bash
git add PROGRESS.md
git commit -m "docs: promote patterns from recent implementation work"
```

**Promotion criteria:**
- Transferable: Would help with future similar work
- Non-obvious: Not in official docs or easy to discover
- Articulable: Can explain value in one sentence

## Status Assessment Examples

| Finding | Status | Reasoning |
|---------|--------|-----------|
| "Replace `any` with proper type" | `ready` | Mechanical replacement, single file |
| "Fix incorrect API endpoint in test" | `ready` | Fix specific line, obvious change |
| "Add error boundary for async operations" | `plan` | Clear what to do, but multiple places |
| "Inconsistent mutation patterns" | `plan` | Clear goal, needs to identify all occurrences |
| "Should we add request caching?" | `brainstorm` | Multiple strategies, needs design decision |

## Integration

The `superpowers:finishing-a-development-branch` skill should suggest running `/retro` before completing work:

```
Implementation complete. Before finishing:
- Run /retro to discover follow-up issues? (recommended)
```
