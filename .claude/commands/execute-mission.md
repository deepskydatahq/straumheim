# Execute Mission

Full autopilot from mission alignment to PRs. Breaks down the mission into epics and stories, triages each story, then launches parallel sub-agents to implement each epic in an isolated worktree. One PR per epic.

## Arguments

- Mission ID (required): `/execute-mission M014`

## Instructions

### Phase 1: Breakdown + Triage

#### 1.1 Read the Mission

Read the mission TOML file:

```bash
ls product/missions/{mission_id}-*.toml
```

Read the file. Extract: `id`, `title`, `outcome.description`, `scope.in_scope`, `scope.out_of_scope`, `context.relevant_paths`, `testing.criteria`.

Also read `CLAUDE.md` for project conventions.

#### 1.2 Mission → Epics

**Check for existing epics first:**

```bash
ls product/epics/{mission_id}-E*.toml 2>/dev/null
```

**If epics already exist:** Read them and proceed to step 1.3.

**If no epics exist:** Create them.

Review the files listed in `relevant_paths` to understand current code state. Identify 3-6 epic boundaries based on:
- Backend vs frontend concerns
- Data layer vs presentation layer
- Core functionality vs supporting features
- Independent workstreams

For each epic, create `product/epics/{mission_id}-E{NNN}-{slug}.toml`:

```toml
id = "{mission_id}-E{NNN}"
parent = "{mission_id}"
title = "Epic title"
status = "active"
created = {today}
depends_on = []  # Other epic IDs if sequential

[outcome]
description = """What this epic delivers."""

[job_story]
description = """When..., I want..., so that..."""

[testing]
approach = "agent-judgment"
criteria = ["Verifiable outcome 1", "Verifiable outcome 2"]
validator_context = ["relevant/paths/"]

[context]
relevant_paths = ["scoped/paths/"]
dependencies = []

[notes]
considerations = """Context for story breakdown."""

estimated_stories = 0
```

Verify: all mission outcomes covered, no gaps, no overlaps, dependencies noted.

#### 1.3 Epics → Stories

For each epic:

**Check for existing stories first:**

```bash
ls product/stories/{epic_id}-S*.toml 2>/dev/null
```

**If stories already exist:** Read them and proceed to step 1.4.

**If no stories exist:** Create them.

Read the epic's `relevant_paths` to understand what exists. Break the epic into implementation units. Each story should:
- Take one implementation session
- Have a single clear purpose
- Be testable in isolation
- Result in working code

For each story, create `product/stories/{epic_id}-S{NNN}-{slug}.toml`:

```toml
id = "{epic_id}-S{NNN}"
parent = "{epic_id}"
title = "Story title"
status = "ready"
created = {today}

[outcome]
description = """One sentence: what this implements."""

[acceptance_criteria]
executable = true

[[acceptance_criteria.criteria]]
test = "unit"
description = "Specific testable behavior"

[context]
relevant_paths = ["src/specific/file.ts"]
input_fixtures = []
depends_on = []  # Other story IDs

[handoff]
implementation_hints = """Approach hints if not self-explanatory."""
reference_files = []
```

Write acceptance criteria that are specific and executable:
- Good: `calculateMRR([]) returns 0`
- Bad: `MRR calculation works correctly`

#### 1.4 LLM Triage

For each story, evaluate and assign a triage label. Read the story TOML and assess:

**READY** (skip brainstorm + plan) — assign when ALL true:
- Acceptance criteria reference specific file paths
- Changes are mechanical or well-defined (add field, wire function, update test)
- Scope is a single file or small set of files
- No design decisions needed
- Implementation approach is obvious from the story context

**PLAN** (skip brainstorm) — assign when:
- Clear goal and acceptance criteria
- Needs codebase exploration to identify exact files
- Implementation approach is obvious but details need working out
- May touch multiple files but the pattern is clear

**BRAINSTORM** (full pipeline) — assign when ANY true:
- Multiple valid approaches exist
- Architectural decisions needed
- Scope is unclear or cross-cutting
- New patterns or abstractions required
- Story notes contain open questions

Output a triage table:

```
## Triage Results

| Story | Title | Label | Justification |
|-------|-------|-------|---------------|
| M015-E001-S001 | Add parser | ready | Clear ACs, single file, mechanical |
| M015-E001-S002 | Wire into pipeline | plan | Needs exploration to identify integration points |
| M015-E002-S001 | Design metrics | brainstorm | Multiple approaches, needs design decision |
```

#### 1.5 Create Beads Tasks

For each story, create a Beads task:

```bash
bd create "{story_id}: {story_title}" --labels {triage_label} -d "{context}" --silent
```

The task description should include:
- Mission context (title + outcome, 2 sentences)
- Epic context (title + outcome, 1 sentence)
- Story outcome
- Acceptance criteria (copied from TOML)
- Relevant paths
- Implementation hints (if any)

Capture the task ID from each `bd create` output.

Register story dependencies as Beads task dependencies:

```bash
bd dep add {dependent_task_id} {blocking_task_id}
```

Record the mapping: `story_id → beads_task_id` — you'll need this for the sub-agent prompts.

#### 1.6 Build Dependency Graph

From the epic TOMLs, identify epic-level dependencies (`depends_on` field).

Classify epics:
- **Independent:** no `depends_on`, or all dependencies are outside this mission (already complete)
- **Dependent:** has `depends_on` referencing other epics in this mission

