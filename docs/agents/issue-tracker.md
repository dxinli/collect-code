# Issue tracker: GitHub

Issues for this repo live as GitHub issues. Use the `gh` CLI for all
operations. Plans and diff notes stay under `.scratch/`.

## Conventions

- **Create an issue**: `gh issue create --template ticket.md`, or
  `gh issue create --title "05a: ..." --body-file ... --label enhancement`.
- **Read an issue**: `gh issue view <number> --json number,title,body,labels,comments`.
- **List issues**: `gh issue list --state open --label ready-for-agent --json number,title,body,labels`.
- **Comment**: `gh issue comment <number> --body "..."`.
- **Apply / remove labels**: `gh issue edit <number> --add-label "..."` /
  `--remove-label "..."`.
- **Close**: `gh issue close <number> --reason completed` (or `not planned`).
- **Ticket ID**: the title starts with the stable ID, e.g. `05a: MQTT 通道`;
  the body starts with `**Ticket:** 05a` so scripts can find it after a title
  edit.
- **Plan and diff**: a ticket's implementation plan lives in its PR body (Plan
  section); diff notes stay at `.scratch/<feature>/diffs/<id>.md`. Historical
  tickets may link an archived plan under `.scratch/<feature>/plans/<id>.md`.

## Triage labels

Issues move through the five state labels and carry one of the two category
labels. The mapping and transition rules are in
[`triage-labels.md`](triage-labels.md). The canonical skill is the `triage`
skill; invoke it as `/triage` when you want it to drive a triage session.

## Dependencies

Use GitHub's native issue dependencies. An issue is startable when every
blocker is closed.

```bash
gh issue edit <child> --add-blocked-by <blocker>,<blocker>
gh api repos/<owner>/<repo>/issues/<n> --jq .issue_dependencies_summary.blocked_by
```

`issue_dependencies_summary.blocked_by` counts only **open** blockers; `0`
means startable. `gh issue view <n> --json blockedBy` lists closed blockers
too, so filter them by state if you use it.

Fallback when native dependencies are unavailable: put
`Blocked by: #<n>, #<n>` at the top of the child body.

## Pull requests

Ticket PRs must contain the closing keyword so GitHub closes the issue when
the PR merges:

```md
Closes #31
```

That is the whole post-merge workflow. There is no ticket-sync bot and no
`Merged:` field; the issue timeline and its `closed` state are the record.

Plan-only PRs do not need a `Closes` line.

## When a skill says "publish to the issue tracker"

Create a GitHub issue.

## When a skill says "fetch the relevant ticket"

Read it with `gh issue view`; the implementation plan is in the ticket PR
that contains `Closes #<issue>`.

## Pull requests as a triage surface

**PRs as a request surface: no.** `/triage` only looks at issues in this repo.

## Ready-to-start view

```bash
make tickets
```

lists open `ready-for-agent` issues whose blockers are all closed, plus the
blocked ones and their open blockers. It requires an authenticated `gh`.

## Complete-ticket walkthroughs

A complete ticket group (every ticket sharing a numeric prefix: `01a/01b/01c`,
`02a/02b`, `06a/06b`) is closed once all its issues are closed. At that point a
committed `docs/代码走读/<NN>-*.md` is expected: the link path with a diagram,
component relationships with a dependency diagram, and timing with a sequence
diagram where applicable. This is a review artifact, not a CI gate; check the
tracked files with `git ls-files 'docs/代码走读/*.md'`.
