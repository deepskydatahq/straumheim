# Straumheim Pi Setup

Project-local Pi resources for Straumheim's product TOML workflow.

## Mission commands

Prompt templates in `.pi/prompts/` become slash commands:

- `/plan-mission <idea>` — explore the repository and create a mission TOML
- `/product-mission-breakdown <Mxxx>` — create epics from a mission
- `/product-epic-breakdown <Mxxx-Exxx>` — create stories from an epic
- `/execute-mission <Mxxx>` — execute a mission from product TOML
- `/product-judgment <artifact>` — validate story, epic, or mission outcomes
- `/fix-pr-feedback <pr>` — fix actionable PR feedback
- `/retro [scope]` — discover follow-up product stories
- `/landing-plane` — run gates, update status, commit, and push

## Skills

- `straumheim-product-workflow` — mission, epic, and story planning
- `straumheim-story-execution` — story implementation and completion

## Extension

`.pi/extensions/straumheim-workflow/index.ts` adds:

- `/straumheim-status`
- `/mission-status <Mxxx>`
- `/story-list [status]`
- `/story-set-status <story-id> <status>`
- `/quality-gates`
- `/pr-status [pr-number]`
- `/pr-merge-if-ready [pr-number]`

The extension also reminds the model that product TOML files are canonical and runs Go-specific quality gates.

## Task model

Stories in `product/stories/*.toml` are implementation tasks:

- Status: `draft`, `ready`, `in_progress`, `blocked`, `complete`, or `failed`
- Triage: `ready`, `plan`, or `brainstorm`

## Quality gates

```bash
gofmt -w <changed-go-files>
go test ./...
go build ./...
```

Run `/reload` after changing project-local Pi resources. Pi must trust the project before loading them.