---

### Phase 2: Epic Sub-Agents

Launch one sub-agent per epic using the Agent tool with `isolation: "worktree"`.

**Dependency-aware launch algorithm:**

1. Start with all epics in a `remaining` set
2. Find `launchable` = epics in `remaining` whose `depends_on` are all completed
3. Launch all `launchable` epics in parallel (multiple Agent tool calls in ONE message)
4. When results return:
   - **Success:** move to `completed`, record PR URL
   - **Failed:** move to `failed`, also mark any epics that depend on it as `cascade-failed`
5. Repeat from step 2 until `remaining` is empty
6. If `launchable` is empty but `remaining` is not — deadlock (all remaining depend on failures). Report and stop.

**For each epic, launch an Agent with this prompt:**

Use `subagent_type: "general-purpose"` and `isolation: "worktree"`.

The prompt template (fill in the `{placeholders}` for each epic):

---

```
You are implementing epic {epic_id} for the basesignal project.

## Project Context

- Monorepo with npm workspaces in packages/
- Test: npm test (vitest, watch mode) or npm run test:run (CI, single run)
- Build: npm run build
- Every feature must have tests
- Import from package names (@basesignal/core), not relative cross-package paths

## Mission: {mission_title}

{mission_outcome — 2-3 sentences}

## Epic: {epic_id} — {epic_title}

{epic_outcome_description}

## Stories to Implement (in order)

{For each story in this epic, numbered:}

### Story {N}: {story_id} — {story_title}
**Triage:** {ready|plan|brainstorm}
**Outcome:** {story_outcome}
**Beads Task:** {beads_task_id}
**Acceptance Criteria:**
{list each criterion with test type}
**Relevant Paths:** {paths}
**Implementation Hints:** {hints or "none"}
**Depends On:** {story dependencies or "none"}

{...repeat for each story...}

## Your Process

For each story, in order:

1. **Claim the task:**
   bd update {beads_task_id} --status in_progress

2. **Handle based on triage label:**

   If BRAINSTORM:
   - Read the relevant code paths
   - Consider 2-3 approaches, pick the simplest one
   - Note your design decision in a brief comment in the code or commit message
   - Then proceed to plan and implement

   If PLAN:
   - Read the relevant code paths
   - Identify the specific files to create or modify
   - Then implement

   If READY:
   - Go straight to implementation

3. **Implement:**
   - Write the code changes
   - Write tests for each acceptance criterion
   - Run tests: npm run test:run
   - Fix any test failures
   - Commit with a descriptive message: feat: {description}

4. **Close the task:**
   bd close {beads_task_id}

5. Move to the next story.

## After All Stories

1. Run the full test suite: npm run test:run
2. Run the build: npm run build
3. Fix any issues
4. Push and create a PR:
   git push -u origin HEAD
   gh pr create --title "feat({epic_id}): {epic_title}" --body "## Summary
   {epic_outcome}

   ## Stories Implemented
   {list of story_ids and titles}

   ## Test Results
   All tests passing.

   ---
   Automated via /execute-mission"

## Rules

- Do NOT ask questions. Make autonomous decisions — pick the simpler option when unsure.
- If you hit a blocker on one story, document it as a comment and move to the next.
- Commit after each story — incremental commits, not one big commit.
- All tests MUST pass before creating the PR.
- If tests fail after all attempts, create the PR anyway but note failures in the PR body.

## Output

When complete, output exactly this block:

EPIC_RESULT: {epic_id}
STATUS: success | partial | failed
PR_URL: {url or "none"}
STORIES_COMPLETED: {comma-separated story_ids}
STORIES_FAILED: {comma-separated story_ids with brief reasons, or "none"}
TASKS_CLOSED: {comma-separated beads_task_ids}
```

---

### Phase 3: PR Feedback

After all epic sub-agents complete, collect the PR URLs from their results.

Wait 60 seconds for automated reviewers (Graptile, CodeRabbit) to process the PRs:

```bash
sleep 60
```

For each PR that was created successfully, launch a sub-agent to fix feedback:

Use the Agent tool with `subagent_type: "general-purpose"` (no worktree isolation needed — it works on the existing PR branch).

Prompt: `"Run /fix-pr-feedback {pr_number} — check out the PR branch, read review comments, fix actionable issues, push fixes. Max 2 rounds."`

Launch all PR feedback agents in parallel (multiple Agent calls in one message, use `run_in_background: true`).

---

### Phase 4: Report

Collect results from all agents and output:

```
## Mission Execution Report: {mission_id} — {mission_title}

### Epics

| Epic | Status | PR | Stories |
|------|--------|-----|---------|
| E001 | success | #{pr} | 3/3 completed |
| E002 | partial | #{pr} | 2/3 completed |
| E003 | cascade-failed | — | blocked by E001 |

### PRs Created
- #{pr}: feat(E001): {title} — {url}
- #{pr}: feat(E002): {title} — {url}

### Tasks Closed
{list of beads task IDs}

### Failures Requiring Attention
{list of failed stories with reasons, or "None — all stories completed successfully."}

### PR Feedback
- #{pr}: {N} issues fixed, {N} unresolved
- #{pr}: No actionable feedback

### Summary
- Epics: {completed}/{total} succeeded
- Stories: {completed}/{total} implemented
- PRs: {count} created
- Tasks: {count} closed
```
